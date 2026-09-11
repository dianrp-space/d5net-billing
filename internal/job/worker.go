package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/billing"
	"github.com/dianrp-space/d5net-billing/internal/monitor"
	"github.com/dianrp-space/d5net-billing/internal/notify"
	"github.com/dianrp-space/d5net-billing/internal/provision"
	"github.com/dianrp-space/d5net-billing/internal/provision/routeros"
	"github.com/dianrp-space/d5net-billing/internal/provisioner"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

type Worker struct {
	store       *store.Store
	billing     *billing.Engine
	notify      *notify.Service
	poller      *monitor.Poller
	provisioner *provisioner.Registry

	isolirInfraMu sync.Mutex
	isolirInfraOK map[string]time.Time

	cycleMu     sync.Mutex
	lastCycleAt map[xid.ID]time.Time

	repairMu     sync.Mutex
	lastRepairAt map[xid.ID]time.Time
}

func NewWorker(st *store.Store, billing *billing.Engine, notify *notify.Service, poller *monitor.Poller, prov *provisioner.Registry) *Worker {
	return &Worker{
		store: st, billing: billing, notify: notify, poller: poller, provisioner: prov,
		isolirInfraOK: map[string]time.Time{},
		lastCycleAt:   map[xid.ID]time.Time{},
		lastRepairAt:  map[xid.ID]time.Time{},
	}
}

func (w *Worker) Run(ctx context.Context) {
	go w.poller.Run(ctx)

	ticker := time.NewTicker(15 * time.Second)
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
	notifyBatch := 100
	for _, t := range tenants {
		if !t.IsActive {
			continue
		}
		cfg, err := w.store.GetJobScheduleSettings(ctx, t.ID)
		if err != nil {
			slog.Warn("job schedule settings", "tenant", t.Slug, "err", err)
			cfg = store.DefaultJobScheduleSettings()
		}
		if cfg.NotifyBatchSize > notifyBatch {
			notifyBatch = cfg.NotifyBatchSize
		}
		interval := time.Duration(cfg.CycleIntervalSeconds) * time.Second
		if interval < time.Minute {
			interval = time.Minute
		}
		if !w.tenantCycleDue(t.ID, interval, now) {
			continue
		}
		w.rememberTenantCycle(t.ID, now)
		res := w.runTenantJobs(ctx, t, cfg, now)
		if res.Invoices > 0 {
			slog.Info("generated invoices", "tenant", t.Slug, "count", res.Invoices)
		}
	}
	if n, err := w.notify.ProcessPending(ctx, notifyBatch); err == nil && n > 0 {
		slog.Info("sent notifications", "count", n)
	}
}

// TenantCycleResult is a short summary after one tenant job cycle.
type TenantCycleResult struct {
	Invoices int `json:"invoices"`
	Isolir   int `json:"isolir"`
	Notify   int `json:"notify"`
}

// RunTenantNow runs billing / isolir / dunning (and due scheduled jobs) immediately,
// ignoring the configured cycle interval.
func (w *Worker) RunTenantNow(ctx context.Context, tenantID xid.ID) (TenantCycleResult, error) {
	var out TenantCycleResult
	if w == nil {
		return out, fmt.Errorf("worker tidak tersedia")
	}
	t, err := w.store.GetTenant(ctx, tenantID)
	if err != nil {
		return out, err
	}
	if t == nil || !t.IsActive {
		return out, fmt.Errorf("tenant nonaktif")
	}
	cfg, err := w.store.GetJobScheduleSettings(ctx, t.ID)
	if err != nil {
		cfg = store.DefaultJobScheduleSettings()
	}
	now := time.Now()
	w.rememberTenantCycle(t.ID, now)
	out = w.runTenantJobs(ctx, *t, cfg, now)
	batch := cfg.NotifyBatchSize
	if batch < 1 {
		batch = 100
	}
	if n, err := w.notify.ProcessPending(ctx, batch); err == nil {
		out.Notify = n
	}
	return out, nil
}

