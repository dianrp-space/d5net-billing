package payment

import (
	"testing"

	"github.com/dianrp/drp-billing/internal/xid"
)

func TestWebhookIsPaid(t *testing.T) {
	if !WebhookIsPaid("settlement") || !WebhookIsPaid("paid") {
		t.Fatal("expected paid")
	}
	if WebhookIsPaid("pending") {
		t.Fatal("pending is not paid")
	}
}

func TestParseWebhookEventIdempotent(t *testing.T) {
	body := map[string]any{"external_id": "INV-1", "status": "paid"}
	a, err := ParseWebhookEvent("midtrans", body)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ParseWebhookEvent("midtrans", body)
	if a.ExternalID != b.ExternalID || a.Status != b.Status {
		t.Fatal("not idempotent")
	}
}

func TestHMAC512UsesSHA512(t *testing.T) {
	got := hmacSHA512("secret", []byte("body"))
	if len(got) != 128 {
		t.Fatalf("sha512 hex should be 128 chars, got %d", len(got))
	}
}

func TestManualProvider(t *testing.T) {
	p := &ManualProvider{}
	tid := xid.MustParse("11111111-1111-1111-1111-111111111111")
	iid := xid.MustParse("22222222-2222-2222-2222-222222222222")
	res, err := p.CreateIntent(t.Context(), IntentRequest{TenantID: tid, InvoiceID: iid, Amount: 1000})
	if err != nil || res.ExternalID == "" {
		t.Fatal(err, res)
	}
}
