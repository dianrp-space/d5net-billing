package payment

import (
	"strings"
	"testing"
)

func TestRefreshMerchantOrderID(t *testing.T) {
	orig := "INV-demo-A12-092026-AAAAA"
	got := RefreshMerchantOrderID(orig)
	if !strings.HasPrefix(got, "INV-demo-A12-092026-") {
		t.Fatalf("got %q", got)
	}
	if got == orig {
		t.Fatal("expected new suffix")
	}
	if len(got) != len(orig) {
		t.Fatalf("len %d vs %d", len(got), len(orig))
	}
}
