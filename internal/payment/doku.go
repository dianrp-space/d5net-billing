package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
)

const ProviderDoku = "doku"

const (
	DokuSandboxBaseURL    = "https://api-sandbox.doku.com"
	DokuProductionBaseURL = "https://api.doku.com"
	DokuCheckoutPath      = "/checkout/v1/payment"
	DokuStatusPath        = "/orders/v1/status"
	DokuWebhookPath       = "/api/webhooks/payment/doku"
	DefaultDokuExpiryMin  = 60
	MaxDokuExpiryMinutes  = 1440
)

func ClampDokuExpiryMinutes(n int) int {
	if n < 1 {
		return DefaultDokuExpiryMin
	}
	if n > MaxDokuExpiryMinutes {
		return MaxDokuExpiryMinutes
	}
	return n
}

func DokuBaseURL(sandbox bool) string {
	if sandbox {
		return DokuSandboxBaseURL
	}
	return DokuProductionBaseURL
}

type DokuProvider struct {
	ClientID         string
	SecretKey        string
	// Direct API (SNAP QRIS) credentials; only needed for QR ops.
	PrivateKey string
	MerchantID string
	TerminalID string
	PostalCode string
	Sandbox          bool
	ExpiresInMinutes int
	HTTP             *http.Client
	// baseOverride reroutes all calls (tests only).
	baseOverride string
}

func NewDokuProvider(clientID, secretKey, privateKey, merchantID, terminalID, postalCode string, sandbox bool, expiryMin int) *DokuProvider {
	return &DokuProvider{
		ClientID:         strings.TrimSpace(clientID),
		SecretKey:        strings.TrimSpace(secretKey),
		PrivateKey:       strings.TrimSpace(privateKey),
		MerchantID:       strings.TrimSpace(merchantID),
		TerminalID:       strings.TrimSpace(terminalID),
		PostalCode:       strings.TrimSpace(postalCode),
		Sandbox:          sandbox,
		ExpiresInMinutes: ClampDokuExpiryMinutes(expiryMin),
		HTTP:             &http.Client{Timeout: 25 * time.Second},
	}
}

func (p *DokuProvider) Name() string { return ProviderDoku }

func (p *DokuProvider) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return &http.Client{Timeout: 25 * time.Second}
}

func (p *DokuProvider) baseURL() string {
	if p.baseOverride != "" {
		return strings.TrimRight(p.baseOverride, "/")
	}
	return DokuBaseURL(p.Sandbox)
}

// dokuTimestamp returns UTC ISO8601 as required by DOKU (subtract 7h from WIB).
func dokuTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// dokuSignature builds "HMACSHA256=<base64>" over the header components.
// Digest is included for POST requests with a body, omitted for GET.
func dokuSignature(secret, clientID, requestID, timestamp, target string, body []byte) string {
	var b strings.Builder
	b.WriteString("Client-Id:")
	b.WriteString(clientID)
	b.WriteString("\nRequest-Id:")
	b.WriteString(requestID)
	b.WriteString("\nRequest-Timestamp:")
	b.WriteString(timestamp)
	b.WriteString("\nRequest-Target:")
	b.WriteString(target)
	if body != nil {
		sum := sha256.Sum256(body)
		b.WriteString("\nDigest:")
		b.WriteString(base64.StdEncoding.EncodeToString(sum[:]))
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(b.String()))
	return "HMACSHA256=" + base64.StdEncoding.EncodeToString(m.Sum(nil))
}

func dokuPhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	p := b.String()
	if strings.HasPrefix(p, "0") {
		p = "62" + p[1:]
	}
	return p
}

