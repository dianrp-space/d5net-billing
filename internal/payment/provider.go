package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
)

const ProviderManual = "manual"

type IntentRequest struct {
	TenantID        xid.ID
	CustomerID      xid.ID
	InvoiceID       xid.ID
	Amount          int64
	ReturnURL       string
	CallbackURL     string
	MerchantOrderID string
	Email           string
	Phone           string
	CustomerName    string
	ProductDetails  string
}

type IntentResult struct {
	ExternalID    string
	TransactionID string
	CheckoutURL   string
	QRString      string
	QRImageBase64 string
	Status        string
	ExpiresAt     *time.Time
	Amount        int64
	PayableAmount int64
	UniqueDigit   int64
	Fee           int64
	// Metadata carries provider-specific extras persisted on the intent and
	// surfaced to the client (e.g. Duitku popup environment).
	Metadata map[string]any
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

// StatusChecker is implemented by providers that can poll remote payment status.
type StatusChecker interface {
	CheckStatus(ctx context.Context, referenceID string) (*IntentResult, error)
}

// Canceller is implemented by providers that can void a pending intent.
type Canceller interface {
	Cancel(ctx context.Context, referenceID string) error
}

type ManualProvider struct{}

func (m *ManualProvider) Name() string { return ProviderManual }

func (m *ManualProvider) CreateIntent(ctx context.Context, req IntentRequest) (*IntentResult, error) {
	return &IntentResult{
		ExternalID: fmt.Sprintf("MAN-%s-%s", req.TenantID, req.InvoiceID),
		Status:     "pending",
		Amount:     req.Amount,
	}, nil
}

func (m *ManualProvider) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	return nil, fmt.Errorf("manual provider has no webhook")
}

type Registry struct {
	providers map[string]Provider
}

// NewRegistry registers the manual provider. Online gateways (Duitku) are
// resolved per provider from the tenant's integration settings at call time.
func NewRegistry() *Registry {
	r := &Registry{providers: make(map[string]Provider)}
	r.Register(&ManualProvider{})
	return r
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

func bearerToken(headers map[string]string) string {
	raw := headerGet(headers, "Authorization")
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		return strings.TrimSpace(raw[7:])
	}
	return raw
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