func (w *Worker) runTenantJobs(ctx context.Context, t store.Tenant, cfg store.JobScheduleSettings, now time.Time) TenantCycleResult {
	var out TenantCycleResult
	if cfg.BillingEnabled {
		if n, err := w.billing.ProcessDueBilling(ctx, t.ID); err == nil {
			out.Invoices = n
		}
	}
	if cfg.IsolirEnabled {
		overdue := map[xid.ID]struct{}{}
		if ids, err := w.billing.ProcessOverdueSuspensions(ctx, t.ID); err == nil {
			out.Isolir = len(ids)
			for _, subID := range ids {
				overdue[subID] = struct{}{}
				w.suspendSubscription(ctx, t.ID, subID)
			}
		}
		tried := w.resumePaidSubscriptions(ctx, t.ID)
		w.repairIsolirSecrets(ctx, t.ID, overdue, tried)
	}
	if cfg.DunningEnabled {
		w.processDunning(ctx, t.ID, cfg.DunningOffsets)
	}
	if cfg.WeeklyReconcileEnabled {
		w.weeklyReconcile(ctx, t.ID, now, cfg.WeeklyReconcileWeekday, cfg.WeeklyReconcileHour)
	}
	if cfg.MonthlyReportEnabled {
		w.monthlyReportEmail(ctx, t, now, cfg.MonthlyReportDay, cfg.MonthlyReportHour)
	}
	return out
}

func (w *Worker) weeklyReconcile(ctx context.Context, tenantID xid.ID, now time.Time, weekday, hour int) {
	if int(now.Weekday()) != weekday {
		return
	}
	if now.Hour() != hour {
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
			Title:      fmt.Sprintf("Router drift: %s", r.Name),
			Message:    fmt.Sprintf("%d drift(s) detected on dry-run reconcile for router %s", len(drifts), r.Name),
			EntityType: &et, EntityID: &eid,
		})
		_ = w.notify.QueueTenantTelegramOnce(ctx, tenantID, "reconcile_drift", notify.OpsWeekKey(r.ID, now),
			notify.OpsMsg("reconcile", r.Name,
				fmt.Sprintf("%d drift terdeteksi (dry-run)", len(drifts)),
			))
	}
}

func (w *Worker) suspendSubscription(ctx context.Context, tenantID xid.ID, subID xid.ID) {
	sub, err := w.store.GetSubscription(ctx, tenantID, subID)
	if err != nil {
		return
	}
	first := sub.Status != "suspended"
	if first {
		if err := w.store.UpdateSubscriptionStatus(ctx, tenantID, subID, "suspended"); err != nil {
			slog.Warn("mark subscription suspended", "sub_id", subID, "err", err)
			return
		}
		sub.Status = "suspended"
	}
	who := strings.TrimSpace(sub.CustomerName)
	if who == "" {
		who = sub.Username
	}
	if sub.CustomerCode != "" {
		who = fmt.Sprintf("%s (%s)", who, sub.CustomerCode)
	}

	provisionNote := "status isolir di billing; router belum di-set"
	if sub.RouterID != nil {
		if note := w.applyIsolirOnRouter(ctx, tenantID, sub); note != "" {
			provisionNote = note
		}
	}
	if !first {
		return
	}
	_ = w.notify.QueueTenantTelegram(ctx, tenantID, notify.OpsMsg("isolir", who,
		"User: "+sub.Username,
		"Paket: "+sub.PlanName,
		provisionNote,
	))
}

