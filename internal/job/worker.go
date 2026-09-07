package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/dianrp/drp-billing/internal/billing"
	"github.com/dianrp/drp-billing/internal/monitor"
	"github.com/dianrp/drp-billing/internal/notify"
	"github.com/dianrp/drp-billing/internal/provision"
	"github.com/dianrp/drp-billing/internal/provisioner"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

type Worker struct {
	store       *store.Store
	billing     *billing.Engine
	notify      *notify.Service
	poller      *monitor.Poller
	provisioner *provisioner.Registry
}

func NewWorker(st *store.Store, billing *billing.Engine, notify *notify.Service, poller *monitor.Poller, prov *provisioner.Registry) *Worker {
	return &Worker{store: st, billing: billing, notify: notify, poller: poller, provisioner: prov}
}

func (w *Worker) Run(ctx context.Context) {
	go w.poller.Run(ctx)

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runCycle(ctx)
		}
	}
}

func (w *Worker) runCycle(ctx context.Context) {
	tenants, err := w.store.ListTenants(ctx)
	if err != nil {
		slog.Error("list tenants", "err", err)
		return
	}
	now := time.Now()
	for _, t := range tenants {
		if !t.IsActive {
			continue
		}
		if n, err := w.billing.ProcessDueBilling(ctx, t.ID); err == nil && n > 0 {
			slog.Info("generated invoices", "tenant", t.Slug, "count", n)
		}
		if ids, err := w.billing.ProcessOverdueSuspensions(ctx, t.ID); err == nil {
			for _, subID := range ids {
				w.suspendSubscription(ctx, t.ID, subID)
			}
		}
		w.processDunning(ctx, t.ID)
		w.checkODPOutages(ctx, t.ID)
		w.weeklyReconcile(ctx, t.ID, now)
		w.monthlyReportEmail(ctx, t, now)
	}
	if n, err := w.notify.ProcessPending(ctx, 100); err == nil && n > 0 {
		slog.Info("sent notifications", "count", n)
	}
}

func (w *Worker) checkODPOutages(ctx context.Context, tenantID xid.ID) {
	odps, err := w.store.ListODPs(ctx, tenantID)
	if err != nil {
		return
	}
	for _, o := range odps {
		ok, err := w.poller.DetectODPOutage(ctx, tenantID, o.ID, 0.5)
		if err != nil || !ok {
			continue
		}
		et := "odp"
		eid := o.ID
		_ = w.store.CreateAlert(ctx, &store.Alert{
			TenantID: tenantID, Severity: "critical", Kind: "odp_outage",
			Title: fmt.Sprintf("Possible ODP outage: %s", o.Name),
			Message: fmt.Sprintf("Offline ratio on ODP %s (%s) exceeds 50%%", o.Name, o.Code),
			EntityType: &et, EntityID: &eid,
		})
	}
}

func (w *Worker) weeklyReconcile(ctx context.Context, tenantID xid.ID, now time.Time) {
	if now.Weekday() != time.Sunday {
		return
	}
	// Run during the configured hour window (default 3 AM local); ClaimJob ensures once per Sunday.
	if now.Hour() != 3 {
		return
	}
	jobKey := now.Format("2006-01-02")
	ok, err := w.store.ClaimJob(ctx, tenantID, "weekly_reconcile", jobKey)
	if err != nil || !ok {
		return
	}
	routers, err := w.store.ListRouters(ctx, tenantID)
	if err != nil {
		return
	}
	for _, r := range routers {
		if !r.IsActive {
			continue
		}
		prov, err := w.provisioner.Get(r.Provisioner)
		if err != nil {
			continue
		}
		drifts, err := provision.Reconcile(ctx, w.store, prov, tenantID, r.ID, false)
		if err != nil {
			slog.Warn("weekly reconcile failed", "router_id", r.ID, "err", err)
			continue
		}
		if len(drifts) == 0 {
			continue
		}
		et := "router"
		eid := r.ID
		_ = w.store.CreateAlert(ctx, &store.Alert{
			TenantID: tenantID, Severity: "warn", Kind: "reconcile_drift",
			Title: fmt.Sprintf("Router drift: %s", r.Name),
			Message: fmt.Sprintf("%d drift(s) detected on dry-run reconcile for router %s", len(drifts), r.Name),
			EntityType: &et, EntityID: &eid,
		})
	}
}

