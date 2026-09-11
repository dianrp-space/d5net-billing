package radius

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"github.com/dianrp-space/d5net-billing/internal/xid"
	"net"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/provision"
)

const (
	radiusDisconnectRequest = 40
	attrUserName            = 1
	coaPort                 = "3799"
)

func (c *Client) Disconnect(ctx context.Context, spec *provision.ServiceSpec) error {
	if xid.IsNil(spec.RouterID) {
		return fmt.Errorf("router_id required for RADIUS Disconnect/CoA")
	}
	r, err := c.store.GetRouter(ctx, spec.TenantID, spec.RouterID)
	if err != nil {
		return err
	}
	secret := "testing123"
	if r.Username != "" {
		secret = r.Username // often NAS secret is configured separately; fallback to stored user as shared secret hint
	}
	addr := net.JoinHostPort(r.Address, coaPort)
	pkt, err := encodeDisconnect(spec.Username, []byte(secret))
	if err != nil {
		return err
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return fmt.Errorf("coa dial %s: %w", addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write(pkt); err != nil {
		return fmt.Errorf("coa send: %w", err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		// Many NAS ignore CoA or firewalled — still count as attempted.
		return fmt.Errorf("coa no ACK from %s: %w", addr, err)
	}
	if n < 1 {
		return fmt.Errorf("coa empty reply")
	}
	return nil
}

func encodeDisconnect(username string, secret []byte) ([]byte, error) {
	attrs := []byte{attrUserName, byte(len(username) + 2)}
	attrs = append(attrs, []byte(username)...)
	length := uint16(20 + len(attrs))
	buf := bytes.NewBuffer(nil)
	buf.WriteByte(radiusDisconnectRequest)
	buf.WriteByte(1) // identifier
	_ = binary.Write(buf, binary.BigEndian, length)
	auth := make([]byte, 16)
	buf.Write(auth)
	buf.Write(attrs)
	pkt := buf.Bytes()
	h := md5.New()
	h.Write(pkt[:4])
	h.Write(make([]byte, 16))
	h.Write(pkt[20:])
	h.Write(secret)
	copy(pkt[4:20], h.Sum(nil))
	return pkt, nil
}
