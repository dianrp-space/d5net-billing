package payment

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Direct API (BI SNAP) untuk QRIS: generate QR, query status, webhook notify.
// Auth: B2B access token (RSA-SHA256 clientId|timestamp) + symmetric
// HMAC-SHA512 per request.

const (
	DokuTokenPath      = "/authorization/v1/access-token/b2b"
	DokuQRGeneratePath = "/snap-adapter/b2b/v1.0/qr/qr-mpm-generate"
	DokuQRQueryPath    = "/snap-adapter/b2b/v1.0/qr/qr-mpm-query"
	DokuQRServiceCode  = "47"
	DokuQRChannelID    = "H2H"
)

var dokuWIB = time.FixedZone("WIB", 7*3600)

// dokuSnapTimestamp formats local (+07:00) time for SNAP headers.
func dokuSnapTimestamp(t time.Time) string {
	return t.In(dokuWIB).Format("2006-01-02T15:04:05+07:00")
}

func dokuParseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	pemStr = strings.TrimSpace(pemStr)
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("private key PEM tidak valid")
	}
	if block.Type == "PRIVATE KEY" {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		if k, ok := key.(*rsa.PrivateKey); ok {
			return k, nil
		}
		return nil, fmt.Errorf("bukan RSA private key")
	}
	if block.Type == "RSA PRIVATE KEY" {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	return nil, fmt.Errorf("tipe private key tidak didukung: %s", block.Type)
}