func (p *DokuProvider) CreateIntent(ctx context.Context, req IntentRequest) (*IntentResult, error) {
	if p.ClientID == "" || p.SecretKey == "" {
		return nil, fmt.Errorf("DOKU belum dikonfigurasi")
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("nominal pembayaran harus > 0")
	}
	orderID := strings.TrimSpace(req.MerchantOrderID)
	if orderID == "" {
		orderID = fmt.Sprintf("inv-%s", req.InvoiceID)
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = "noreply@localhost"
	}
	name := strings.TrimSpace(req.CustomerName)
	if name == "" {
		name = "Pelanggan"
	}
	expiry := ClampDokuExpiryMinutes(p.ExpiresInMinutes)
	payload, err := json.Marshal(map[string]any{
		"order": map[string]any{
			"amount":         req.Amount,
			"invoice_number": orderID,
			"currency":       "IDR",
			"callback_url":   strings.TrimSpace(req.ReturnURL),
			"language":       "ID",
			"auto_redirect":  true,
		},
		"payment": map[string]any{
			"payment_due_date": expiry,
			"type":             "SALE",
		},
		"customer": map[string]any{
			"id":      truncateRunes(strings.TrimSpace(req.CustomerName), 50),
			"name":    truncateRunes(name, 255),
			"phone":   dokuPhone(req.Phone),
			"email":   email,
			"country": "ID",
		},
	})
	if err != nil {
		return nil, err
	}
	requestID := xid.New().String()
	timestamp := dokuTimestamp(time.Now())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL()+DokuCheckoutPath, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Client-Id", p.ClientID)
	httpReq.Header.Set("Request-Id", requestID)
	httpReq.Header.Set("Request-Timestamp", timestamp)
	httpReq.Header.Set("Signature", dokuSignature(p.SecretKey, p.ClientID, requestID, timestamp, DokuCheckoutPath, payload))

	resp, err := p.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("DOKU: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DOKU %d: %s", resp.StatusCode, dokuErrMessage(body))
	}
	var out struct {
		Message  []string `json:"message"`
		Response struct {
			Payment struct {
				URL         string `json:"url"`
				TokenID     string `json:"token_id"`
				ExpiredDate string `json:"expired_date"`
			} `json:"payment"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("DOKU: parse response: %w", err)
	}
	url := strings.TrimSpace(out.Response.Payment.URL)
	if url == "" {
		return nil, fmt.Errorf("DOKU: payment url kosong (%s)", strings.Join(out.Message, "; "))
	}
	return &IntentResult{
		ExternalID:    orderID,
		TransactionID: strings.TrimSpace(out.Response.Payment.TokenID),
		CheckoutURL:   url,
		Status:        "pending",
		ExpiresAt:     dokuExpiredDate(out.Response.Payment.ExpiredDate, expiry),
		Amount:        req.Amount,
		PayableAmount: req.Amount,
		Metadata: map[string]any{
			"doku_sandbox": p.Sandbox,
		},
	}, nil
}

// dokuExpiredDate parses DOKU's yyyyMMddHHmmss (UTC+7), falling back to now+expiry.
func dokuExpiredDate(s string, expiryMin int) *time.Time {
	s = strings.TrimSpace(s)
	if s != "" {
		if t, err := time.ParseInLocation("20060102150405", s, time.FixedZone("WIB", 7*3600)); err == nil {
			return &t
		}
	}
	t := time.Now().Add(time.Duration(ClampDokuExpiryMinutes(expiryMin)) * time.Minute)
	return &t
}

func (p *DokuProvider) CheckStatus(ctx context.Context, referenceID string) (*IntentResult, error) {
	referenceID = strings.TrimSpace(referenceID)
	if referenceID == "" {
		return nil, fmt.Errorf("invoice_number kosong")
	}
	if p.ClientID == "" || p.SecretKey == "" {
		return nil, fmt.Errorf("DOKU belum dikonfigurasi")
	}
	target := DokuStatusPath + "/" + referenceID
	requestID := xid.New().String()
	timestamp := dokuTimestamp(time.Now())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL()+target, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Client-Id", p.ClientID)
	httpReq.Header.Set("Request-Id", requestID)
	httpReq.Header.Set("Request-Timestamp", timestamp)
	httpReq.Header.Set("Signature", dokuSignature(p.SecretKey, p.ClientID, requestID, timestamp, target, nil))
	resp, err := p.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("DOKU: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DOKU %d: %s", resp.StatusCode, dokuErrMessage(body))
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("DOKU: parse status: %w", err)
	}
	return dokuStatusResult(referenceID, m), nil
}

func dokuStatusResult(referenceID string, m map[string]any) *IntentResult {
	ext := dokuNestedString(m, "order", "invoice_number")
	if ext == "" {
		ext = referenceID
	}
	amount := dokuNestedAmount(m)
	ref := dokuNestedString(m, "transaction", "original_request_id")
	return &IntentResult{
		ExternalID:    ext,
		TransactionID: ref,
		Status:        dokuStatus(dokuNestedString(m, "transaction", "status"), dokuNestedString(m, "order", "status")),
		Amount:        amount,
		PayableAmount: amount,
	}
}

func dokuStatus(txStatus, orderStatus string) string {
	switch strings.ToUpper(strings.TrimSpace(txStatus)) {
	case "SUCCESS":
		return "paid"
	case "PENDING":
		return "pending"
	case "EXPIRED":
		return "expired"
	case "FAILED":
		return "failed"
	}
	switch strings.ToUpper(strings.TrimSpace(orderStatus)) {
	case "ORDER_EXPIRED":
		return "expired"
	}
	if WebhookIsPaid(txStatus) {
		return "paid"
	}
	return strings.ToLower(strings.TrimSpace(txStatus))
}

func (p *DokuProvider) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	// SNAP (Direct API) notifications carry X-PARTNER-ID; checkout
	// notifications use Client-Id + Signature instead.
	if strings.TrimSpace(headerGet(headers, "X-PARTNER-ID")) != "" {
		return p.dokuVerifySnapWebhook(ctx, headers, body)
	}
	_ = ctx
	if p.SecretKey == "" {
		return nil, fmt.Errorf("DOKU webhook secret belum dikonfigurasi")
	}
	clientID := strings.TrimSpace(headerGet(headers, "Client-Id"))
	requestID := strings.TrimSpace(headerGet(headers, "Request-Id"))
	timestamp := strings.TrimSpace(headerGet(headers, "Request-Timestamp"))
	sig := strings.TrimSpace(headerGet(headers, "Signature"))
	if clientID == "" || requestID == "" || timestamp == "" || sig == "" {
		return nil, fmt.Errorf("header notifikasi DOKU tidak lengkap")
	}
	want := dokuSignature(p.SecretKey, clientID, requestID, timestamp, DokuWebhookPath, body)
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return nil, fmt.Errorf("invalid DOKU webhook signature")
	}
	var m map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &m); err != nil {
			return nil, fmt.Errorf("parse webhook body: %w", err)
		}
	}
	ext := dokuNestedString(m, "order", "invoice_number")
	if ext == "" {
		return nil, fmt.Errorf("invoice_number tidak ditemukan di notifikasi")
	}
	amount := dokuNestedAmount(m)
	return &WebhookEvent{
		ExternalID: ext,
		Status:     dokuStatus(dokuNestedString(m, "transaction", "status"), dokuNestedString(m, "order", "status")),
		Amount:     amount,
		Reference:  dokuNestedString(m, "transaction", "original_request_id"),
		Raw:        m,
	}, nil
}

func dokuNestedString(m map[string]any, keys ...string) string {
	cur := any(m)
	for _, k := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = obj[k]
	}
	s, _ := cur.(string)
	return strings.TrimSpace(s)
}

func dokuNestedAmount(m map[string]any) int64 {
	var obj map[string]any
	if o, ok := m["order"].(map[string]any); ok {
		obj = o
	} else {
		obj = m
	}
	return toInt64(obj["amount"])
}

func dokuErrMessage(body []byte) string {
	var e struct {
		ErrorMessages []string `json:"error_messages"`
		Message       []string `json:"message"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if msg := strings.TrimSpace(firstNonEmpty(append(e.ErrorMessages, e.Message...)...)); msg != "" {
			return msg
		}
	}
	s := strings.TrimSpace(string(body))
	if s == "" {
		return "request failed"
	}
	if len(s) > 240 {
		return s[:240]
	}
	return s
}