func (w *Worker) suspendSubscription(ctx context.Context, tenantID xid.ID, subID xid.ID) {
	sub, err := w.store.GetSubscription(ctx, tenantID, subID)
	if err != nil {
		return
	}
	plan, err := w.store.GetPlan(ctx, tenantID, sub.PlanID)
	if err != nil {
		return
	}
	if sub.RouterID == nil {
		return
	}
	r, err := w.store.GetRouter(ctx, tenantID, *sub.RouterID)
	if err != nil {
		return
	}
	prov, err := w.provisioner.Get(r.Provisioner)
	if err != nil {
		return
	}
	isolirCfg, _ := w.store.GetIsolirNetworkSettings(ctx, tenantID)
	_ = w.store.ResolveIsolirPool(ctx, tenantID, &isolirCfg)
	isolir := isolirCfg.ProfileName
	if plan.IsolirProfile != nil && *plan.IsolirProfile != "" {
		isolir = *plan.IsolirProfile
	}
	if isolir == "" {
		isolir = "isolir"
	}
	ten, _ := w.store.GetTenant(ctx, tenantID)
	slug := ""
	if ten != nil {
		slug = ten.Slug
	}
	// Only push Web Proxy/pool infra when this subscription's router matches isolir settings.
	sameRouter := isolirCfg.RouterID != nil && !xid.IsNil(*isolirCfg.RouterID) && *isolirCfg.RouterID == *sub.RouterID
	if ensurer, ok := prov.(provision.IsolirEnsurer); ok && sameRouter && isolirCfg.PoolRanges != "" && isolirCfg.PortalBaseURL != "" {
		if err := ensurer.EnsureIsolirInfra(ctx, tenantID, *sub.RouterID, isolirCfg, slug); err != nil {
			slog.Warn("ensure isolir infra", "router_id", *sub.RouterID, "err", err)
		}
	}
	profile := ""
	if plan.ProfileName != nil {
		profile = *plan.ProfileName
	}
	appName, _ := w.store.EffectiveAppName(ctx, tenantID)
	spec := &provision.ServiceSpec{
		TenantID:       tenantID,
		SubscriptionID: subID,
		RouterID:       *sub.RouterID,
		Username:       sub.Username,
		ServiceType:    sub.ServiceType,
		ProfileName:    profile,
		IsolirProfile:  isolir,
		Comment:        provision.CommentTag(appName, sub.CustomerCode, sub.CustomerName),
	}
	if err := prov.Suspend(ctx, spec); err != nil {
		slog.Error("suspend subscription", "sub_id", subID, "err", err)
	}
}

func (w *Worker) processDunning(ctx context.Context, tenantID xid.ID) {
	invoices, err := w.store.ListDunningInvoices(ctx, tenantID)
	if err != nil {
		return
	}
	for _, inv := range invoices {
		cust, err := w.store.GetCustomer(ctx, tenantID, inv.CustomerID)
		if err != nil {
			continue
		}
		due := inv.DueDate
		now := time.Now()
		dueDay := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, now.Location())
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		days := int(today.Sub(dueDay).Hours() / 24) // negative = before due
		for _, target := range []int{-7, -3, 0, 1, 3} {
			if days != target {
				continue
			}
			jobKey := fmt.Sprintf("inv-%d-day-%d", inv.ID, days)
			ok, err := w.store.ClaimJob(ctx, tenantID, "dunning", jobKey)
			if err != nil || !ok {
				break
			}
			_ = w.notify.SendInvoiceReminder(ctx, tenantID, inv.CustomerID, cust.Phone, inv.InvoiceNumber, inv.TotalAmount, due.Format("02/01/2006"))
			break
		}
	}
}

func (w *Worker) monthlyReportEmail(ctx context.Context, t store.Tenant, now time.Time) {
	if now.Day() != 1 || now.Hour() != 8 {
		return
	}
	ok, err := w.store.ClaimJob(ctx, t.ID, "monthly_report", now.Format("2006-01"))
	if err != nil || !ok {
		return
	}
	stats, _ := w.store.DashboardStats(ctx, t.ID)
	to := ""
	if t.Email != nil {
		to = *t.Email
	}
	if to == "" {
		return
	}
	body := fmt.Sprintf("Laporan %s %s — pelanggan aktif: %v, langganan aktif: %v, tagihan belum lunas: %v, pendapatan bulan ini: %v",
		t.Name, now.Format("January 2006"), stats["active_customers"], stats["active_subscriptions"], stats["unpaid_invoices"], stats["monthly_revenue"])
	_ = w.notify.Queue(ctx, notify.Message{
		TenantID: t.ID, Channel: "email", Recipient: to,
		Subject: "Laporan bisnis bulanan drp-billing", Body: body,
	})
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
