package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dianrp/drp-billing/internal/auth"
	ros "github.com/dianrp/drp-billing/internal/provision/routeros"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

type Poller struct {
	store    *store.Store
	routeros *ros.Client
	interval time.Duration
}

func NewPoller(st *store.Store, enc *auth.Encryptor, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &Poller{store: st, routeros: ros.New(st, enc), interval: interval}
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
	tenantsSeen := map[xid.ID]struct{}{}
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
		tenantsSeen[tenantID] = struct{}{}
		if err := p.PollRouter(ctx, tenantID, id); err != nil {
			slog.Warn("poll router failed", "router_id", id, "err", err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Also include tenants that may only have ODPs (no active routers).
	tenants, err := p.store.ListTenants(ctx)
	if err == nil {
		for _, t := range tenants {
			if t.IsActive {
				tenantsSeen[t.ID] = struct{}{}
			}
		}
	}

	for tenantID := range tenantsSeen {
		p.checkODPOutages(ctx, tenantID)
	}
	return nil
}

func (p *Poller) checkODPOutages(ctx context.Context, tenantID xid.ID) {
	odps, err := p.store.ListODPs(ctx, tenantID)
	if err != nil {
		return
	}
	for _, o := range odps {
		ok, err := p.DetectODPOutage(ctx, tenantID, o.ID, 0.5)
		if err != nil || !ok {
			continue
		}
		et := "odp"
		eid := o.ID
		_ = p.store.CreateAlert(ctx, &store.Alert{
			TenantID: tenantID, Severity: "critical", Kind: "odp_outage",
			Title:      fmt.Sprintf("Possible ODP outage: %s", o.Name),
			Message:    fmt.Sprintf("Offline ratio on ODP %s (%s) exceeds 50%%", o.Name, o.Code),
			EntityType: &et, EntityID: &eid,
		})
	}
}

func (p *Poller) PollRouter(ctx context.Context, tenantID xid.ID, routerID xid.ID) error {
	r, err := p.store.GetRouter(ctx, tenantID, routerID)
	if err != nil {
		return err
	}
	if _, err := p.routeros.TestConnection(ctx, routerID); err != nil {
		msg := err.Error()
		_ = p.store.UpdateRouterStatus(ctx, tenantID, routerID, nil, &msg)
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

func (p *Poller) DetectODPOutage(ctx context.Context, tenantID xid.ID, odpID xid.ID, threshold float64) (bool, error) {
	var total, offline int64
	err := p.store.Pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE s.status IN ('suspended','overdue','offline'))
		FROM odp_ports op
		LEFT JOIN subscriptions s ON s.id = op.subscription_id
		WHERE op.odp_id = $1 AND op.tenant_id = $2 AND op.status = 'used'
	`, odpID, tenantID).Scan(&total, &offline)
	if err != nil || total == 0 {
		return false, err
	}
	ratio := float64(offline) / float64(total)
	if ratio >= threshold {
		var odpName string
		_ = p.store.Pool.QueryRow(ctx, `SELECT name FROM odps WHERE id = $1`, odpID).Scan(&odpName)
		slog.Warn("possible ODP outage", "odp", odpName, "offline_ratio", ratio)
		return true, nil
	}
	return false, nil
}
