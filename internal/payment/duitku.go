package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const ProviderDuitku = "duitku"

const (
	DuitkuSandboxBaseURL    = "https://api-sandbox.duitku.com"
	DuitkuProductionBaseURL = "https://api-prod.duitku.com"
	DefaultDuitkuExpiryMin  = 60
	MaxDuitkuExpiryMinutes  = 1440
)

func ClampDuitkuExpiryMinutes(n int) int {
	if n < 1 {
		return DefaultDuitkuExpiryMin
	}
	if n > MaxDuitkuExpiryMinutes {
		return MaxDuitkuExpiryMinutes
	}
	return n
}

func DuitkuBaseURL(sandbox bool) string {
	if sandbox {
		return DuitkuSandboxBaseURL
	}
	return DuitkuProductionBaseURL
}

type DuitkuProvider struct {
	MerchantCode     string
	APIKey           string
	Sandbox          bool
	ExpiresInMinutes int
	HTTP             *http.Client
}

func NewDuitkuProvider(merchantCode, apiKey string, sandbox bool, expiryMin int) *DuitkuProvider {
	return &DuitkuProvider{
		MerchantCode:     strings.TrimSpace(merchantCode),
		APIKey:           strings.TrimSpace(apiKey),
		Sandbox:          sandbox,
		ExpiresInMinutes: ClampDuitkuExpiryMinutes(expiryMin),
		HTTP:             &http.Client{Timeout: 25 * time.Second},
	}
}

func (p *DuitkuProvider) Name() string { return ProviderDuitku }

func (p *DuitkuProvider) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return &http.Client{Timeout: 25 * time.Second}
}

func (p *DuitkuProvider) baseURL() string {
	return DuitkuBaseURL(p.Sandbox)
}

func duitkuTimestampMS() string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10)
}

func duitkuRequestSignature(merchantCode, timestamp, apiKey string) string {
	return hmacSHA256(apiKey, []byte(merchantCode+timestamp))
}

func duitkuCallbackSignature(merchantCode, amount, merchantOrderID, apiKey string) string {
	return hmacSHA256(apiKey, []byte(merchantCode+amount+merchantOrderID))
}

