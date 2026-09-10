package payment

import (
	"crypto/rand"
	"strings"
	"time"
)

const merchantOrderIDMaxLen = 50

const orderIDAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func RandomOrderSuffix(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		now := time.Now().UnixNano()
		for i := 0; i < n; i++ {
			b[i] = orderIDAlphabet[int(now>>uint(i*5))%len(orderIDAlphabet)]
		}
		return string(b)
	}
	for i := range b {
		b[i] = orderIDAlphabet[int(b[i])%len(orderIDAlphabet)]
	}
	return string(b)
}

// RefreshMerchantOrderID replaces the random suffix so a PG 409 can retry.
func RefreshMerchantOrderID(id string) string {
	id = strings.TrimSpace(id)
	suffix := RandomOrderSuffix(5)
	if i := strings.LastIndex(id, "-"); i >= 0 && len(id)-i-1 == 5 {
		out := id[:i+1] + suffix
		if len(out) > merchantOrderIDMaxLen {
			out = out[:merchantOrderIDMaxLen]
		}
		return out
	}
	out := strings.Trim(id, "-")
	if out == "" {
		return "INV-retry-" + suffix
	}
	if len(out)+1+5 > merchantOrderIDMaxLen {
		out = out[:merchantOrderIDMaxLen-6]
	}
	return out + "-" + suffix
}
