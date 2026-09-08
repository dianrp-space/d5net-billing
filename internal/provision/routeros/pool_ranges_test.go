package routeros

import "testing"

func TestHostIP(t *testing.T) {
	if got := HostIP("10.10.0.5/32"); got != "10.10.0.5" {
		t.Fatalf("got %q", got)
	}
	if got := HostIP("10.10.0.1"); got != "10.10.0.1" {
		t.Fatalf("got %q", got)
	}
}

func TestCIDRToPoolRanges(t *testing.T) {
	gw := "10.10.0.1"
	got, err := CIDRToPoolRanges("10.10.0.0/24", &gw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "10.10.0.2-10.10.0.254" {
		t.Fatalf("got %q", got)
	}

	// Isolir sync often has CIDR without a stored gateway — still skip .1 (router).
	got, err = CIDRToPoolRanges("10.10.0.0/24", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "10.10.0.2-10.10.0.254" {
		t.Fatalf("nil gateway /24: got %q", got)
	}

	// PostgreSQL inet often returns gateway with /32
	gw32 := "10.10.0.1/32"
	got, err = CIDRToPoolRanges("10.10.0.0/24", &gw32)
	if err != nil {
		t.Fatal(err)
	}
	if got != "10.10.0.2-10.10.0.254" {
		t.Fatalf("gateway /32: got %q", got)
	}

	got, err = CIDRToPoolRanges("192.168.1.0/30", nil)
	if err != nil {
		t.Fatal(err)
	}
	// network .0, .1 = router, .2 = remote, broadcast .3
	if got != "192.168.1.2" {
		t.Fatalf("got %q", got)
	}
}

func TestCIDRLocalAddress(t *testing.T) {
	if got := CIDRLocalAddress("10.10.0.0/24", nil); got != "10.10.0.1" {
		t.Fatalf("got %q", got)
	}
	gw := "10.99.0.1"
	if got := CIDRLocalAddress("10.99.0.0/24", &gw); got != "10.99.0.1" {
		t.Fatalf("explicit = %q", got)
	}
}