func (w *Worker) applyIsolirOnRouter(ctx context.Context, tenantID xid.ID, sub *store.Subscription) string {
	if sub.RouterID == nil {
		return "router belum di-set"
	}
	plan, err := w.store.GetPlan(ctx, tenantID, sub.PlanID)
	if err != nil {
		return "gagal baca paket"
	}
	r, err := w.store.GetRouter(ctx, tenantID, *sub.RouterID)
	if err != nil {
		return "router tidak ditemukan"
	}
	prov, err := w.provisioner.Get(r.Provisioner)
	if err != nil {
		return "provisioner tidak tersedia"
	}
	isolirCfg, _ := w.store.GetIsolirNetworkSettings(ctx, tenantID)
	_ = w.store.ResolveIsolirPool(ctx, tenantID, &isolirCfg)
	isolir := store.IsolirProfileName(isolirCfg, plan)
	isolirCfg.ProfileName = isolir
	ten, _ := w.store.GetTenant(ctx, tenantID)
	slug := ""
	if ten != nil {
		slug = ten.Slug
	}
	infraKey := isolirInfraCacheKey(tenantID, *sub.RouterID, isolirCfg, slug)
	if ensurer, ok := prov.(provision.IsolirEnsurer); ok && isolirCfg.PoolRanges != "" && isolirCfg.PortalBaseURL != "" {
		if !w.isolirInfraFresh(infraKey) {
			if err := ensurer.EnsureIsolirInfra(ctx, tenantID, *sub.RouterID, isolirCfg, slug); err != nil {
				slog.Warn("ensure isolir infra", "router_id", *sub.RouterID, "err", err)
			} else {
				w.rememberIsolirInfra(infraKey)
			}
		}
	}
	profile := planRouterProfile(plan)
	if getter, ok := prov.(provision.SecretGetter); ok {
		st := sub.ServiceType
		if strings.TrimSpace(st) == "" {
			st = "pppoe"
		}
		if sec, err := getter.GetServiceSecret(ctx, tenantID, *sub.RouterID, sub.Username, st); err == nil && sec != nil {
			if provision.SecretLooksIsolir(sec.Profile, sec.Comment, isolir, profile) {
				return "profil isolir sudah di " + r.Name
			}
		}
	}
	appName, _ := w.store.EffectiveAppName(ctx, tenantID)
	local := strings.TrimSpace(isolirCfg.PoolGateway)
	if local == "" && isolirCfg.PoolRanges != "" {
		local = routeros.CIDRLocalAddress(isolirCfg.PoolRanges, nil)
	}
	spec := &provision.ServiceSpec{
		TenantID:       tenantID,
		SubscriptionID: sub.ID,
		RouterID:       *sub.RouterID,
		Username:       sub.Username,
		ServiceType:    sub.ServiceType,
		ProfileName:    profile,
		IsolirProfile:  isolir,
		LocalAddress:   local,
		Comment:        provision.CommentTag(appName, sub.CustomerCode, sub.CustomerName),
	}
	if err := prov.Suspend(ctx, spec); err != nil {
		slog.Error("suspend subscription", "sub_id", sub.ID, "err", err)
		return "gagal di router: " + err.Error()
	}
	return "profil isolir diterapkan di " + r.Name
}

const isolirInfraTTL = 12 * time.Hour

func isolirInfraCacheKey(tenantID, routerID xid.ID, cfg store.IsolirNetworkSettings, slug string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		tenantID.String(),
		routerID.String(),
		strings.TrimSpace(slug),
		strings.TrimSpace(cfg.ProfileName),
		strings.TrimSpace(cfg.PoolName),
		strings.TrimSpace(cfg.PoolRanges),
		strings.TrimSpace(cfg.PoolGateway),
		strings.TrimSpace(cfg.PortalBaseURL),
		strings.TrimSpace(cfg.RedirectMode),
	}, "|")))
	return hex.EncodeToString(sum[:])
}

func (w *Worker) isolirInfraFresh(key string) bool {
	if w == nil {
		return false
	}
	w.isolirInfraMu.Lock()
	defer w.isolirInfraMu.Unlock()
	t, ok := w.isolirInfraOK[key]
	return ok && time.Since(t) < isolirInfraTTL
}

func (w *Worker) rememberIsolirInfra(key string) {
	if w == nil {
		return
	}
	w.isolirInfraMu.Lock()
	defer w.isolirInfraMu.Unlock()
	if w.isolirInfraOK == nil {
		w.isolirInfraOK = map[string]time.Time{}
	}
	w.isolirInfraOK[key] = time.Now()
}

func (w *Worker) tenantCycleDue(tenantID xid.ID, interval time.Duration, now time.Time) bool {
	if w == nil {
		return true
	}
	if interval < time.Minute {
		interval = time.Minute
	}
	w.cycleMu.Lock()
	defer w.cycleMu.Unlock()
	last, ok := w.lastCycleAt[tenantID]
	if !ok {
		return true
	}
	return !now.Before(last.Add(interval))
}

func (w *Worker) rememberTenantCycle(tenantID xid.ID, at time.Time) {
	if w == nil {
		return
	}
	w.cycleMu.Lock()
	defer w.cycleMu.Unlock()
	if w.lastCycleAt == nil {
		w.lastCycleAt = map[xid.ID]time.Time{}
	}
	w.lastCycleAt[tenantID] = at
}

