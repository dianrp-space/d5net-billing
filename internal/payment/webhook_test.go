package payment

import (
	"testing"

	"github.com/dianrp-space/d5net-billing/internal/xid"
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

func TestParseWebhookBodyBytesForm(t *testing.T) {
	raw := []byte("merchantCode=D123&amount=15000&merchantOrderId=inv-1&signature=abc&resultCode=00")
	got := ParseWebhookBodyBytes("application/x-www-form-urlencoded", raw)
	if got["merchantOrderId"] != "inv-1" || got["amount"] != "15000" || got["resultCode"] != "00" {
		t.Fatalf("%+v", got)
	}
	ev, err := ParseWebhookEvent("duitku", got)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalID != "inv-1" || ev.Amount != 15000 || !WebhookIsPaid(ev.Status) {
		t.Fatalf("%+v", ev)
	}
}

func TestParseWebhookBodyBytesJSON(t *testing.T) {
	raw := []byte(`{"referenceId":"ref-1","status":"PAID","amount":1000}`)
	got := ParseWebhookBodyBytes("application/json", raw)
	if got["referenceId"] != "ref-1" {
		t.Fatalf("%+v", got)
	}
}

func TestHMAC256HexLength(t *testing.T) {
	got := hmacSHA256("secret", []byte("body"))
	if len(got) != 64 {
		t.Fatalf("sha256 hex should be 64 chars, got %d", len(got))
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

func TestParseWebhookEventDokuNested(t *testing.T) {
	body := map[string]any{
		"order":       map[string]any{"invoice_number": "INV-D5N-1", "amount": float64(150000)},
		"transaction": map[string]any{"status": "SUCCESS", "original_request_id": "req-7"},
	}
	ev, err := ParseWebhookEvent("doku", body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalID != "INV-D5N-1" {
		t.Fatalf("external_id = %q", ev.ExternalID)
	}
	if ev.Status != "paid" {
		t.Fatalf("status = %q", ev.Status)
	}
	if ev.Amount != 150000 {
		t.Fatalf("amount = %d", ev.Amount)
	}
	if ev.Reference != "req-7" {
		t.Fatalf("reference = %q", ev.Reference)
	}
}
