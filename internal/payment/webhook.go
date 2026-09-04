package payment

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseWebhookEvent(provider string, body map[string]any) (*WebhookEvent, error) {
	ev := &WebhookEvent{Raw: body, Status: "paid"}
	if body == nil {
		return nil, fmt.Errorf("empty webhook body")
	}

	ev.ExternalID = firstString(body,
		"external_id", "order_id", "merchant_ref", "reference", "id",
	)
	ev.Reference = firstString(body, "reference", "payment_id", "transaction_id", "merchant_ref")

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

	ev.Amount = firstAmount(body, "amount", "gross_amount", "paid_amount", "total_amount")

	// Nested Tripay-style payload: { "data": { ... } }
	if data, ok := body["data"].(map[string]any); ok {
		if ev.ExternalID == "" {
			ev.ExternalID = firstString(data, "merchant_ref", "reference", "external_id", "order_id")
		}
		if ev.Amount == 0 {
			ev.Amount = firstAmount(data, "amount", "total_amount", "gross_amount")
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

	_ = provider
	return ev, nil
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