func (w *Worker) resumePaidSubscriptions(ctx context.Context, tenantID xid.ID) map[xid.ID]struct{} {
	tried := map[xid.ID]struct{}{}
	ids, err := w.store.ListSubscriptionsNeedingResume(ctx, tenantID)
	if err != nil {
		slog.Warn("list subscriptions needing resume", "err", err)
		return tried
	}
	for _, subID := range ids {
		if free, ferr := w.store.IsFreeSubscription(ctx, tenantID, subID); ferr == nil && free {
			continue
		}
		tried[subID] = struct{}{}
		w.resumeSubscription(ctx, tenantID, subID)
	}
	return tried
}

func (w *Worker) repairIsolirSecrets(ctx context.Context, tenantID xid.ID, overdue, tried map[xid.ID]struct{}) {
	// Safety net only: scanning every router logs into MikroTik on each worker
	// cycle, which floods router logs. resumePaidSubscriptions() already handles
	// the common case, so run this repair at most once per hour per tenant.
	const repairInterval = time.Hour
	now := time.Now()
	w.repairMu.Lock()
	if last, ok := w.lastRepairAt[tenantID]; ok && now.Before(last.Add(repairInterval)) {
		w.repairMu.Unlock()
		return
	}
	w.lastRepairAt[tenantID] = now
	w.repairMu.Unlock()

	isolirCfg, _ := w.store.GetIsolirNetworkSettings(ctx, tenantID)
	_ = w.store.ResolveIsolirPool(ctx, tenantID, &isolirCfg)
	routers, err := w.store.ListRouters(ctx, tenantID)
	if err != nil {
		return
	}
	plans := map[xid.ID]*store.Plan{}
	planOf := func(planID xid.ID) *store.Plan {
		if p, ok := plans[planID]; ok {
			return p
		}
		p, err := w.store.GetPlan(ctx, tenantID, planID)
		if err != nil {
			return nil
		}
		plans[planID] = p
		return p
	}
	for _, r := range routers {
		if !r.IsActive {
			continue
		}
		prov, err := w.provisioner.Get(r.Provisioner)
		if err != nil {
			continue
		}
		reader, ok := prov.(provision.SecretReader)
		if !ok {
			continue
		}
		secrets, err := reader.ListServiceSecrets(ctx, tenantID, r.ID)
		if err != nil {
			slog.Warn("list service secrets", "router_id", r.ID, "err", err)
			continue
		}
		byUser := map[string]provision.SecretState{}
		for _, sec := range secrets {
			st := strings.TrimSpace(sec.ServiceType)
			if st == "" {
				st = "pppoe"
			}
			byUser[sec.Username+"|"+st] = sec
			if _, exists := byUser[sec.Username]; !exists {
				byUser[sec.Username] = sec
			}
		}
		subs, err := w.store.ListProvisionableSubscriptionsByRouter(ctx, tenantID, r.ID)
		if err != nil {
			continue
		}
		for i := range subs {
			sub := &subs[i]
			if _, skip := overdue[sub.ID]; skip {
				continue
			}
			if _, skip := tried[sub.ID]; skip {
				continue
			}
			if free, ferr := w.store.IsFreeSubscription(ctx, tenantID, sub.ID); ferr == nil && free {
				continue
			}
			want := strings.TrimSpace(sub.ServiceType)
			if want == "" {
				want = "pppoe"
			}
			sec, ok := byUser[sub.Username+"|"+want]
			if !ok {
				sec, ok = byUser[sub.Username]
			}
			if !ok {
				continue
			}
			plan := planOf(sub.PlanID)
			isolir := store.IsolirProfileName(isolirCfg, plan)
			planProfile := ""
			if plan != nil {
				planProfile = planRouterProfile(plan)
			}
			if !provision.SecretLooksIsolir(sec.Profile, sec.Comment, isolir, planProfile) {
				continue
			}
			tried[sub.ID] = struct{}{}
			w.resumeSubscription(ctx, tenantID, sub.ID)
		}
	}
}

func planRouterProfile(plan *store.Plan) string {
	if plan == nil {
		return ""
	}
	if plan.ProfileName != nil {
		if p := strings.TrimSpace(*plan.ProfileName); p != "" {
			return p
		}
	}
	return strings.TrimSpace(plan.Code)
}

