package monitor

import (
	"context"
	"log/slog"
	"time"

	"github.com/dianrp/drp-billing/internal/auth"
	"github.com/dianrp/drp-billing/internal/notify"
	ros "github.com/dianrp/drp-billing/internal/provision/routeros"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

type Poller struct {
	store    *store.Store
	routeros *ros.Client
	interval time.Duration
	notify   *notify.Service
}

func NewPoller(st *store.Store, enc *auth.Encryptor, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &Poller{store: st, routeros: ros.New(st, enc), interval: interval}
}

func (p *Poller) WithNotify(n *notify.Service) *Poller {
	p.notify = n
	return p
}

func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.PollAll(ctx); err != nil {
				slog.Error("poll all routers", "err", err)
			}
		}
	}
}

func (p *Poller) PollAll(ctx context.Context) error {
	rows, err := p.store.Pool.Query(ctx, `SELECT id, tenant_id FROM routers WHERE is_active = true`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, tenantID xid.ID
		if err := rows.Scan(&id, &tenantID); err != nil {
			continue
		}
		if err := p.PollRouter(ctx, tenantID, id); err != nil {
			slog.Warn("poll router failed", "router_id", id, "err", err)
		}
	}
	return rows.Err()
}

func (p *Poller) PollRouter(ctx context.Context, tenantID xid.ID, routerID xid.ID) error {
	r, err := p.store.GetRouter(ctx, tenantID, routerID)
	if err != nil {
		return err
	}
	if _, err := p.routeros.TestConnection(ctx, routerID); err != nil {
		msg := err.Error()
		_ = p.store.UpdateRouterStatus(ctx, tenantID, routerID, nil, &msg)
		if p.notify != nil {
			_ = p.notify.QueueTenantTelegramOnce(ctx, tenantID, "router_down", notify.OpsDayKey(routerID),
				notify.OpsMsg("router", r.Name, "Tidak merespons poll", msg))
		}
		return err
	}

	sessions, err := p.routeros.ActiveSessions(ctx, routerID)
	activeCount := 0
	if err == nil {
		activeCount = len(sessions)
	}

	var cpuLoad float64
	var memUsed, uptimeSecs int64
	if metrics, err := p.routeros.CollectResourceMetrics(ctx, routerID); err == nil && metrics != nil {
		cpuLoad = metrics.CPULoad
		memUsed = metrics.MemoryUsed
		uptimeSecs = metrics.UptimeSecs
	} else if err != nil {
		slog.Debug("resource metrics unavailable", "router_id", routerID, "err", err)
	}

	_, err = p.store.Pool.Exec(ctx, `
		INSERT INTO router_metrics (tenant_id, router_id, cpu_load, memory_used, uptime, active_sessions, recorded_at)
		VALUES ($1,$2,$3,$4,$5,$6,NOW())
	`, tenantID, routerID, cpuLoad, memUsed, uptimeSecs, activeCount)
	now := time.Now()
	_ = p.store.UpdateRouterStatus(ctx, tenantID, routerID, &now, nil)
	slog.Debug("polled router", "router", r.Name, "sessions", activeCount, "cpu", cpuLoad, "mem", memUsed)
	return err
}
