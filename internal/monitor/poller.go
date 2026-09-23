package monitor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/auth"
	"github.com/dianrp-space/d5net-billing/internal/notify"
	ros "github.com/dianrp-space/d5net-billing/internal/provision/routeros"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

const pollerTick = 15 * time.Second

type Poller struct {
	store    *store.Store
	routeros *ros.Client
	interval time.Duration
	notify   *notify.Service

	lastPollMu sync.Mutex
	lastPollAt map[xid.ID]time.Time
}

func NewPoller(st *store.Store, enc *auth.Encryptor, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = pollerTick
	}
	return &Poller{
		store: st, routeros: ros.New(st, enc), interval: interval,
		lastPollAt: map[xid.ID]time.Time{},
	}
}

func (p *Poller) WithNotify(n *notify.Service) *Poller {
	p.notify = n
	return p
}

func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	_ = p.PollAll(ctx)
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
	intervals := map[xid.ID]time.Duration{}
	now := time.Now()
	for rows.Next() {
		var id, tenantID xid.ID
		if err := rows.Scan(&id, &tenantID); err != nil {
			continue
		}
		interval, ok := intervals[tenantID]
		if !ok {
			interval = defaultPollerInterval()
			if cfg, err := p.store.GetJobScheduleSettings(ctx, tenantID); err == nil {
				interval = time.Duration(cfg.PollerIntervalSeconds) * time.Second
			}
			intervals[tenantID] = interval
		}
		if !p.routerPollDue(id, interval, now) {
			continue
		}
		p.rememberPoll(id, now)
		if err := p.PollRouter(ctx, tenantID, id); err != nil {
			slog.Warn("poll router failed", "router_id", id, "err", err)
		}
	}
	return rows.Err()
}

func defaultPollerInterval() time.Duration {
	return time.Duration(store.DefaultJobScheduleSettings().PollerIntervalSeconds) * time.Second
}

// PollTenantNow mem-poll SEMUA router aktif milik tenant sekaligus, mengabaikan
// interval per-router. Dipakai tombol "Jalankan sekarang" agar sampling sesi /
// metrik / traffic ikut terpicu manual. Interval rutin tidak diganggu: jadwal
// berikutnya tetap dihitung dari lastPollAt normal.
func (p *Poller) PollTenantNow(ctx context.Context, tenantID xid.ID) (polled, failed int) {
	if p == nil {
		return 0, 0
	}
	routers, err := p.store.ListRouters(ctx, tenantID)
	if err != nil {
		return 0, 0
	}
	for _, r := range routers {
		if !r.IsActive {
			continue
		}
		if err := p.PollRouter(ctx, tenantID, r.ID); err != nil {
			failed++
			continue
		}
		polled++
		p.rememberPoll(r.ID, time.Now())
	}
	return polled, failed
}

func (p *Poller) routerPollDue(routerID xid.ID, interval time.Duration, now time.Time) bool {
	p.lastPollMu.Lock()
	defer p.lastPollMu.Unlock()
	last, ok := p.lastPollAt[routerID]
	return pollDue(last, ok, interval, now)
}

func (p *Poller) rememberPoll(routerID xid.ID, at time.Time) {
	p.lastPollMu.Lock()
	defer p.lastPollMu.Unlock()
	if p.lastPollAt == nil {
		p.lastPollAt = map[xid.ID]time.Time{}
	}
	p.lastPollAt[routerID] = at
}

func pollDue(last time.Time, ok bool, interval time.Duration, now time.Time) bool {
	if interval < time.Minute {
		interval = time.Minute
	}
	if !ok {
		return true
	}
	return !now.Before(last.Add(interval))
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
			_ = p.notify.QueueTenantTelegramOnce(ctx, tenantID, "router_down", notify.OpsDayKey(routerID, p.store.TenantNow(ctx, tenantID)),
				notify.OpsMsg("router", r.Name, "Tidak merespons poll", msg))
		}
		if dup, derr := p.store.HasRecentAlert(ctx, tenantID, "router_down", &routerID, 24*time.Hour); derr == nil && !dup {
			et := "router"
			_ = p.store.CreateAlert(ctx, &store.Alert{
				TenantID: tenantID, Severity: "critical", Kind: "router_down",
				Title:    "Router tidak merespons: " + r.Name,
				Message:  msg, EntityType: &et, EntityID: &routerID,
			})
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
	if err == nil {
		p.CollectTraffic(ctx, tenantID, routerID)
	}
	return err
}