func (p *DuitkuProvider) CreateIntent(ctx context.Context, req IntentRequest) (*IntentResult, error) {
	if p.MerchantCode == "" || p.APIKey == "" {
		return nil, fmt.Errorf("Duitku POP belum dikonfigurasi")
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("nominal pembayaran harus > 0")
	}
	orderID := strings.TrimSpace(req.MerchantOrderID)
	if orderID == "" {
		orderID = fmt.Sprintf("inv-%s", req.InvoiceID)
	}
	if len(orderID) > 50 {
		orderID = orderID[:50]
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = "noreply@localhost"
	}
	callback := strings.TrimSpace(req.CallbackURL)
	if callback == "" {
		return nil, fmt.Errorf("callback URL Duitku kosong")
	}
	returnURL := strings.TrimSpace(req.ReturnURL)
	if returnURL == "" {
		returnURL = callback
	}
	product := strings.TrimSpace(req.ProductDetails)
	if product == "" {
		product = "Pembayaran tagihan"
	}
	name := strings.TrimSpace(req.CustomerName)
	if name == "" {
		name = "Pelanggan"
	}
	payload, err := json.Marshal(map[string]any{
		"paymentAmount":   req.Amount,
		"merchantOrderId": orderID,
		"productDetails":  product,
		"email":           email,
		"phoneNumber":     strings.TrimSpace(req.Phone),
		"customerVaName":  truncateRunes(name, 20),
		"callbackUrl":     callback,
		"returnUrl":       returnURL,
		"expiryPeriod":    ClampDuitkuExpiryMinutes(p.ExpiresInMinutes),
		"itemDetails": []map[string]any{{
			"name":     truncateRunes(product, 50),
			"price":    req.Amount,
			"quantity": 1,
		}},
	})
	if err != nil {
		return nil, err
	}
	ts := duitkuTimestampMS()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL()+"/api/merchant/createInvoice", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-duitku-merchantcode", p.MerchantCode)
	httpReq.Header.Set("x-duitku-timestamp", ts)
	httpReq.Header.Set("x-duitku-signature", duitkuRequestSignature(p.MerchantCode, ts, p.APIKey))

	resp, err := p.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Duitku POP: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Duitku POP %d: %s", resp.StatusCode, duitkuErrMessage(body))
	}
	var out struct {
		MerchantCode  string `json:"merchantCode"`
		Reference     string `json:"reference"`
		PaymentURL    string `json:"paymentUrl"`
		StatusCode    string `json:"statusCode"`
		StatusMessage string `json:"statusMessage"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("Duitku POP: parse response: %w", err)
	}
	if out.StatusCode != "" && out.StatusCode != "00" {
		return nil, fmt.Errorf("Duitku POP: %s", firstNonEmpty(out.StatusMessage, out.StatusCode))
	}
	if strings.TrimSpace(out.PaymentURL) == "" {
		return nil, fmt.Errorf("Duitku POP: paymentUrl kosong")
	}
	exp := time.Now().Add(time.Duration(ClampDuitkuExpiryMinutes(p.ExpiresInMinutes)) * time.Minute)
	return &IntentResult{
		ExternalID:    orderID,
		TransactionID: strings.TrimSpace(out.Reference),
		CheckoutURL:   strings.TrimSpace(out.PaymentURL),
		Status:        "pending",
		ExpiresAt:     &exp,
		Amount:        req.Amount,
		PayableAmount: req.Amount,
		Metadata: map[string]any{
			// Reference + environment untuk korelasi/dukungan (portal membuka
			// paymentUrl sebagai halaman penuh, bukan popup checkout.process).
			"reference":      strings.TrimSpace(out.Reference),
			"duitku_sandbox": p.Sandbox,
		},
	}, nil
}

func (p *DuitkuProvider) CheckStatus(ctx context.Context, referenceID string) (*IntentResult, error) {
	referenceID = strings.TrimSpace(referenceID)
	if referenceID == "" {
		return nil, fmt.Errorf("merchantOrderId kosong")
	}
	if p.MerchantCode == "" || p.APIKey == "" {
		return nil, fmt.Errorf("Duitku POP belum dikonfigurasi")
	}
	payload, err := json.Marshal(map[string]string{"merchantOrderId": referenceID})
	if err != nil {
		return nil, err
	}
	ts := duitkuTimestampMS()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL()+"/api/merchant/transactionStatus", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-duitku-merchantcode", p.MerchantCode)
	httpReq.Header.Set("x-duitku-timestamp", ts)
	httpReq.Header.Set("x-duitku-signature", duitkuRequestSignature(p.MerchantCode, ts, p.APIKey))
	resp, err := p.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Duitku POP: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Duitku POP %d: %s", resp.StatusCode, duitkuErrMessage(body))
	}
	var out struct {
		MerchantOrderID string `json:"merchantOrderId"`
		Reference       string `json:"reference"`
		Amount          any    `json:"amount"`
		StatusCode      string `json:"statusCode"`
		StatusMessage   string `json:"statusMessage"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("Duitku POP: parse status: %w", err)
	}
	status := "pending"
	switch strings.TrimSpace(out.StatusCode) {
	case "00":
		status = "paid"
	case "01":
		status = "pending"
	case "02":
		status = "failed"
	}
	return &IntentResult{
		ExternalID:    firstNonEmpty(out.MerchantOrderID, referenceID),
		TransactionID: strings.TrimSpace(out.Reference),
		Status:        status,
		Amount:        toInt64(out.Amount),
	}, nil
}

func (p *DuitkuProvider) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	_ = ctx
	_ = headers
	fields := ParseWebhookBodyBytes(headerGet(headers, "Content-Type"), body)
	merchantCode := firstString(fields, "merchantCode", "merchant_code")
	amount := webhookAmountString(fields)
	orderID := firstString(fields, "merchantOrderId", "merchant_order_id")
	sig := firstString(fields, "signature")
	if merchantCode == "" || amount == "" || orderID == "" || sig == "" {
		return nil, fmt.Errorf("callback Duitku tidak lengkap")
	}
	want := duitkuCallbackSignature(merchantCode, amount, orderID, p.APIKey)
	if !hmac.Equal([]byte(strings.ToLower(sig)), []byte(strings.ToLower(want))) {
		return nil, fmt.Errorf("invalid Duitku callback signature")
	}
	ev, err := ParseWebhookEvent(ProviderDuitku, fields)
	if err != nil {
		return nil, err
	}
	ev.ExternalID = orderID
	ev.Reference = firstNonEmpty(firstString(fields, "reference"), ev.Reference)
	ev.Amount = toInt64(amount)
	return ev, nil
}

func webhookAmountString(m map[string]any) string {
	for _, k := range []string{"amount", "paymentAmount"} {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if s := strings.TrimSpace(t); s != "" {
					return s
				}
			default:
				if n := toInt64(v); n != 0 {
					return strconv.FormatInt(n, 10)
				}
			}
		}
	}
	return ""
}

func duitkuErrMessage(body []byte) string {
	var e struct {
		StatusMessage string `json:"statusMessage"`
		Message       string `json:"Message"`
		Error         string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil {
		if msg := strings.TrimSpace(firstNonEmpty(e.StatusMessage, e.Message, e.Error)); msg != "" {
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

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}
