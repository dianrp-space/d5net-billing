package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

func (p *DokuProvider) createRetailIntent(ctx context.Context, req IntentRequest, ch DokuChannel) (*IntentResult, error) {
	orderID := dokuOrderID(req)
	name := strings.TrimSpace(req.CustomerName)
	if name == "" {
		name = "Pelanggan"
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = "noreply@localhost"
	}
	expiry := ClampDokuExpiryMinutes(p.ExpiresInMinutes)
	path := DokuAlfaPath
	payload := map[string]any{
		"order": map[string]any{
			"invoice_number": orderID,
			"amount":         req.Amount,
		},
		"online_to_offline_info": map[string]any{
			"expired_time":    expiry,
			"reusable_status": false,
			"info":            truncateRunes(firstNonEmpty(req.ProductDetails, "Tagihan"), 32),
		},
		"customer": map[string]any{
			"name":  truncateRunes(name, 64),
			"email": email,
		},
	}
	if ch.ID == "retail_indomaret" {
		path = DokuIndomaretPath
		payload["indomaret_info"] = map[string]any{
			"receipt": map[string]any{
				"description":    truncateRunes(firstNonEmpty(req.ProductDetails, "Pembayaran tagihan"), 128),
				"footer_message": "Terima kasih",
			},
		}
	}
	if cb := strings.TrimSpace(req.CallbackURL); cb != "" {
		payload["additional_info"] = map[string]any{"override_notification_url": cb}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	requestID := xid.New().String()
	timestamp := dokuTimestamp(time.Now())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL()+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Client-Id", p.ClientID)
	httpReq.Header.Set("Request-Id", requestID)
	httpReq.Header.Set("Request-Timestamp", timestamp)
	httpReq.Header.Set("Signature", dokuSignature(p.SecretKey, p.ClientID, requestID, timestamp, path, body))

	resp, err := p.client().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("DOKU retail: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DOKU retail %d: %s", resp.StatusCode, dokuErrMessage(raw))
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("DOKU retail: parse: %w", err)
	}
	oto, _ := m["online_to_offline_info"].(map[string]any)
	code := firstString(oto, "payment_code")
	if code == "" {
		return nil, fmt.Errorf("DOKU retail: payment_code kosong")
	}
	howTo := firstString(oto, "how_to_pay_page")
	exp := dokuExpiredDate(firstString(oto, "expired_date"), expiry)
	return &IntentResult{
		ExternalID:    orderID,
		CheckoutURL:   howTo,
		Status:        "pending",
		ExpiresAt:     exp,
		Amount:        req.Amount,
		PayableAmount: req.Amount,
		Metadata: map[string]any{
			"doku_sandbox": p.Sandbox,
			"doku_kind":    DokuKindRetail,
			"doku_channel": ch.ID,
			"payment_code": code,
			"retail_label": ch.Label,
			"how_to_pay":   howTo,
		},
	}, nil
}
