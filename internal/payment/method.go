package payment

import "strings"

const (
	MethodQRIS     = "qris"
	MethodTunai    = "tunai"
	MethodTransfer = "transfer"
)

// MethodFromProvider maps a payment-gateway / kasir id to the method stored on payments.
// Unknown providers are kept as-is so a new PG is not silently recorded as tunai/QRIS.
func MethodFromProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "":
		return ""
	case ProviderManual, MethodTunai, "cash", "kasir":
		return MethodTunai
	case MethodQRIS, "qr":
		return MethodQRIS
	case ProviderDuitku, "duitku_pop", "duitkupop":
		return ProviderDuitku
	case ProviderDoku:
		return ProviderDoku
	case MethodTransfer, "bank", "va":
		return MethodTransfer
	default:
		return p
	}
}

// NormalizeMethod canonicalizes a method string from API or DB.
func NormalizeMethod(method string) string {
	return MethodFromProvider(method)
}