func (w *Worker) resumeSubscription(ctx context.Context, tenantID, subID xid.ID) {
	sub, err := w.store.GetSubscription(ctx, tenantID, subID)
	if err != nil {
		return
	}
	if err := w.applyResumeOnRouter(ctx, tenantID, sub); err != nil {
		slog.Error("resume subscription", "sub_id", subID, "err", err)
		return
	}
	if sub.Status != "active" {
		if err := w.store.UpdateSubscriptionStatus(ctx, tenantID, subID, "active"); err != nil {
			slog.Warn("mark subscription active after resume", "sub_id", subID, "err", err)
		}
	}
	if err := w.store.ClearSubscriptionSuspendedAt(ctx, tenantID, subID); err != nil {
		slog.Warn("clear suspended_at after resume", "sub_id", subID, "err", err)
	}
}

func (w *Worker) applyResumeOnRouter(ctx context.Context, tenantID xid.ID, sub *store.Subscription) error {
	if sub.RouterID == nil {
		return fmt.Errorf("router belum di-set")
	}
	plan, err := w.store.GetPlan(ctx, tenantID, sub.PlanID)
	if err != nil {
		return err
	}
	r, err := w.store.GetRouter(ctx, tenantID, *sub.RouterID)
	if err != nil {
		return err
	}
	prov, err := w.provisioner.Get(r.Provisioner)
	if err != nil {
		return err
	}
	appName, _ := w.store.EffectiveAppName(ctx, tenantID)
	spec := &provision.ServiceSpec{
		TenantID:       tenantID,
		SubscriptionID: sub.ID,
		RouterID:       *sub.RouterID,
		Username:       sub.Username,
		ServiceType:    sub.ServiceType,
		ProfileName:    planRouterProfile(plan),
		Comment:        provision.CommentTag(appName, sub.CustomerCode, sub.CustomerName),
	}
	if sub.Password != nil {
		spec.Password = *sub.Password
	}
	w.fillResumeIPAM(ctx, sub, spec)
	if err := prov.Resume(ctx, spec); err != nil {
		return fmt.Errorf("router %s: %w", r.Name, err)
	}
	return nil
}

func (w *Worker) fillResumeIPAM(ctx context.Context, sub *store.Subscription, spec *provision.ServiceSpec) {
	asg, err := w.store.GetAssignedIPForCustomer(ctx, sub.TenantID, sub.CustomerID)
	if err != nil || asg == nil {
		return
	}
	if asg.Pool.RouterID != nil && sub.RouterID != nil && *asg.Pool.RouterID != *sub.RouterID {
		return
	}
	spec.IPAddress = strings.TrimSpace(asg.IPAddress)
	if asg.Pool.Gateway != nil {
		spec.LocalAddress = strings.TrimSpace(*asg.Pool.Gateway)
	}
}

func (w *Worker) processDunning(ctx context.Context, tenantID xid.ID, offsets []int) {
	if len(offsets) == 0 {
		offsets = store.DefaultJobScheduleSettings().DunningOffsets
	}
	invoices, err := w.store.ListDunningInvoices(ctx, tenantID)
	if err != nil {
		return
	}
	for _, inv := range invoices {
		if strings.TrimSpace(inv.CustomerPhone) == "" {
			continue
		}
		if inv.SubscriptionID != nil {
			if free, ferr := w.store.IsFreeSubscription(ctx, tenantID, *inv.SubscriptionID); ferr == nil && free {
				continue
			}
		}
		due := inv.DueDate
		now := time.Now()
		dueDay := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, now.Location())
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		days := int(today.Sub(dueDay).Hours() / 24) // negative = before due
		for _, target := range offsets {
			if days != target {
				continue
			}
			jobKey := fmt.Sprintf("inv-%d-day-%d", inv.ID, days)
			ok, err := w.store.ClaimJob(ctx, tenantID, "dunning", jobKey)
			if err != nil || !ok {
				break
			}
			planName := w.store.PlanNameForSubscription(ctx, tenantID, inv.SubscriptionID)
			_ = w.notify.SendInvoiceReminder(ctx, tenantID, inv.CustomerPhone, inv.CustomerName, planName, inv.InvoiceNumber, inv.TotalAmount, due.Format("02/01/2006"))
			break
		}
	}
}

func (w *Worker) monthlyReportEmail(ctx context.Context, t store.Tenant, now time.Time, day, hour int) {
	if now.Day() != day || now.Hour() != hour {
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
		Subject: fmt.Sprintf("Laporan bisnis bulanan %s", t.Name), Body: body, Event: "monthly_report",
	})
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