func dokuRSASign(key *rsa.PrivateKey, s string) (string, error) {
	sum := sha256.Sum256([]byte(s))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// dokuSymmetricSignature builds X-SIGNATURE for SNAP transaction calls.
func dokuSymmetricSignature(secret, method, endpoint, token string, body []byte, timestamp string) string {
	sum := sha256.Sum256(body)
	hexHash := strings.ToLower(hex.EncodeToString(sum[:]))
	raw := method + ":" + endpoint + ":" + token + ":" + hexHash + ":" + timestamp
	m := hmac.New(sha512.New, []byte(secret))
	m.Write([]byte(raw))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

type dokuCachedToken struct {
	token     string
	expiresAt time.Time
}

var dokuTokenCache sync.Map // key: sandbox\x00clientID -> dokuCachedToken

func dokuTokenCacheKey(sandbox bool, clientID string) string {
	if sandbox {
		return "1\x00" + clientID
	}
	return "0\x00" + clientID
}

func dokuInvalidateToken(sandbox bool, clientID string) {
	dokuTokenCache.Delete(dokuTokenCacheKey(sandbox, clientID))
}

// dokuAccessToken returns a cached B2B token or fetches a new one.
func dokuAccessToken(ctx context.Context, p *DokuProvider) (string, error) {
	if p.ClientID == "" {
		return "", fmt.Errorf("DOKU client ID kosong")
	}
	key := dokuTokenCacheKey(p.Sandbox, p.ClientID)
	if v, ok := dokuTokenCache.Load(key); ok {
		if ct, ok := v.(dokuCachedToken); ok && ct.token != "" && time.Now().Before(ct.expiresAt) {
			return ct.token, nil
		}
	}
	tok, expiresIn, err := dokuFetchToken(ctx, p)
	if err != nil {
		return "", err
	}
	ttl := time.Duration(expiresIn) * time.Second
	if ttl <= 0 || ttl > 55*time.Minute {
		ttl = 25 * time.Minute
	} else {
		ttl -= 60 * time.Second
		if ttl < 60*time.Second {
			ttl = 60 * time.Second
		}
	}
	dokuTokenCache.Store(key, dokuCachedToken{token: tok, expiresAt: time.Now().Add(ttl)})
	return tok, nil
}

func dokuFetchToken(ctx context.Context, p *DokuProvider) (string, int64, error) {
	if p.ClientID == "" || strings.TrimSpace(p.PrivateKey) == "" {
		return "", 0, fmt.Errorf("DOKU client ID / private key belum dikonfigurasi")
	}
	key, err := dokuParseRSAPrivateKey(p.PrivateKey)
	if err != nil {
		return "", 0, fmt.Errorf("DOKU private key: %w", err)
	}
	timestamp := dokuSnapTimestamp(time.Now())
	sig, err := dokuRSASign(key, p.ClientID+"|"+timestamp)
	if err != nil {
		return "", 0, fmt.Errorf("DOKU sign token: %w", err)
	}
	payload, _ := json.Marshal(map[string]string{"grantType": "client_credentials"})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL()+DokuTokenPath, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-CLIENT-KEY", p.ClientID)
	httpReq.Header.Set("X-SIGNATURE", sig)
	resp, err := p.client().Do(httpReq)
	if err != nil {
		return "", 0, fmt.Errorf("DOKU token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("DOKU token %d: %s", resp.StatusCode, dokuErrMessage(body))
	}
	var out struct {
		AccessToken string `json:"accessToken"`
		AccessToken2 string `json:"access_token"`
		ExpiresIn   any    `json:"expiresIn"`
		ExpiresIn2  any    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", 0, fmt.Errorf("DOKU token: parse response: %w", err)
	}
	tok := firstNonEmpty(out.AccessToken, out.AccessToken2)
	if tok == "" {
		return "", 0, fmt.Errorf("DOKU token kosong")
	}
	return tok, toInt64(firstNonEmpty(fmt.Sprint(out.ExpiresIn), fmt.Sprint(out.ExpiresIn2))), nil
}

func dokuExternalID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func dokuSnapHeaders(p *DokuProvider, token, timestamp string) map[string]string {
	return map[string]string{
		"X-PARTNER-ID":  p.ClientID,
		"X-EXTERNAL-ID": dokuExternalID(),
		"X-TIMESTAMP":   timestamp,
		"CHANNEL-ID":    DokuQRChannelID,
		"Authorization": "Bearer " + token,
	}
}

func dokuSnapDo(ctx context.Context, p *DokuProvider, path string, payload map[string]any) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	call := func(token string) (map[string]any, int, error) {
		timestamp := dokuSnapTimestamp(time.Now())
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL()+path, bytes.NewReader(body))
		if err != nil {
			return nil, 0, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		for k, v := range dokuSnapHeaders(p, token, timestamp) {
			httpReq.Header.Set(k, v)
		}
		httpReq.Header.Set("X-SIGNATURE", dokuSymmetricSignature(p.SecretKey, http.MethodPost, path, token, body, timestamp))
		resp, err := p.client().Do(httpReq)
		if err != nil {
			return nil, 0, err
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var m map[string]any
		if jerr := json.Unmarshal(raw, &m); jerr != nil {
			if resp.StatusCode != http.StatusOK {
				return nil, 0, fmt.Errorf("DOKU %d: %s", resp.StatusCode, dokuErrMessage(raw))
			}
			return nil, 0, fmt.Errorf("DOKU: parse response: %w", jerr)
		}
		return m, resp.StatusCode, nil
	}
	token, err := dokuAccessToken(ctx, p)
	if err != nil {
		return nil, err
	}
	m, code, err := call(token)
	if code == http.StatusUnauthorized {
		dokuInvalidateToken(p.Sandbox, p.ClientID)
		token, err = dokuAccessToken(ctx, p)
		if err != nil {
			return nil, err
		}
		m, code, err = call(token)
	}
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("DOKU %d: %v", code, m["responseMessage"])
	}
	return m, nil
}

// DokuQR is a generated QRIS code ready to render/send.
type DokuQR struct {
	Content   string
	Reference string // DOKU referenceNo
	ExpiresAt *time.Time
}

// DokuQRGenerator is implemented by providers that can mint QRIS codes directly.
type DokuQRGenerator interface {
	GenerateQR(ctx context.Context, orderRef string, amount int64) (*DokuQR, error)
}

// GenerateQR mints a dynamic QRIS via Direct API (SNAP MPM).
func (p *DokuProvider) GenerateQR(ctx context.Context, orderRef string, amount int64) (*DokuQR, error) {
	if p.ClientID == "" || p.SecretKey == "" || strings.TrimSpace(p.PrivateKey) == "" {
		return nil, fmt.Errorf("kredensial DOKU (client ID / secret / private key) belum lengkap")
	}
	if strings.TrimSpace(p.MerchantID) == "" || strings.TrimSpace(p.TerminalID) == "" || strings.TrimSpace(p.PostalCode) == "" {
		return nil, fmt.Errorf("merchant ID / terminal ID / postal code DOKU belum diisi di Integrasi")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("nominal pembayaran harus > 0")
	}
	orderRef = strings.TrimSpace(orderRef)
	if orderRef == "" {
		return nil, fmt.Errorf("order ref kosong")
	}
	expiry := ClampDokuExpiryMinutes(p.ExpiresInMinutes)
	validity := time.Now().Add(time.Duration(expiry) * time.Minute).In(dokuWIB).Format("2006-01-02T15:04:05+07:00")
	m, err := dokuSnapDo(ctx, p, DokuQRGeneratePath, map[string]any{
		"partnerReferenceNo": orderRef,
		"amount":             map[string]any{"value": fmt.Sprintf("%d.00", amount), "currency": "IDR"},
		"merchantId":         strings.TrimSpace(p.MerchantID),
		"terminalId":         strings.TrimSpace(p.TerminalID),
		"validityPeriod":     validity,
		"additionalInfo":     map[string]any{"postalCode": strings.TrimSpace(p.PostalCode), "feeType": 1},
	})
	if err != nil {
		return nil, err
	}
	content := firstString(m, "qrContent")
	if content == "" {
		return nil, fmt.Errorf("DOKU: qrContent kosong (%v)", m["responseMessage"])
	}
	exp := time.Now().Add(time.Duration(expiry) * time.Minute)
	return &DokuQR{
		Content:   content,
		Reference: firstString(m, "referenceNo"),
		ExpiresAt: &exp,
	}, nil
}

// QueryQR checks a QRIS status via Direct API. Needs DOKU's referenceNo.
func (p *DokuProvider) QueryQR(ctx context.Context, dokuRef, partnerRef string) (string, int64, error) {
	m, err := dokuSnapDo(ctx, p, DokuQRQueryPath, map[string]any{
		"originalReferenceNo":        strings.TrimSpace(dokuRef),
		"originalPartnerReferenceNo": strings.TrimSpace(partnerRef),
		"serviceCode":                DokuQRServiceCode,
		"merchantId":                 strings.TrimSpace(p.MerchantID),
	})
	if err != nil {
		return "", 0, err
	}
	st := firstString(m, "latestTransactionStatus")
	var amt int64
	if a, ok := m["amount"].(map[string]any); ok {
		amt = toInt64(a["value"])
	}
	switch strings.TrimSpace(st) {
	case "00":
		return "paid", amt, nil
	case "03":
		return "pending", amt, nil
	case "04":
		return "failed", amt, nil
	case "05":
		return "cancelled", amt, nil
	case "06":
		return "failed", amt, nil
	default:
		if st == "" {
			return "pending", amt, nil
		}
		return "pending", amt, nil
	}
}

// dokuVerifySnapWebhook handles an incoming SNAP QR notification.
// Trust anchor is active confirmation: the claimed paid status is only
// returned after our own authenticated query reports paid, so spoofed
// notifications can never complete an invoice.
func (p *DokuProvider) dokuVerifySnapWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	sig := strings.TrimSpace(headerGet(headers, "X-SIGNATURE"))
	timestamp := strings.TrimSpace(headerGet(headers, "X-TIMESTAMP"))
	if sig == "" || timestamp == "" {
		return nil, fmt.Errorf("header notifikasi DOKU tidak lengkap")
	}
	var m map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &m); err != nil {
			return nil, fmt.Errorf("parse webhook body: %w", err)
		}
	}
	partnerRef := firstString(m, "originalPartnerReferenceNo", "partnerReferenceNo")
	if partnerRef == "" {
		return nil, fmt.Errorf("partner reference tidak ditemukan di notifikasi")
	}
	st := firstString(m, "latestTransactionStatus")
	var amt int64
	if a, ok := m["amount"].(map[string]any); ok {
		amt = toInt64(a["value"])
	}
	paid := strings.TrimSpace(st) == "00"
	if paid {
		// Active confirmation: only trust after DOKU itself reports paid.
		dokuRef := firstString(m, "originalReferenceNo", "referenceNo")
		if qs, _, qerr := p.QueryQR(ctx, dokuRef, partnerRef); qerr != nil || qs != "paid" {
			if qerr != nil {
				return &WebhookEvent{ExternalID: partnerRef, Status: "pending", Amount: amt, Raw: m}, nil
			}
			return &WebhookEvent{ExternalID: partnerRef, Status: qs, Amount: amt, Reference: dokuRef, Raw: m}, nil
		}
	}
	return &WebhookEvent{
		ExternalID: partnerRef,
		Status:     snapQRStatus(st),
		Amount:     amt,
		Reference:  firstString(m, "originalReferenceNo", "referenceNo"),
		Raw:        m,
	}, nil
}

func snapQRStatus(st string) string {
	switch strings.TrimSpace(st) {
	case "00":
		return "paid"
	case "03":
		return "pending"
	case "04":
		return "failed"
	case "05":
		return "cancelled"
	case "06":
		return "failed"
	default:
		return "pending"
	}
}
