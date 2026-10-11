package notify

import (
	"strings"
	"testing"
)

func TestTemplateCatalogHasItemName(t *testing.T) {
	t.Parallel()
	found := 0
	for _, ev := range TemplateCatalog() {
		if ev.Event != "invoice_issued" && ev.Event != "invoice_generated" && ev.Event != "invoice_reminder" && ev.Event != "payment_confirmation" {
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
	if found != 4 {
		t.Fatalf("expected 4 events, got %d", found)
	}
}

func TestServiceIsolirTemplate(t *testing.T) {
	t.Parallel()
	var ev TemplateEvent
	found := false
	for _, item := range TemplateCatalog() {
		if item.Event == "service_isolir" {
			ev = item
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing service_isolir")
	}
	if len(ev.Channels) != 1 || ev.Channels[0] != "whatsapp" {
		t.Fatalf("channels = %#v", ev.Channels)
	}
	body := applyVars(ev.DefaultBody, map[string]string{
		"customer_name": "Budi",
		"plan_name":     "Home 20",
		"username":      "budi01",
	})
	if strings.Contains(body, "{{") {
		t.Fatalf("unrendered body: %s", body)
	}
	if !strings.Contains(body, "budi01") || !strings.Contains(body, "diisolir") {
		t.Fatalf("body = %s", body)
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
