package radius

import "testing"

func TestEncodeDisconnect(t *testing.T) {
	pkt, err := encodeDisconnect("user1", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt) < 20 {
		t.Fatalf("packet too short: %d", len(pkt))
	}
	if pkt[0] != radiusDisconnectRequest {
		t.Fatalf("code=%d", pkt[0])
	}
}
