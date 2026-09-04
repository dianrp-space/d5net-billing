package routeros

import (
	"github.com/dianrp/drp-billing/internal/xid"
	"sync"
	"time"
)

type breaker struct {
	failures  int
	openUntil time.Time
	mu        sync.Mutex
}

func (c *Client) allow(routerID xid.ID) bool {
	v, _ := c.breakers.LoadOrStore(routerID, &breaker{})
	b := v.(*breaker)
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Now().Before(b.openUntil) {
		return false
	}
	return true
}

func (c *Client) success(routerID xid.ID) {
	v, ok := c.breakers.Load(routerID)
	if !ok {
		return
	}
	b := v.(*breaker)
	b.mu.Lock()
	b.failures = 0
	b.openUntil = time.Time{}
	b.mu.Unlock()
}

func (c *Client) fail(routerID xid.ID) {
	v, _ := c.breakers.LoadOrStore(routerID, &breaker{})
	b := v.(*breaker)
	b.mu.Lock()
	b.failures++
	if b.failures >= 3 {
		b.openUntil = time.Now().Add(30 * time.Second)
	}
	b.mu.Unlock()
}
