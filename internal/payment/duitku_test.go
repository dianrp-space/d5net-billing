package payment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dianrp/drp-billing/internal/xid"
)

func TestDuitkuSignatures(t *testing.T) {
	req := duitkuRequestSignature("D123", "1700000000000", "apikey")
	if len(req) != 64 {
		t.Fatalf("request sig len %d", len(req))
	}
	cb := duitkuCallbackSignature("D123", "15000", "inv-1", "apikey")
	if cb == req {
		t.Fatal("callback signature should differ from request signature")
	}
	if duitkuCallbackSignature("D123", "15000", "inv-1", "apikey") != cb {
		t.Fatal("callback signature not stable")
	}
}

func TestDuitkuVerifyWebhookForm(t *testing.T) {
	p := NewDuitkuProvider("D123", "apikey", true, 60)
	vals := url.Values{
		"merchantCode":    {"D123"},
		"amount":          {"15000"},
		"merchantOrderId": {"inv-abc"},
		"resultCode":      {"00"},
		"reference":       {"REF-9"},
		"signature":       {duitkuCallbackSignature("D123", "15000", "inv-abc", "apikey")},
	}
	ev, err := p.VerifyWebhook(context.Background(), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}, []byte(vals.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalID != "inv-abc" || ev.Reference != "REF-9" || ev.Amount != 15000 || !WebhookIsPaid(ev.Status) {
		t.Fatalf("%+v", ev)
	}
	_, err = p.VerifyWebhook(context.Background(), nil, []byte(vals.Encode()+"x"))
	if err == nil {
		t.Fatal("expected invalid signature")
	}
}

func TestDuitkuCreateIntentPOP(t *testing.T) {
	var gotMerchant, gotSig, gotTS string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/merchant/createInvoice" {
			http.NotFound(w, r)
			return
		}
		gotMerchant = r.Header.Get("x-duitku-merchantcode")
		gotSig = r.Header.Get("x-duitku-signature")
		gotTS = r.Header.Get("x-duitku-timestamp")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"merchantCode":  "D123",
			"reference":     "REF-1",
			"paymentUrl":    "https://app-sandbox.duitku.com/redirect/abc",
			"statusCode":    "00",
			"statusMessage": "SUCCESS",
		})
	}))
	t.Cleanup(srv.Close)

	p := NewDuitkuProvider("D123", "apikey", true, 30)
	p.HTTP = srv.Client()
	p.HTTP.Transport = rewriteHost(srv.URL)

	tid := xid.MustParse("11111111-1111-1111-1111-111111111111")
	iid := xid.MustParse("22222222-2222-2222-2222-222222222222")
	res, err := p.CreateIntent(context.Background(), IntentRequest{
		TenantID:        tid,
		InvoiceID:       iid,
		Amount:          25000,
		Email:           "a@b.id",
		CallbackURL:     "https://billing.example.com/api/webhooks/payment/duitku",
		ReturnURL:       "https://billing.example.com/portal",
		MerchantOrderID: "inv-22222222-2222-2222-2222-222222222222",
		ProductDetails:  "Tagihan INV-1",
		CustomerName:    "Budi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMerchant != "D123" || gotTS == "" || gotSig == "" {
		t.Fatalf("missing POP headers merchant=%q ts=%q sig=%q", gotMerchant, gotTS, gotSig)
	}
	if gotSig != duitkuRequestSignature("D123", gotTS, "apikey") {
		t.Fatal("request signature mismatch")
	}
	if body["merchantOrderId"] != "inv-22222222-2222-2222-2222-222222222222" {
		t.Fatalf("order id %+v", body["merchantOrderId"])
	}
	if body["callbackUrl"] != "https://billing.example.com/api/webhooks/payment/duitku" {
		t.Fatalf("callback %+v", body["callbackUrl"])
	}
	if res.CheckoutURL == "" || res.TransactionID != "REF-1" || res.ExternalID == "" {
		t.Fatalf("%+v", res)
	}
	if res.CheckoutURL != "https://app-sandbox.duitku.com/redirect/abc" {
		t.Fatalf("paymentUrl %q", res.CheckoutURL)
	}
}

type rewriteHost string

func (h rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(string(h))
	if err != nil {
		return nil, err
	}
	req = req.Clone(req.Context())
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	req.Host = u.Host
	return http.DefaultTransport.RoundTrip(req)
}
