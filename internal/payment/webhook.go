package payment

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func ParseWebhookEvent(provider string, body map[string]any) (*WebhookEvent, error) {
	ev := &WebhookEvent{Raw: body, Status: "paid"}
	if body == nil {
		return nil, fmt.Errorf("empty webhook body")
	}

	ev.ExternalID = firstString(body,
		"merchantOrderId", "merchant_order_id", "referenceId", "reference_id", "external_id", "order_id", "merchant_ref", "reference", "id",
	)
	ev.Reference = firstString(body, "transactionId", "transaction_id", "reference", "payment_id", "merchant_ref")

	if v, ok := body["transaction_status"].(string); ok {
		switch strings.ToLower(v) {
		case "settlement", "capture", "paid", "success":
			ev.Status = "paid"
		default:
			ev.Status = v
		}
	}
	if v, ok := body["status"].(string); ok && v != "" {
		switch strings.ToUpper(v) {
		case "PAID", "SUCCESS", "SETTLEMENT", "CAPTURE":
			ev.Status = "paid"
		default:
			ev.Status = v
		}
	}
	if rc := firstString(body, "resultCode", "result_code", "statusCode", "status_code"); rc != "" {
		switch rc {
		case "00":
			ev.Status = "paid"
		case "01":
			ev.Status = "pending"
		case "02":
			ev.Status = "failed"
		}
	}

	ev.Amount = firstAmount(body, "amount", "paidAmount", "paid_amount", "gross_amount", "totalAmount", "total_amount")

	// Nested Tripay-style payload: { "data": { ... } }
	if data, ok := body["data"].(map[string]any); ok {
		if ev.ExternalID == "" {
			ev.ExternalID = firstString(data, "referenceId", "reference_id", "merchant_ref", "reference", "external_id", "order_id")
		}
		if ev.Reference == "" {
			ev.Reference = firstString(data, "transactionId", "transaction_id")
		}
		if ev.Amount == 0 {
			ev.Amount = firstAmount(data, "amount", "paidAmount", "totalAmount", "total_amount", "gross_amount")
		}
		if st, ok := data["status"].(string); ok && st != "" {
			switch strings.ToUpper(st) {
			case "PAID", "SUCCESS":
				ev.Status = "paid"
			default:
				ev.Status = st
			}
		}
	}

	// Nested DOKU-style payload: { "order": { ... } }
	if order, ok := body["order"].(map[string]any); ok {
		if ev.ExternalID == "" {
			ev.ExternalID = firstString(order, "invoice_number", "merchantOrderId", "merchant_order_id", "reference", "external_id", "order_id")
		}
		if ev.Amount == 0 {
			ev.Amount = firstAmount(order, "amount", "total_amount", "gross_amount")
		}
	}

	_ = provider
	return ev, nil
}

func ParseWebhookBodyBytes(contentType string, raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	ct := strings.ToLower(contentType)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct == "application/x-www-form-urlencoded" || (!strings.Contains(ct, "json") && bytesLookLikeForm(raw)) {
		vals, err := url.ParseQuery(string(raw))
		if err == nil && len(vals) > 0 {
			for k, vs := range vals {
				if len(vs) > 0 {
					out[k] = vs[0]
				}
			}
			return out
		}
	}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func bytesLookLikeForm(raw []byte) bool {
	s := strings.TrimSpace(string(raw))
	if s == "" || strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return false
	}
	return strings.Contains(s, "=")
}

func WebhookIsPaid(status string) bool {
	switch strings.ToLower(status) {
	case "paid", "settlement", "capture", "success":
		return true
	default:
		return strings.EqualFold(status, "PAID") || strings.EqualFold(status, "SUCCESS")
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func firstAmount(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if n := toInt64(v); n != 0 {
				return n
			}
		}
	}
	return 0
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case float32:
		return int64(t)
	case string:
		s := strings.TrimSpace(strings.ReplaceAll(t, ",", ""))
		if s == "" {
			return 0
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int64(f)
		}
	}
	return 0
}
