package store

import "testing"

func TestNotificationItemName(t *testing.T) {
	t.Parallel()
	items := []InvoiceItem{
		{Description: "Tes 3"},
		{Description: "Denda keterlambatan"},
		{Description: " tes 3 "},
	}
	if got := NotificationItemName("Home 20 Mbps", items); got != "Tes 3" {
		t.Fatalf("item name = %q", got)
	}
	if got := NotificationItemName("Home 20 Mbps", nil); got != "Home 20 Mbps" {
		t.Fatalf("plan fallback = %q", got)
	}
	if got := NotificationItemName("", nil); got != "layanan" {
		t.Fatalf("generic fallback = %q", got)
	}
}
