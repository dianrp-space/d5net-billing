package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const DefaultDRPBaseURL = "https://payment.dianrp.com"

const DefaultQRISExpiresMinutes = 15
const MaxQRISExpiresMinutes = 1440

func ClampQRISExpiresMinutes(n int) int {
	if n < 1 {
		return DefaultQRISExpiresMinutes
	}
	if n > MaxQRISExpiresMinutes {
		return MaxQRISExpiresMinutes
	}
	return n
}

type DRPProvider struct {
	BaseURL          string
	APIKey           string
	WebhookSecret    string
	ExpiresInMinutes int
	HTTP             *http.Client
}

func NewDRPProvider(baseURL, apiKey, webhookSecret string) *DRPProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultDRPBaseURL
	}
	return &DRPProvider{
		BaseURL:          baseURL,
		APIKey:           strings.TrimSpace(apiKey),
		WebhookSecret:    webhookSecret,
		ExpiresInMinutes: DefaultQRISExpiresMinutes,
		HTTP:             &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *DRPProvider) Name() string { return ProviderDRP }

func (p *DRPProvider) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (p *DRPProvider) ttl() int {
	return ClampQRISExpiresMinutes(p.ExpiresInMinutes)
}

type drpCreateRequest struct {
	ReferenceID      string `json:"referenceId"`
	Amount           int64  `json:"amount"`
	Fee              int64  `json:"fee"`
	ExpiresInMinutes int    `json:"expiresInMinutes"`
}

type drpTransaction struct {
	TransactionID   string  `json:"transactionId"`
	ReferenceID     string  `json:"referenceId"`
	Status          string  `json:"status"`
	Amount          int64   `json:"amount"`
	Fee             int64   `json:"fee"`
	UniqueDigit     int64   `json:"uniqueDigit"`
	TotalAmount     int64   `json:"totalAmount"`
	QRISString      string  `json:"qrisString"`
	QRISImageBase64 string  `json:"qrisImageBase64"`
	ExpiresAt       string  `json:"expiresAt"`
	PaidAt          *string `json:"paidAt"`
	PaidAmount      *int64  `json:"paidAmount"`
}

type drpErrorBody struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (p *DRPProvider) CreateIntent(ctx context.Context, req IntentRequest) (*IntentResult, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("DRP Payment API key belum dikonfigurasi")
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("nominal pembayaran harus > 0")
	}
	ref := fmt.Sprintf("drp-%s", req.InvoiceID)
	res, err := p.createQRIS(ctx, ref, req.Amount)
	if err != nil && isConflict(err) {
		ref = fmt.Sprintf("drp-%s-%d", req.InvoiceID, time.Now().Unix())
		res, err = p.createQRIS(ctx, ref, req.Amount)
	}
	return res, err
}

func isConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "409")
}

func (p *DRPProvider) createQRIS(ctx context.Context, referenceID string, amount int64) (*IntentResult, error) {
	payload, err := json.Marshal(drpCreateRequest{
		ReferenceID:      referenceID,
		Amount:           amount,
		Fee:              0,
		ExpiresInMinutes: p.ttl(),
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v2/qris", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := p.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("DRP Payment: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("DRP Payment %d: %s", resp.StatusCode, drpErrMessage(body))
	}
	var tx drpTransaction
	if err := json.Unmarshal(body, &tx); err != nil {
		return nil, fmt.Errorf("DRP Payment: parse response: %w", err)
	}
	return tx.toIntent(amount), nil
}

func (p *DRPProvider) CheckStatus(ctx context.Context, referenceID string) (*IntentResult, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("DRP Payment API key belum dikonfigurasi")
	}
	referenceID = strings.TrimSpace(referenceID)
	if referenceID == "" {
		return nil, fmt.Errorf("referenceId kosong")
	}
	u, err := url.Parse(p.BaseURL + "/v2/payment-status")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("referenceId", referenceID)
	u.RawQuery = q.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	httpReq.Header.Set("Accept", "application/json")
	resp, err := p.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("DRP Payment: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DRP Payment %d: %s", resp.StatusCode, drpErrMessage(body))
	}
	var tx drpTransaction
	if err := json.Unmarshal(body, &tx); err != nil {
		return nil, fmt.Errorf("DRP Payment: parse response: %w", err)
	}
	return tx.toIntent(tx.Amount), nil
}

func (p *DRPProvider) Cancel(ctx context.Context, referenceID string) error {
	if p.APIKey == "" {
		return fmt.Errorf("DRP Payment API key belum dikonfigurasi")
	}
	referenceID = strings.TrimSpace(referenceID)
	if referenceID == "" {
		return fmt.Errorf("referenceId kosong")
	}
	payload, err := json.Marshal(map[string]string{"referenceId": referenceID})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v2/qris-cancel", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := p.client().Do(httpReq)
	if err != nil {
		return fmt.Errorf("DRP Payment: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DRP Payment %d: %s", resp.StatusCode, drpErrMessage(body))
	}
	return nil
}

func (p *DRPProvider) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	_ = ctx
	if strings.TrimSpace(p.WebhookSecret) == "" {
		return nil, fmt.Errorf("DRP Payment webhook secret belum dikonfigurasi")
	}
	sig := strings.TrimSpace(headerGet(headers, "X-Signature"))
	token := strings.TrimSpace(headerGet(headers, "X-DRP-Token"))
	if token == "" {
		token = bearerToken(headers)
	}
	okHMAC := sig != "" && hmac.Equal([]byte(sig), []byte(hmacSHA256(p.WebhookSecret, body)))
	okJWT := token != "" && verifyDRPJWT(p.WebhookSecret, token) == nil
	if !okHMAC && !okJWT {
		return nil, fmt.Errorf("invalid DRP Payment webhook signature")
	}
	return parseWebhookBody(ProviderDRP, body)
}

func (tx drpTransaction) toIntent(fallbackAmount int64) *IntentResult {
	status := strings.ToLower(strings.TrimSpace(tx.Status))
	if status == "" {
		status = "pending"
	}
	amount := tx.Amount
	if amount <= 0 {
		amount = fallbackAmount
	}
	payable := tx.TotalAmount
	if payable <= 0 {
		payable = amount + tx.Fee + tx.UniqueDigit
	}
	var exp *time.Time
	if ts := strings.TrimSpace(tx.ExpiresAt); ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			exp = &t
		} else if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			exp = &t
		}
	}
	return &IntentResult{
		ExternalID:    firstNonEmpty(tx.ReferenceID, tx.TransactionID),
		TransactionID: tx.TransactionID,
		QRString:      tx.QRISString,
		QRImageBase64: tx.QRISImageBase64,
		Status:        status,
		ExpiresAt:     exp,
		Amount:        amount,
		PayableAmount: payable,
		UniqueDigit:   tx.UniqueDigit,
		Fee:           tx.Fee,
	}
}

func drpErrMessage(body []byte) string {
	var e drpErrorBody
	if err := json.Unmarshal(body, &e); err == nil {
		msg := strings.TrimSpace(firstNonEmpty(e.Error, e.Message, e.Code))
		if msg != "" {
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

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func verifyDRPJWT(secret, token string) error {
	token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
	if token == "" {
		return fmt.Errorf("empty token")
	}
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return err
	}
	if parsed == nil || !parsed.Valid {
		return fmt.Errorf("invalid token")
	}
	return nil
}
