package store

import "testing"

func TestTicketStatusLabel(t *testing.T) {
	if got := TicketStatusLabel("open"); got != "Menunggu" {
		t.Fatalf("open = %q", got)
	}
	if got := TicketStatusLabel("in_progress"); got != "Sedang diproses" {
		t.Fatalf("in_progress = %q", got)
	}
	if !TicketAllowsCustomerReply("open") || !TicketAllowsCustomerReply("in_progress") {
		t.Fatal("open/in_progress should allow reply")
	}
	if TicketAllowsCustomerReply("resolved") || TicketAllowsCustomerReply("closed") {
		t.Fatal("closed tickets should not allow reply")
	}
}
