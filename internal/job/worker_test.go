package job

import (
	"testing"
	"time"

	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

func TestIsolirInfraCacheSkipsUntilConfigChanges(t *testing.T) {
	w := &Worker{isolirInfraOK: map[string]time.Time{}}
	cfg := store.IsolirNetworkSettings{
		ProfileName:   "isolir",
		PoolRanges:    "10.10.70.0/24",
		PortalBaseURL: "https://billing.example.com",
	}
	k := isolirInfraCacheKey(xid.Nil(), xid.Nil(), cfg, "drpnet")
	if w.isolirInfraFresh(k) {
		t.Fatal("expected cache miss")
	}
	w.rememberIsolirInfra(k)
	if !w.isolirInfraFresh(k) {
		t.Fatal("expected cache hit")
	}
	cfg.PoolRanges = "10.10.71.0/24"
	k2 := isolirInfraCacheKey(xid.Nil(), xid.Nil(), cfg, "drpnet")
	if k2 == k {
		t.Fatal("config change must change cache key")
	}
	if w.isolirInfraFresh(k2) {
		t.Fatal("new config should miss")
	}
}

func TestTenantCycleDueHonorsInterval(t *testing.T) {
	w := &Worker{lastCycleAt: map[xid.ID]time.Time{}}
	id := xid.Nil()
	now := time.Now()
	if !w.tenantCycleDue(id, time.Minute, now) {
		t.Fatal("first run should be due")
	}
	w.rememberTenantCycle(id, now)
	if w.tenantCycleDue(id, 5*time.Minute, now.Add(time.Minute)) {
		t.Fatal("should wait for interval")
	}
	if !w.tenantCycleDue(id, 5*time.Minute, now.Add(5*time.Minute)) {
		t.Fatal("should run when interval elapsed")
	}
}
