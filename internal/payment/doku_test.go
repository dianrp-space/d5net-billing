package payment

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testRSAPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestDokuCreateIntentRequiresChannel(t *testing.T) {
	p := NewDokuProvider("MCH-TEST-1", "s3cr3t", "", "", "", "", true, 60)
	_, err := p.CreateIntent(t.Context(), IntentRequest{Amount: 150000, MerchantOrderID: "INV-1"})
	if err == nil || !strings.Contains(err.Error(), "channel") {
		t.Fatalf("err = %v", err)
	}
}

func TestDokuCreateQRIntent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/authorization/v1/access-token/b2b", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"tok-test","expiresIn":"900"}`))
	})
	mux.HandleFunc("/snap-adapter/b2b/v1.0/qr/qr-mpm-generate", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"responseCode":"2004700","qrContent":"00020101021226650016ID.CO.DOKU.WWW011893600","referenceNo":"REF-QR-1"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	p := NewDokuProvider("MCH-TEST-1", "s3cr3t", testRSAPrivateKey(t), "MALL1", "T001", "28111", true, 60)
	p.baseOverride = srv.URL
	res, err := p.CreateIntent(t.Context(), IntentRequest{
		Amount: 150000, MerchantOrderID: "INV-QR-1", Channel: "qris",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.QRString == "" || res.QRImageBase64 == "" {
		t.Fatalf("missing QR: %+v", res)
	}
	if res.Metadata["doku_kind"] != DokuKindQR {
		t.Fatalf("meta = %v", res.Metadata)
	}
}

func TestDokuCreateRetailAlfamart(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"order":{"invoice_number":"INV-R1"},"online_to_offline_info":{"payment_code":"6059000000000205","expired_date":"20260913120000"}}`))
	}))
	defer srv.Close()
	p := NewDokuProvider("MCH-TEST-1", "s3cr3t", "", "", "", "", true, 60)
	p.baseOverride = srv.URL
	res, err := p.CreateIntent(t.Context(), IntentRequest{
		Amount: 50000, MerchantOrderID: "INV-R1", Channel: "retail_alfamart", CustomerName: "Budi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != DokuAlfaPath {
		t.Fatalf("path = %s", gotPath)
	}
	if res.Metadata["payment_code"] != "6059000000000205" {
		t.Fatalf("meta = %v", res.Metadata)
	}
}

func TestDokuCreateVARequiresBIN(t *testing.T) {
	p := NewDokuProvider("MCH-TEST-1", "s3cr3t", "", "", "", "", true, 60)
	_, err := p.CreateIntent(t.Context(), IntentRequest{Amount: 100, MerchantOrderID: "INV-1", Channel: "va_bca"})
	if err == nil || !strings.Contains(err.Error(), "BIN") {
		t.Fatalf("err = %v", err)
	}
}

func TestDokuVerifyWebhook(t *testing.T) {
	secret := "whsec-doku"
	body := []byte(`{"order":{"invoice_number":"INV-1","amount":150000},"transaction":{"status":"SUCCESS","date":"2026-09-11T02:00:00Z","original_request_id":"req-1"}}`)
	headers := map[string]string{
		"Client-Id":         "MCH-TEST-1",
		"Request-Id":        "rid-1",
		"Request-Timestamp": "2026-09-11T02:00:10Z",
	}
	headers["Signature"] = dokuSignature(secret, headers["Client-Id"], headers["Request-Id"], headers["Request-Timestamp"], DokuWebhookPath, body)

	p := NewDokuProvider("MCH-TEST-1", secret, "", "", "", "", false, 60)
	ev, err := p.VerifyWebhook(t.Context(), headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalID != "INV-1" || !WebhookIsPaid(ev.Status) || ev.Amount != 150000 {
		t.Fatalf("%+v", ev)
	}
	headers["Signature"] = "HMACSHA256=tampered"
	if _, err := p.VerifyWebhook(t.Context(), headers, body); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestDokuGenerateQR(t *testing.T) {
	var gotToken, gotGenHeaders http.Header
	var gotGenBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/authorization/v1/access-token/b2b", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-CLIENT-KEY") != "MCH-QR-2" {
			t.Errorf("client key = %q", r.Header.Get("X-CLIENT-KEY"))
		}
		if sig := r.Header.Get("X-SIGNATURE"); sig == "" {
			t.Error("missing token signature")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"responseCode":"2007300","accessToken":"tok-abc","tokenType":"Bearer","expiresIn":900}`))
	})
	mux.HandleFunc("/snap-adapter/b2b/v1.0/qr/qr-mpm-generate", func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Clone()
		gotGenHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotGenBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"responseCode":"2004700","referenceNo":"DOKU-QR-1","partnerReferenceNo":"INV-9","qrContent":"00020101021226660018ID.CO.EXAMPLE0118936009140000000001021500000000000000005204541153031505802ID5914MERCHANT 16003JKT6105123406215051100000208230001123456786304ABCD","additionalInfo":{}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewDokuProvider("MCH-QR-2", "s3cr3t", testRSAPrivateKey(t), "MALL-1", "T001", "28111", true, 30)
	p.baseOverride = srv.URL
	qr, err := p.GenerateQR(t.Context(), "INV-9", 75000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(qr.Content, "00020101") || qr.Reference != "DOKU-QR-1" {
		t.Fatalf("%+v", qr)
	}
	if gotToken.Get("Authorization") != "Bearer tok-abc" {
		t.Fatalf("auth = %q", gotToken.Get("Authorization"))
	}
	if gotGenHeaders.Get("X-PARTNER-ID") != "MCH-QR-2" || gotGenHeaders.Get("CHANNEL-ID") != "H2H" {
		t.Fatalf("headers = %v", gotGenHeaders)
	}
	if gotGenHeaders.Get("X-SIGNATURE") == "" || gotGenHeaders.Get("X-EXTERNAL-ID") == "" {
		t.Fatal("missing snap headers")
	}
	genBodyOrder, _ := gotGenBody["order"], gotGenBody["partnerReferenceNo"]
	_ = genBodyOrder
	if gotGenBody["partnerReferenceNo"] != "INV-9" {
		t.Fatalf("body = %v", gotGenBody)
	}
	amt, _ := gotGenBody["amount"].(map[string]any)
	if amt["value"] != "75000.00" || amt["currency"] != "IDR" {
		t.Fatalf("amount = %v", amt)
	}
	add, _ := gotGenBody["additionalInfo"].(map[string]any)
	if add["postalCode"] != "28111" {
		t.Fatalf("additionalInfo = %v", add)
	}
}

func TestDokuVerifySnapWebhook(t *testing.T) {
	var queried bool
	mux := http.NewServeMux()
	mux.HandleFunc("/authorization/v1/access-token/b2b", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"accessToken":"tok-snap","expiresIn":900}`))
	})
	mux.HandleFunc("/snap-adapter/b2b/v1.0/qr/qr-mpm-query", func(w http.ResponseWriter, r *http.Request) {
		queried = true
		_, _ = w.Write([]byte(`{"responseCode":"2004700","originalPartnerReferenceNo":"INV-7","latestTransactionStatus":"00","amount":{"value":"100000.00","currency":"IDR"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := NewDokuProvider("MCH-QR-3", "s3cr3t", testRSAPrivateKey(t), "MALL-1", "T001", "28111", true, 30)
	p.baseOverride = srv.URL
	body := []byte(`{"originalPartnerReferenceNo":"INV-7","originalReferenceNo":"DOKU-QR-7","latestTransactionStatus":"00","amount":{"value":"100000.00","currency":"IDR"}}`)
	headers := map[string]string{
		"X-PARTNER-ID": "MCH-QR-3",
		"X-TIMESTAMP":  "2026-09-11T09:00:00+07:00",
		"X-SIGNATURE":  "anything-here-trust-comes-from-query",
	}
	ev, err := p.VerifyWebhook(t.Context(), headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalID != "INV-7" || !WebhookIsPaid(ev.Status) || ev.Amount != 100000 {
		t.Fatalf("%+v", ev)
	}
	if !queried {
		t.Fatal("expected active query confirmation")
	}
}

func TestDokuExpiredDate(t *testing.T) {
	if got := dokuExpiredDate("20260911235959", 60); got == nil {
		t.Fatal("expected parsed expiry")
	} else if got.Format("20060102150405") != "20260911235959" {
		t.Fatalf("got %v", got)
	}
	if got := dokuExpiredDate("bogus", 60); got == nil {
		t.Fatal("expected fallback expiry")
	}
}
