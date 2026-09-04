package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/dianrp/drp-billing/internal/xid"
)

type IntentRequest struct {
	TenantID   xid.ID
	CustomerID xid.ID
	InvoiceID  xid.ID
	Amount     int64
	ReturnURL  string
}

type IntentResult struct {
	ExternalID  string
	CheckoutURL string
	QRString    string
	Status      string
}

type WebhookEvent struct {
	ExternalID string
	Status     string
	Amount     int64
	Reference  string
	Raw        map[string]any
}

type Provider interface {
	Name() string
	CreateIntent(ctx context.Context, req IntentRequest) (*IntentResult, error)
	VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error)
}

type ManualProvider struct{}

func (m *ManualProvider) Name() string { return "manual" }

func (m *ManualProvider) CreateIntent(ctx context.Context, req IntentRequest) (*IntentResult, error) {
	return &IntentResult{
		ExternalID: fmt.Sprintf("MAN-%s-%s", req.TenantID, req.InvoiceID),
		Status:     "pending",
	}, nil
}

func (m *ManualProvider) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	return nil, fmt.Errorf("manual provider has no webhook")
}

type MidtransProvider struct {
	ServerKey string
	IsProd    bool
}

func (p *MidtransProvider) Name() string { return "midtrans" }

func (p *MidtransProvider) CreateIntent(ctx context.Context, req IntentRequest) (*IntentResult, error) {
	extID := fmt.Sprintf("MT-%s-%s", req.TenantID, req.InvoiceID)
	return &IntentResult{
		ExternalID:  extID,
		CheckoutURL: fmt.Sprintf("https://app.midtrans.com/snap/v1/transactions/%s", extID),
		Status:      "pending",
	}, nil
}

func (p *MidtransProvider) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	sig := headerGet(headers, "X-Signature")
	expected := hmacSHA512(p.ServerKey, body)
	if sig != expected {
		return nil, fmt.Errorf("invalid midtrans signature")
	}
	return parseWebhookBody("midtrans", body)
}

type XenditProvider struct {
	SecretKey string
}

func (p *XenditProvider) Name() string { return "xendit" }

func (p *XenditProvider) CreateIntent(ctx context.Context, req IntentRequest) (*IntentResult, error) {
	extID := fmt.Sprintf("XD-%s-%s", req.TenantID, req.InvoiceID)
	return &IntentResult{
		ExternalID:  extID,
		CheckoutURL: fmt.Sprintf("https://checkout.xendit.co/web/%s", extID),
		Status:      "pending",
	}, nil
}

func (p *XenditProvider) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	token := headerGet(headers, "X-CALLBACK-TOKEN")
	if token != p.SecretKey {
		return nil, fmt.Errorf("invalid xendit token")
	}
	return parseWebhookBody("xendit", body)
}

type TripayProvider struct {
	PrivateKey   string
	MerchantCode string
}

func (p *TripayProvider) Name() string { return "tripay" }

func (p *TripayProvider) CreateIntent(ctx context.Context, req IntentRequest) (*IntentResult, error) {
	extID := fmt.Sprintf("TP-%s-%s", req.TenantID, req.InvoiceID)
	return &IntentResult{
		ExternalID:  extID,
		CheckoutURL: fmt.Sprintf("https://tripay.co.id/checkout/%s", extID),
		Status:      "pending",
	}, nil
}

func (p *TripayProvider) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	sig := headerGet(headers, "X-Callback-Signature")
	expected := hmacSHA256(p.PrivateKey, body)
	if sig != expected {
		return nil, fmt.Errorf("invalid tripay signature")
	}
	return parseWebhookBody("tripay", body)
}

type Registry struct {
	providers map[string]Provider
}

// NewRegistry registers Manual always, and Midtrans/Xendit/Tripay when their keys are non-empty.
func NewRegistry(midtransKey, xenditKey, tripayKey string) *Registry {
	r := &Registry{providers: make(map[string]Provider)}
	r.Register(&ManualProvider{})
	if midtransKey != "" {
		r.Register(&MidtransProvider{ServerKey: midtransKey})
	}
	if xenditKey != "" {
		r.Register(&XenditProvider{SecretKey: xenditKey})
	}
	if tripayKey != "" {
		r.Register(&TripayProvider{PrivateKey: tripayKey})
	}
	return r
}

// NewRegistryFromEnv is an alias for NewRegistry using gateway keys from config/env.
func NewRegistryFromEnv(midtransKey, xenditKey, tripayKey string) *Registry {
	return NewRegistry(midtransKey, xenditKey, tripayKey)
}

func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("payment provider not found: %s", name)
	}
	return p, nil
}

func (r *Registry) Has(name string) bool {
	_, ok := r.providers[name]
	return ok
}

func parseWebhookBody(provider string, body []byte) (*WebhookEvent, error) {
	var m map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &m); err != nil {
			return nil, fmt.Errorf("parse webhook body: %w", err)
		}
	}
	return ParseWebhookEvent(provider, m)
}

func headerGet(headers map[string]string, key string) string {
	if headers == nil {
		return ""
	}
	if v, ok := headers[key]; ok && v != "" {
		return v
	}
	// case-insensitive fallback
	for k, v := range headers {
		if equalFoldASCII(k, key) {
			return v
		}
	}
	return ""
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func hmacSHA256(key string, data []byte) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write(data)
	return hex.EncodeToString(m.Sum(nil))
}

func hmacSHA512(key string, data []byte) string {
	m := hmac.New(sha512.New, []byte(key))
	m.Write(data)
	return hex.EncodeToString(m.Sum(nil))
}
