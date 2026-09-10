package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/golang-jwt/jwt/v5"
)

func TestDRPCreateIntent(t *testing.T) {
	iid := xid.MustParse("22222222-2222-2222-2222-222222222222")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/qris" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer drp_live_test" {
			t.Fatalf("auth header %q", got)
		}
		var req drpCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ReferenceID == "" || req.Amount != 25000 {
			t.Fatalf("bad request %+v", req)
		}
		if req.ExpiresInMinutes != DefaultQRISExpiresMinutes {
			t.Fatalf("ttl %d", req.ExpiresInMinutes)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"transactionId":   "tx-1",
			"referenceId":     req.ReferenceID,
			"status":          "PENDING",
			"amount":          req.Amount,
			"fee":             0,
			"uniqueDigit":     17,
			"totalAmount":     25017,
			"qrisString":      "00020101",
			"qrisImageBase64": "data:image/png;base64,abc",
			"expiresAt":       time.Now().Add(15 * time.Minute).Format(time.RFC3339),
		})
	}))
	t.Cleanup(srv.Close)

	p := NewDRPProvider(srv.URL, "drp_live_test", "whsec")
	res, err := p.CreateIntent(t.Context(), IntentRequest{InvoiceID: iid, Amount: 25000})
	if err != nil {
		t.Fatal(err)
	}
	if res.QRString != "00020101" || res.PayableAmount != 25017 || res.UniqueDigit != 17 {
		t.Fatalf("unexpected result %+v", res)
	}
	if !strings.HasPrefix(res.ExternalID, "drp-") {
		t.Fatalf("external id %q", res.ExternalID)
	}
}

func TestDRPCreateIntentMerchantOrderID(t *testing.T) {
	var gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req drpCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		gotRef = req.ReferenceID
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"transactionId": "tx-1",
			"referenceId":   req.ReferenceID,
			"status":        "PENDING",
			"amount":        req.Amount,
			"qrisString":    "00020101",
			"totalAmount":   req.Amount,
		})
	}))
	t.Cleanup(srv.Close)
	p := NewDRPProvider(srv.URL, "k", "s")
	orderID := "INV-demo-C001-092026-AB3K7"
	res, err := p.CreateIntent(t.Context(), IntentRequest{Amount: 1000, MerchantOrderID: orderID})
	if err != nil {
		t.Fatal(err)
	}
	if gotRef != orderID || res.ExternalID != orderID {
		t.Fatalf("ref %q external %q", gotRef, res.ExternalID)
	}
}

func TestClampQRISExpiresMinutes(t *testing.T) {
	if got := ClampQRISExpiresMinutes(0); got != 15 {
		t.Fatalf("zero = %d", got)
	}
	if got := ClampQRISExpiresMinutes(1440); got != 1440 {
		t.Fatalf("max = %d", got)
	}
	if got := ClampQRISExpiresMinutes(2000); got != 1440 {
		t.Fatalf("over = %d", got)
	}
}

func TestDRPCreateIntentCustomTTL(t *testing.T) {
	iid := xid.MustParse("22222222-2222-2222-2222-222222222222")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req drpCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ExpiresInMinutes != 1440 {
			t.Fatalf("ttl %d", req.ExpiresInMinutes)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"transactionId": "tx-ttl",
			"referenceId":   req.ReferenceID,
			"status":        "PENDING",
			"amount":        req.Amount,
			"qrisString":    "00020101",
			"expiresAt":     time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		})
	}))
	t.Cleanup(srv.Close)
	p := NewDRPProvider(srv.URL, "k", "s")
	p.ExpiresInMinutes = 1440
	if _, err := p.CreateIntent(t.Context(), IntentRequest{InvoiceID: iid, Amount: 1000}); err != nil {
		t.Fatal(err)
	}
}

func TestDRPCreateIntentConflictRetries(t *testing.T) {
	iid := xid.MustParse("22222222-2222-2222-2222-222222222222")
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		body, _ := io.ReadAll(r.Body)
		var req drpCreateRequest
		_ = json.Unmarshal(body, &req)
		if n == 1 {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"already used"}`))
			return
		}
		if !strings.Contains(req.ReferenceID, "-") {
			t.Fatalf("expected suffixed reference, got %s", req.ReferenceID)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"transactionId": "tx-2",
			"referenceId":   req.ReferenceID,
			"status":        "PENDING",
			"amount":        req.Amount,
			"totalAmount":   req.Amount,
			"qrisString":    "qr",
		})
	}))
	t.Cleanup(srv.Close)
	p := NewDRPProvider(srv.URL, "k", "s")
	res, err := p.CreateIntent(t.Context(), IntentRequest{InvoiceID: iid, Amount: 1000})
	if err != nil || res.QRString != "qr" {
		t.Fatal(err, res)
	}
	if n != 2 {
		t.Fatalf("calls=%d", n)
	}
}

func TestDRPVerifyWebhookHMAC(t *testing.T) {
	secret := "whsec"
	body := []byte(`{"referenceId":"drp-1","status":"PAID","amount":25000}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	p := NewDRPProvider("", "k", secret)
	ev, err := p.VerifyWebhook(t.Context(), map[string]string{"X-Signature": sig}, body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalID != "drp-1" || !WebhookIsPaid(ev.Status) {
		t.Fatalf("%+v", ev)
	}
}

func TestDRPVerifyWebhookJWT(t *testing.T) {
	secret := "whsec"
	body := []byte(`{"referenceId":"drp-2","status":"PAID","amount":1000}`)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	})
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	p := NewDRPProvider("", "k", secret)
	ev, err := p.VerifyWebhook(t.Context(), map[string]string{"X-DRP-Token": signed}, body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalID != "drp-2" {
		t.Fatalf("%+v", ev)
	}
}

func TestDRPVerifyWebhookRejectsBadSig(t *testing.T) {
	p := NewDRPProvider("", "k", "whsec")
	_, err := p.VerifyWebhook(t.Context(), map[string]string{"X-Signature": "deadbeef"}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDRPCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/qris-cancel" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req struct {
			ReferenceID string `json:"referenceId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ReferenceID != "drp-1" {
			t.Fatalf("ref %q", req.ReferenceID)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"CANCELLED"}`))
	}))
	t.Cleanup(srv.Close)
	p := NewDRPProvider(srv.URL, "k", "s")
	if err := p.Cancel(t.Context(), "drp-1"); err != nil {
		t.Fatal(err)
	}
}
