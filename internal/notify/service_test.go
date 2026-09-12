package notify

import "testing"

func TestTemplateCatalogHasItemName(t *testing.T) {
	t.Parallel()
	found := 0
	for _, ev := range TemplateCatalog() {
		if ev.Event != "invoice_issued" && ev.Event != "invoice_reminder" && ev.Event != "payment_confirmation" {
			continue
		}
		ok := false
		for _, v := range ev.Variables {
			if v.Name == "item_name" {
				ok = true
				break
			}
		}
		if !ok {
			t.Fatalf("%s missing item_name", ev.Event)
		}
		if applyVars(ev.DefaultBody, map[string]string{"item_name": "Tes 3"}) == ev.DefaultBody {
			t.Fatalf("%s default body does not use item_name", ev.Event)
		}
		found++
	}
	if found != 3 {
		t.Fatalf("expected 3 events, got %d", found)
	}
}

func TestNotificationItemNameFallback(t *testing.T) {
	t.Parallel()
	if got := notificationItemName("Home 20 Mbps", "Tes 3"); got != "Tes 3" {
		t.Fatalf("got %q", got)
	}
	if got := notificationItemName("Home 20 Mbps", "  "); got != "Home 20 Mbps" {
		t.Fatalf("plan fallback = %q", got)
	}
	if got := notificationItemName("", ""); got != "layanan" {
		t.Fatalf("generic fallback = %q", got)
	}
}
