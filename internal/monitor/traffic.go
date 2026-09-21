package monitor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// CollectTraffic membaca counter PPPoE router dan mengakumulasikannya ke
// pemakaian bulanan tiap pelanggan. Dipanggil sekali per poll router yang
// berhasil; gagal diam-diam (best-effort) agar tidak mengganggu monitoring.
func (p *Poller) CollectTraffic(ctx context.Context, tenantID, routerID xid.ID) {
	counters, err := p.routeros.PPPCounters(ctx, tenantID, routerID)
	if err != nil {
		slog.Debug("traffic collect skipped", "router_id", routerID, "err", err)
		return
	}
	now := time.Now()
	for username, c := range counters {
		subID, customerID, err := p.store.SubscriptionByRouterUsername(ctx, tenantID, routerID, username)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				slog.Debug("traffic map skipped", "router_id", routerID, "username", username, "err", err)
			}
			continue
		}
		if err := p.store.AccumulateTrafficSample(ctx, tenantID, routerID, customerID, subID, username, c.RxBytes, c.TxBytes, now); err != nil {
			slog.Debug("traffic accumulate skipped", "router_id", routerID, "username", username, "err", err)
		}
	}
}
