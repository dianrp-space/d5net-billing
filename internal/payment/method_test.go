package payment

import "testing"

func TestMethodFromProvider(t *testing.T) {
	if got := MethodFromProvider("drp"); got != MethodQRIS {
		t.Fatalf("drp: %q", got)
	}
	if got := MethodFromProvider("qris"); got != MethodQRIS {
		t.Fatalf("qris: %q", got)
	}
	if got := MethodFromProvider("manual"); got != MethodTunai {
		t.Fatalf("manual: %q", got)
	}
	if got := MethodFromProvider(""); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got := MethodFromProvider("duitku"); got != ProviderDuitku {
		t.Fatalf("duitku: %q", got)
	}
	if got := MethodFromProvider("midtrans"); got != "midtrans" {
		t.Fatalf("unknown pg should stay: %q", got)
	}
}
