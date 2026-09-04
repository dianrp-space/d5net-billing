package routeros

import (
	"net"
	"strconv"
	"testing"
)

func TestJoinHostPort(t *testing.T) {
	addr := net.JoinHostPort("router.example.com", strconv.Itoa(8728))
	if addr != "router.example.com:8728" {
		t.Fatal(addr)
	}
	addr = net.JoinHostPort("10.0.0.1", strconv.Itoa(8729))
	if addr != "10.0.0.1:8729" {
		t.Fatal(addr)
	}
	addr = net.JoinHostPort("2001:db8::1", "8728")
	if addr != "[2001:db8::1]:8728" {
		t.Fatal(addr)
	}
}
