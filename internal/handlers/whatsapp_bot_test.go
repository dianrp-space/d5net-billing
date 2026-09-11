package handlers

import (
	"testing"
)

func TestBotDeviceAllows(t *testing.T) {
	if !botDeviceAllows("", "62812@s.whatsapp.net", "org_2") {
		t.Fatal("empty bot device should allow all")
	}
	if !botDeviceAllows("org_2", "62812@s.whatsapp.net", "org_2") {
		t.Fatal("session match")
	}
	if !botDeviceAllows("62812@s.whatsapp.net", "62812@s.whatsapp.net", "") {
		t.Fatal("jid match")
	}
	if !botDeviceAllows("62812", "62812@s.whatsapp.net", "") {
		t.Fatal("digits match")
	}
	if botDeviceAllows("org_9", "62812@s.whatsapp.net", "org_2") {
		t.Fatal("different device should be dropped")
	}
	if !botDeviceAllows("org_2", "", "") {
		t.Fatal("no device info should pass (single-number setup)")
	}
}

func TestBotPhoneCandidates(t *testing.T) {
	got := botPhoneCandidates("6281234567890@s.whatsapp.net")
	want := map[string]bool{"6281234567890": true, "081234567890": true}
	for _, c := range got {
		delete(want, c)
	}
	if len(want) != 0 {
		t.Fatalf("missing candidates: %v (got %v)", want, got)
	}
	got = botPhoneCandidates("081234567890")
	found := false
	for _, c := range got {
		if c == "6281234567890" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected normalized 62… in %v", got)
	}
	if botPhoneCandidates("") != nil {
		t.Fatal("empty jid should give no candidates")
	}
}

func TestFormatRupiahID(t *testing.T) {
	if got := formatRupiahID(150000); got != "Rp 150.000" {
		t.Fatalf("got %q", got)
	}
	if got := formatRupiahID(500); got != "Rp 500" {
		t.Fatalf("got %q", got)
	}
	if got := formatRupiahID(0); got != "Rp 0" {
		t.Fatalf("got %q", got)
	}
}

func TestParseGOWAMessage(t *testing.T) {
	in := &whatsappWebhookInput{
		Event:     "message",
		DeviceID:  "628111@s.whatsapp.net",
		SessionID: "org_2",
		Payload: map[string]any{
			"from":       "6281234567890@s.whatsapp.net",
			"chat_id":    "6281234567890@s.whatsapp.net",
			"body":       "  /TAGIHAN ",
			"is_from_me": false,
		},
	}
	got, ok := parseGOWAMessage(in)
	if !ok {
		t.Fatal("expected ok")
	}
	if got.From != "6281234567890@s.whatsapp.net" || got.Body != "/TAGIHAN" {
		t.Fatalf("%+v", got)
	}
	if _, ok := parseGOWAMessage(&whatsappWebhookInput{Event: "message.ack"}); ok {
		t.Fatal("non-message events must be ignored")
	}
	if _, ok := parseGOWAMessage(&whatsappWebhookInput{Event: "message", Payload: nil}); ok {
		t.Fatal("missing payload must be ignored")
	}
}
