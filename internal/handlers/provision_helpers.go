package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/payment"
	"github.com/dianrp-space/d5net-billing/internal/provision"
	"github.com/dianrp-space/d5net-billing/internal/provision/routeros"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

func ownershipComment(ctx context.Context, d *Deps, tenantID xid.ID, code, name string) string {
	app, err := d.Store.EffectiveAppName(ctx, tenantID)
	if err != nil || app == "" {
		app = "Delima Net"
	}
	return provision.CommentTag(app, code, name)
}

func planProfileName(plan *store.Plan) string {
	if plan == nil {
		return ""
	}
	if plan.ProfileName != nil {
		if p := strings.TrimSpace(*plan.ProfileName); p != "" {
			return p
		}
	}
	return plan.Code
}

func removeSubscriptionFromRouter(ctx context.Context, d *Deps, tid xid.ID, sub *store.Subscription) {
	if sub == nil || sub.RouterID == nil {
		return
	}
	r, err := d.Store.GetRouter(ctx, tid, *sub.RouterID)
	if err != nil {
		return
	}
	prov, err := d.Provisioner.Get(r.Provisioner)
	if err != nil {
		return
	}
	password := ""
	if sub.Password != nil {
		password = *sub.Password
	}
	if err := prov.Remove(ctx, &provision.ServiceSpec{
		TenantID: tid, SubscriptionID: sub.ID, RouterID: *sub.RouterID,
		Username: sub.Username, Password: password, ServiceType: sub.ServiceType,
	}); err != nil {
		slog.Warn("remove subscription from router", "sub_id", sub.ID, "err", err)
	}
}

func dismantleCustomerServices(ctx context.Context, d *Deps, tid xid.ID, customerID xid.ID) error {
	subs, _, err := d.Store.ListSubscriptions(ctx, tid, "", &customerID, 500, 0)
	if err != nil {
		return err
	}
	for _, row := range subs {
		sub, gerr := d.Store.GetSubscription(ctx, tid, row.ID)
		if gerr != nil {
			continue
		}
		removeSubscriptionFromRouter(ctx, d, tid, sub)
		_ = d.Store.ReleaseODPPortBySubscription(ctx, tid, sub.ID)
	}
	if err := d.Store.CancelCustomerSubscriptions(ctx, tid, customerID); err != nil {
		return err
	}
	asgs, err := d.Store.ListIPAssignmentsByCustomer(ctx, tid, customerID)
	if err != nil {
		return err
	}
	for _, a := range asgs {
		pool, perr := d.Store.GetIPPool(ctx, tid, a.PoolID)
		if perr == nil {
			clearCustomerStaticIPOnRouter(ctx, d, pool, customerID)
		}
		_ = d.Store.DeleteIPAssignment(ctx, tid, a.PoolID, a.ID)
	}
	return nil
}

// applyPlanHotspotLimits copies quota / uptime / shared-users from plan onto a ServiceSpec.
func applyPlanHotspotLimits(spec *provision.ServiceSpec, plan *store.Plan) {
	if spec == nil || plan == nil {
		return
	}
	if plan.QuotaGB != nil && *plan.QuotaGB > 0 {
		spec.LimitBytesTotal = int64(*plan.QuotaGB) * 1024 * 1024 * 1024
	}
	if plan.LimitUptime != nil {
		if u := strings.TrimSpace(*plan.LimitUptime); u != "" {
			spec.LimitUptime = u
		}
	}
	if plan.SharedUsers != nil && *plan.SharedUsers > 0 {
		spec.SharedUsers = *plan.SharedUsers
	}
}

// syncSubscriptionToRouter pushes PPPoE/hotspot secret to RouterOS (remove+add).
// When oldUsername/oldRouter differ, removes the previous secret first.
func syncSubscriptionToRouter(
	ctx context.Context,
	d *Deps,
	sub *store.Subscription,
	plan *store.Plan,
	oldUsername string,
	oldRouterID *xid.ID,
) error {
	if sub == nil || plan == nil {
		return nil
	}
	switch sub.Status {
	case "active", "suspended", "overdue":
	default:
		// pending / terminated: belum (atau tidak) ada secret di router
		return nil
	}
	password := ""
	if sub.Password != nil {
		password = strings.TrimSpace(*sub.Password)
	}
	if password == "" {
		return fmt.Errorf("password PPPoE kosong — isi password saat edit untuk sync ke RouterOS")
	}
	profile := planProfileName(plan)
	comment := ownershipComment(ctx, d, sub.TenantID, sub.CustomerCode, sub.CustomerName)

	removeOn := func(routerID xid.ID, username string) {
		if username == "" {
			return
		}
		r, err := d.Store.GetRouter(ctx, sub.TenantID, routerID)
		if err != nil {
			return
		}
		prov, err := d.Provisioner.Get(r.Provisioner)
		if err != nil {
			return
		}
		_ = prov.Remove(ctx, &provision.ServiceSpec{
			TenantID: sub.TenantID, SubscriptionID: sub.ID, RouterID: routerID,
			Username: username, Password: password, ServiceType: sub.ServiceType,
		})
	}

	newRouter := sub.RouterID
	usernameChanged := oldUsername != "" && oldUsername != sub.Username
	routerChanged := false
	if oldRouterID != nil && newRouter != nil {
		routerChanged = *oldRouterID != *newRouter
	} else if oldRouterID != nil && newRouter == nil {
		routerChanged = true
	} else if oldRouterID == nil && newRouter != nil {
		routerChanged = false // first assign handled by Apply
	}

	if oldRouterID != nil && (usernameChanged || routerChanged || newRouter == nil) {
		removeOn(*oldRouterID, oldUsername)
	}
	// If only username changed on same router, also remove new name collision then Apply
	if newRouter != nil && usernameChanged && !routerChanged {
		removeOn(*newRouter, oldUsername)
	}

	if newRouter == nil {
		return nil
	}
	r, err := d.Store.GetRouter(ctx, sub.TenantID, *newRouter)
	if err != nil {
		return fmt.Errorf("router: %w", err)
	}
	prov, err := d.Provisioner.Get(r.Provisioner)
	if err != nil {
		return fmt.Errorf("provisioner: %w", err)
	}
	spec := &provision.ServiceSpec{
		TenantID:       sub.TenantID,
		SubscriptionID: sub.ID,
		RouterID:       *newRouter,
		Username:       sub.Username,
		Password:       password,
		ServiceType:    sub.ServiceType,
		ProfileName:    profile,
		DownloadMbps:   plan.DownloadMbps,
		UploadMbps:     plan.UploadMbps,
		Comment:        comment,
	}
	if sub.ServiceType == "hotspot" {
		applyPlanHotspotLimits(spec, plan)
	}
	applyIPAMToSpec(ctx, d, sub, spec)
	preferred := resolveOfferIPPoolID(ctx, d, sub.TenantID, plan.ID, clusterIDForSubscription(ctx, d, sub))
	if p := resolveIPPoolForRouter(ctx, d, sub.TenantID, *newRouter, preferred); p != nil {
		_ = syncIPPoolToRouter(ctx, d, p)
		// Dynamic IP: pin profile to offer/first pool. Static assignment keeps its own pool name.
		if spec.IPAddress == "" {
			spec.AddressPool = p.Name
			if p.Gateway != nil {
				spec.LocalAddress = strings.TrimSpace(*p.Gateway)
			}
		}
	}
	if ensurer, ok := prov.(provision.ProfileEnsurer); ok && profile != "" {
		price := plan.Price
		if cust, err := d.Store.GetCustomer(ctx, sub.TenantID, sub.CustomerID); err == nil && cust != nil {
			if p, err := d.Store.ResolvePlanPrice(ctx, sub.TenantID, plan.ID, cust.ClusterID); err == nil {
				price = p
			}
		}
		_ = ensurer.EnsureBandwidthProfile(ctx, sub.TenantID, *newRouter, profile, plan.DownloadMbps, plan.UploadMbps, sub.ServiceType, spec.AddressPool, price)
	}
	if err := prov.Apply(ctx, spec); err != nil {
		return err
	}
	// Keep suspended state on router after rewrite
	if sub.Status == "suspended" {
		isolirCfg, _ := d.Store.GetIsolirNetworkSettings(ctx, sub.TenantID)
		_ = d.Store.ResolveIsolirPool(ctx, sub.TenantID, &isolirCfg)
		spec.IsolirProfile = store.IsolirProfileName(isolirCfg, plan)
		isolirCfg.ProfileName = spec.IsolirProfile
		spec.IPAddress = ""
		spec.AddressPool = ""
		local := strings.TrimSpace(isolirCfg.PoolGateway)
		if local == "" {
			local = routeros.CIDRLocalAddress(isolirCfg.PoolRanges, nil)
		}
		spec.LocalAddress = local
		if ensurer, ok := prov.(provision.IsolirEnsurer); ok && isolirCfg.PoolRanges != "" && isolirCfg.PortalBaseURL != "" {
			ten, _ := d.Store.GetTenant(ctx, sub.TenantID)
			slug := ""
			if ten != nil {
				slug = ten.Slug
			}
			if err := ensurer.EnsureIsolirInfra(ctx, sub.TenantID, *newRouter, isolirCfg, slug); err != nil {
				slog.Warn("ensure isolir infra after sync", "sub_id", sub.ID, "err", err)
			}
		}
		if err := prov.Suspend(ctx, spec); err != nil {
			slog.Warn("re-suspend after sync", "sub_id", sub.ID, "err", err)
		}
	}
	return nil
}

// applyIPAMToSpec fills static remote-address / local-address / profile pool from IPAM.
func applyIPAMToSpec(ctx context.Context, d *Deps, sub *store.Subscription, spec *provision.ServiceSpec) {
	if sub == nil || spec == nil {
		return
	}
	asg, err := d.Store.GetAssignedIPForCustomer(ctx, sub.TenantID, sub.CustomerID)
	if err == nil && asg != nil {
		if asg.Pool.RouterID == nil || sub.RouterID == nil || *asg.Pool.RouterID == *sub.RouterID {
			spec.IPAddress = strings.TrimSpace(asg.IPAddress)
			spec.AddressPool = asg.Pool.Name
			if asg.Pool.Gateway != nil {
				spec.LocalAddress = strings.TrimSpace(*asg.Pool.Gateway)
			}
			return
		}
	}
	// No static assignment: attach pool from cluster offer (if any) or first pool on router.
	if sub.RouterID != nil {
		preferred := resolveOfferIPPoolID(ctx, d, sub.TenantID, sub.PlanID, clusterIDForSubscription(ctx, d, sub))
		if p := resolveIPPoolForRouter(ctx, d, sub.TenantID, *sub.RouterID, preferred); p != nil {
			spec.AddressPool = p.Name
			if p.Gateway != nil {
				spec.LocalAddress = strings.TrimSpace(*p.Gateway)
			}
		}
	}
}

// resolveOfferIPPoolID returns the IP pool chosen on plan×cluster offer, if any.
func resolveOfferIPPoolID(ctx context.Context, d *Deps, tenantID, planID xid.ID, clusterID *xid.ID) *xid.ID {
	if clusterID == nil {
		return nil
	}
	offer, err := d.Store.GetPlanOfferByPlanCluster(ctx, tenantID, planID, *clusterID)
	if err != nil || offer == nil {
		return nil
	}
	return offer.IPPoolID
}

// clusterIDForSubscription prefers customer cluster, else the router's site/cluster.
func clusterIDForSubscription(ctx context.Context, d *Deps, sub *store.Subscription) *xid.ID {
	if sub == nil {
		return nil
	}
	if cust, err := d.Store.GetCustomer(ctx, sub.TenantID, sub.CustomerID); err == nil && cust != nil && cust.ClusterID != nil {
		return cust.ClusterID
	}
	if sub.RouterID != nil {
		if r, err := d.Store.GetRouter(ctx, sub.TenantID, *sub.RouterID); err == nil && r != nil && r.SiteID != nil {
			return r.SiteID
		}
	}
	return nil
}

// resolveIPPoolForRouter picks the preferred offer pool when it belongs to this router,
// otherwise the first IPAM pool linked to the router.
func resolveIPPoolForRouter(ctx context.Context, d *Deps, tenantID, routerID xid.ID, preferredPoolID *xid.ID) *store.IPPool {
	if preferredPoolID != nil {
		if p, err := d.Store.GetIPPool(ctx, tenantID, *preferredPoolID); err == nil && p != nil {
			if p.RouterID != nil && *p.RouterID == routerID {
				return p
			}
		}
	}
	if p, err := d.Store.GetFirstIPPoolForRouter(ctx, tenantID, routerID); err == nil && p != nil {
		return p
	}
	return nil
}

// ensureProfileIPPool syncs the resolved pool to RouterOS and returns its name for profile address-pool.
func ensureProfileIPPool(ctx context.Context, d *Deps, tenantID, routerID xid.ID, preferredPoolID *xid.ID) string {
	p := resolveIPPoolForRouter(ctx, d, tenantID, routerID, preferredPoolID)
	if p == nil {
		return ""
	}
	if err := syncIPPoolToRouter(ctx, d, p); err != nil {
		slog.Warn("ensure profile ip pool", "pool", p.Name, "router_id", routerID, "err", err)
	}
	return p.Name
}

// syncIPPoolToRouter creates/updates /ip/pool on the linked MikroTik.
func syncIPPoolToRouter(ctx context.Context, d *Deps, pool *store.IPPool) error {
	if pool == nil || pool.RouterID == nil {
		return nil
	}
	r, err := d.Store.GetRouter(ctx, pool.TenantID, *pool.RouterID)
	if err != nil {
		return fmt.Errorf("router: %w", err)
	}
	prov, err := d.Provisioner.Get(r.Provisioner)
	if err != nil {
		return fmt.Errorf("provisioner: %w", err)
	}
	ensurer, ok := prov.(provision.IPPoolEnsurer)
	if !ok {
		return fmt.Errorf("provisioner %s tidak mendukung IP pool", r.Provisioner)
	}
	return ensurer.EnsureIPPool(ctx, pool.TenantID, *pool.RouterID, pool.Name, pool.Network, pool.Gateway)
}

func removeIPPoolFromRouter(ctx context.Context, d *Deps, tenantID xid.ID, routerID xid.ID, poolName string) {
	r, err := d.Store.GetRouter(ctx, tenantID, routerID)
	if err != nil {
		return
	}
	prov, err := d.Provisioner.Get(r.Provisioner)
	if err != nil {
		return
	}
	ensurer, ok := prov.(provision.IPPoolEnsurer)
	if !ok {
		return
	}
	if err := ensurer.RemoveIPPool(ctx, tenantID, routerID, poolName); err != nil {
		slog.Warn("hapus ip pool di RouterOS gagal", "pool", poolName, "err", err)
	}
}

// syncCustomerIPAssignment pushes static IP onto the customer's PPP secret (and ensures pool exists).
func syncCustomerIPAssignment(ctx context.Context, d *Deps, pool *store.IPPool, customerID xid.ID) error {
	if pool == nil {
		return nil
	}
	if err := syncIPPoolToRouter(ctx, d, pool); err != nil {
		slog.Warn("sync ip pool ke RouterOS", "pool", pool.Name, "err", err)
	}
	sub, err := d.Store.FindProvisionableSubscriptionByCustomer(ctx, pool.TenantID, customerID, pool.RouterID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	plan, err := d.Store.GetPlan(ctx, pool.TenantID, sub.PlanID)
	if err != nil {
		return err
	}
	return syncSubscriptionToRouter(ctx, d, sub, plan, sub.Username, sub.RouterID)
}

func clearCustomerStaticIPOnRouter(ctx context.Context, d *Deps, pool *store.IPPool, customerID xid.ID) {
	if pool == nil {
		return
	}
	sub, err := d.Store.FindProvisionableSubscriptionByCustomer(ctx, pool.TenantID, customerID, pool.RouterID)
	if err != nil {
		return
	}
	plan, err := d.Store.GetPlan(ctx, pool.TenantID, sub.PlanID)
	if err != nil {
		return
	}
	if err := syncSubscriptionToRouter(ctx, d, sub, plan, sub.Username, sub.RouterID); err != nil {
		slog.Warn("clear static IP di RouterOS gagal", "sub_id", sub.ID, "err", err)
	}
}

func resumeAfterInvoicePaid(ctx context.Context, d *Deps, tenantID xid.ID, inv *store.Invoice) {
	if inv == nil {
		return
	}
	ids := map[xid.ID]struct{}{}
	if inv.SubscriptionID != nil && !xid.IsNil(*inv.SubscriptionID) {
		ids[*inv.SubscriptionID] = struct{}{}
	}
	cid := inv.CustomerID
	subs, _, err := d.Store.ListSubscriptions(ctx, tenantID, "", &cid, 200, 0)
	if err == nil {
		for _, sub := range subs {
			ids[sub.ID] = struct{}{}
		}
	}
	for sid := range ids {
		stillDue, err := d.Store.SubscriptionHasPastDueUnpaid(ctx, tenantID, sid)
		if err != nil || stillDue {
			continue
		}
		_ = d.Store.UpdateSubscriptionStatus(ctx, tenantID, sid, "active")
		resumeSubscription(ctx, d, tenantID, sid)
	}
}

func resumeSubscription(ctx context.Context, d *Deps, tenantID xid.ID, subID xid.ID) {
	sub, err := d.Store.GetSubscription(ctx, tenantID, subID)
	if err != nil {
		return
	}
	plan, err := d.Store.GetPlan(ctx, tenantID, sub.PlanID)
	if err != nil {
		return
	}
	if sub.RouterID == nil {
		return
	}
	r, err := d.Store.GetRouter(ctx, tenantID, *sub.RouterID)
	if err != nil {
		return
	}
	prov, err := d.Provisioner.Get(r.Provisioner)
	if err != nil {
		return
	}
	profile := planProfileName(plan)
	spec := &provision.ServiceSpec{
		TenantID:       tenantID,
		SubscriptionID: subID,
		RouterID:       *sub.RouterID,
		Username:       sub.Username,
		ServiceType:    sub.ServiceType,
		ProfileName:    profile,
		Comment:        ownershipComment(ctx, d, tenantID, sub.CustomerCode, sub.CustomerName),
	}
	if sub.Password != nil {
		spec.Password = *sub.Password
	}
	applyStaticIPAMToSpec(ctx, d, sub, spec)
	if err := prov.Resume(ctx, spec); err != nil {
		slog.Error("resume subscription after payment", "sub_id", subID, "err", err)
		return
	}
	if err := d.Store.ClearSubscriptionSuspendedAt(ctx, tenantID, subID); err != nil {
		slog.Warn("clear suspended_at after resume", "sub_id", subID, "err", err)
	}
}

// isolirSubscription forces a subscription into isolir (suspend) immediately:
// it marks the billing status suspended and swaps the RouterOS secret to the
// isolir profile. Used by the manual admin "suspend" action so operators do not
// have to wait for the billing worker / grace period.
func isolirSubscription(ctx context.Context, d *Deps, tenantID xid.ID, subID xid.ID) error {
	sub, err := d.Store.GetSubscription(ctx, tenantID, subID)
	if err != nil {
		return httpx.NotFound("subscription not found")
	}
	if err := d.Store.UpdateSubscriptionStatus(ctx, tenantID, subID, "suspended"); err != nil {
		return httpx.Internal(err)
	}
	if sub.RouterID == nil {
		return nil
	}
	plan, err := d.Store.GetPlan(ctx, tenantID, sub.PlanID)
	if err != nil {
		return httpx.Internal(err)
	}
	r, err := d.Store.GetRouter(ctx, tenantID, *sub.RouterID)
	if err != nil {
		return nil
	}
	prov, err := d.Provisioner.Get(r.Provisioner)
	if err != nil {
		return nil
	}
	isolirCfg, _ := d.Store.GetIsolirNetworkSettings(ctx, tenantID)
	_ = d.Store.ResolveIsolirPool(ctx, tenantID, &isolirCfg)
	isolir := store.IsolirProfileName(isolirCfg, plan)
	isolirCfg.ProfileName = isolir
	if ensurer, ok := prov.(provision.IsolirEnsurer); ok && isolirCfg.PoolRanges != "" && isolirCfg.PortalBaseURL != "" {
		ten, _ := d.Store.GetTenant(ctx, tenantID)
		slug := ""
		if ten != nil {
			slug = ten.Slug
		}
		if err := ensurer.EnsureIsolirInfra(ctx, tenantID, *sub.RouterID, isolirCfg, slug); err != nil {
			slog.Warn("ensure isolir infra (manual suspend)", "sub_id", subID, "err", err)
		}
	}
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
		ProfileName:    planProfileName(plan),
		IsolirProfile:  isolir,
		LocalAddress:   local,
		Comment:        ownershipComment(ctx, d, tenantID, sub.CustomerCode, sub.CustomerName),
	}
	if err := prov.Suspend(ctx, spec); err != nil {
		slog.Error("manual suspend subscription", "sub_id", subID, "err", err)
		return httpx.BadRequest("gagal isolir di router: " + err.Error())
	}
	return nil
}

// normalizeCustomerStatus maps UI/legacy labels to canonical customer statuses.
func normalizeCustomerStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "active", "aktif":
		return "active"
	case "isolir", "suspend", "suspended":
		return "isolir"
	case "inactive", "nonaktif":
		return "inactive"
	case "overdue", "tunggakan":
		return "overdue"
	case "dismantled", "cabut":
		return "dismantled"
	default:
		return ""
	}
}

// applyCustomerStatus syncs every live subscription of a customer to the given
// customer-level status: "active" resumes isolir services, "isolir"/"inactive"
// suspends them (isolir profile on the router). Best-effort per subscription so
// one offline router does not block the rest.
func applyCustomerStatus(ctx context.Context, d *Deps, tenantID, customerID xid.ID, status string) error {
	cid := customerID
	subs, _, err := d.Store.ListSubscriptions(ctx, tenantID, "", &cid, 500, 0)
	if err != nil {
		return httpx.Internal(err)
	}
	switch status {
	case "active":
		for _, sub := range subs {
			if sub.Status != "suspended" {
				continue
			}
			if err := d.Store.UpdateSubscriptionStatus(ctx, tenantID, sub.ID, "active"); err != nil {
				return httpx.Internal(err)
			}
			resumeSubscription(ctx, d, tenantID, sub.ID)
		}
	case "isolir", "inactive":
		for _, sub := range subs {
			if sub.Status != "active" && sub.Status != "overdue" {
				continue
			}
			if err := isolirSubscription(ctx, d, tenantID, sub.ID); err != nil {
				slog.Warn("customer status: isolir subscription failed", "customer_id", customerID, "sub_id", sub.ID, "err", err)
			}
		}
	}
	return nil
}

// applyStaticIPAMToSpec pins a static assignment on resume. Dynamic users leave
// remote/local empty so the PPP profile (not the isolir pool) assigns addresses.
func applyStaticIPAMToSpec(ctx context.Context, d *Deps, sub *store.Subscription, spec *provision.ServiceSpec) {
	if sub == nil || spec == nil {
		return
	}
	asg, err := d.Store.GetAssignedIPForCustomer(ctx, sub.TenantID, sub.CustomerID)
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

func completePaidWebhook(ctx context.Context, d *Deps, provider string, event *payment.WebhookEvent) error {
	pi, err := d.Store.GetPaymentIntentByExternalID(ctx, event.ExternalID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			slog.Warn("payment intent not found for webhook", "external_id", event.ExternalID)
			return nil
		}
		return err
	}
	if pi.Status == "paid" {
		return nil // idempotent
	}

	// Topup saldo: tidak terkait invoice. Tambah saldo, notifikasi, lalu lunasi
	// tagihan menunggak dari saldo.
	if metaString(pi.Metadata, "purpose") == "wallet_topup" {
		if err := completeWalletTopup(ctx, d, pi, event); err != nil {
			return err
		}
		_ = d.Store.UpdatePaymentIntentStatus(ctx, pi.ExternalID, "paid")
		_ = d.Store.DispatchOutboundEvent(ctx, pi.TenantID, "wallet.topup", map[string]any{
			"external_id": event.ExternalID,
			"provider":    provider,
			"customer_id": pi.CustomerID,
		})
		return nil
	}

	// Tagihan asli (tanpa biaya admin). Untuk DOKU dengan surcharge, pi.Amount
	// = base + fee, sedangkan yang dilunasi ke invoice hanya base.
	base := pi.Amount
	if b := metaInt64(pi.Metadata, "base_amount"); b > 0 {
		base = b
	}
	amount := event.Amount
	if amount <= 0 {
		amount = base
	}
	if base > 0 && amount > base {
		// Biaya admin customer / unique-digit QRIS bisa di atas tagihan; yang
		// dicatat sebagai pembayaran invoice tetap sebesar tagihan asli.
		amount = base
	}

	var invoiceID *xid.ID
	var inv *store.Invoice
	var items []store.InvoiceItem
	if pi.InvoiceID != nil {
		invoiceID = pi.InvoiceID
		inv, items, err = d.Store.GetInvoice(ctx, pi.TenantID, *pi.InvoiceID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}

	ref := event.Reference
	if ref == "" {
		ref = event.ExternalID
	}
	p := &store.Payment{
		TenantID:   pi.TenantID,
		CustomerID: pi.CustomerID,
		InvoiceID:  invoiceID,
		Amount:     amount,
		Method:     payment.MethodFromProvider(provider),
		Status:     "paid",
		Sandbox:    intentSandbox(pi.Metadata),
		Reference:  &ref,
	}
	if err := d.Store.RecordPayment(ctx, p); err != nil {
		return err
	}
	_ = d.Store.UpdatePaymentIntentStatus(ctx, pi.ExternalID, "paid")
	if pi.InvoiceID != nil {
		_ = d.Store.CancelPendingPaymentIntentsForInvoice(ctx, pi.TenantID, *pi.InvoiceID)
	}

	if inv != nil {
		resumeAfterInvoicePaid(ctx, d, pi.TenantID, inv)
	}

	// Optional tip credit (0 = skip).
	tip := webhookTipAmount(event)
	if tip > 0 {
		_ = d.Store.CreditWallet(ctx, pi.TenantID, pi.CustomerID, tip, "tip", event.ExternalID, "payment tip")
	}

	cashID, revID, _ := d.Store.FindCashAndRevenueAccounts(ctx, pi.TenantID)
	if !xid.IsNil(cashID) && !xid.IsNil(revID) {
		invRef := event.ExternalID
		if inv != nil {
			invRef = inv.InvoiceNumber
		}
		if err := d.Store.RecordPaymentJournal(ctx, pi.TenantID, amount, cashID, revID, invRef); err != nil {
			slog.Warn("record payment journal", "err", err)
		}
	}

	_ = d.Store.DispatchOutboundEvent(ctx, pi.TenantID, "payment.paid", map[string]any{
		"external_id": event.ExternalID,
		"provider":    provider,
		"amount":      amount,
		"invoice_id":  invoiceID,
		"customer_id": pi.CustomerID,
		"payment_id":  p.ID,
	})

	if inv != nil {
		cust, _ := d.Store.GetCustomer(ctx, pi.TenantID, pi.CustomerID)
		if cust != nil && cust.Phone != "" {
			planName := d.Store.PlanNameForSubscription(ctx, pi.TenantID, inv.SubscriptionID)
			itemName := store.NotificationItemName(planName, items)
			_ = d.Notify.SendPaymentConfirmation(ctx, pi.TenantID, cust.Phone, cust.FullName, planName, itemName, inv.InvoiceNumber, amount)
		}
	}

	return nil
}

func webhookTipAmount(event *payment.WebhookEvent) int64 {
	if event == nil || event.Raw == nil {
		return 0
	}
	for _, key := range []string{"tip", "tip_amount", "tips"} {
		switch v := event.Raw[key].(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		case int:
			return int64(v)
		case string:
			var n int64
			_, _ = fmt.Sscan(v, &n)
			return n
		}
	}
	return 0
}

// syncVoucherBatchToRouter pushes hotspot users (username=password=code) to the batch router.
// Returns updated batch from DB after writing sync_status.
func syncVoucherBatchToRouter(ctx context.Context, d *Deps, batch *store.VoucherBatch) (*store.VoucherBatch, error) {
	if batch == nil {
		return nil, fmt.Errorf("batch kosong")
	}
	if batch.RouterID == nil {
		_ = d.Store.UpdateVoucherBatchSync(ctx, batch.TenantID, batch.ID, "none", 0, "")
		return d.Store.GetVoucherBatch(ctx, batch.TenantID, batch.ID)
	}
	r, err := d.Store.GetRouter(ctx, batch.TenantID, *batch.RouterID)
	if err != nil {
		_ = d.Store.UpdateVoucherBatchSync(ctx, batch.TenantID, batch.ID, "failed", 0, "router tidak ditemukan")
		return d.Store.GetVoucherBatch(ctx, batch.TenantID, batch.ID)
	}
	if !r.IsActive {
		_ = d.Store.UpdateVoucherBatchSync(ctx, batch.TenantID, batch.ID, "failed", 0, "router nonaktif")
		return d.Store.GetVoucherBatch(ctx, batch.TenantID, batch.ID)
	}
	prov, err := d.Provisioner.Get(r.Provisioner)
	if err != nil {
		msg := fmt.Sprintf("provisioner: %v", err)
		_ = d.Store.UpdateVoucherBatchSync(ctx, batch.TenantID, batch.ID, "failed", 0, msg)
		return d.Store.GetVoucherBatch(ctx, batch.TenantID, batch.ID)
	}

	profile := ""
	download, upload := 0, 0
	price := batch.Price
	var plan *store.Plan
	if batch.PlanID != nil {
		if p, err := d.Store.GetPlan(ctx, batch.TenantID, *batch.PlanID); err == nil && p != nil {
			plan = p
			profile = planProfileName(plan)
			download, upload = plan.DownloadMbps, plan.UploadMbps
			if price <= 0 {
				price = plan.Price
			}
		}
	}

	// Hotspot users get IPs from the profile address-pool (not per-code static IPs).
	// Prefer pool from plan×cluster offer when router belongs to a cluster.
	var preferredPoolID *xid.ID
	if batch.PlanID != nil {
		var clusterID *xid.ID
		if r.SiteID != nil {
			clusterID = r.SiteID
		}
		preferredPoolID = resolveOfferIPPoolID(ctx, d, batch.TenantID, *batch.PlanID, clusterID)
	}
	poolName := ensureProfileIPPool(ctx, d, batch.TenantID, r.ID, preferredPoolID)

	if ensurer, ok := prov.(provision.ProfileEnsurer); ok && profile != "" {
		if err := ensurer.EnsureBandwidthProfile(ctx, batch.TenantID, r.ID, profile, download, upload, "hotspot", poolName, price); err != nil {
			slog.Warn("voucher ensure hotspot profile", "batch_id", batch.ID, "profile", profile, "err", err)
		}
	} else if profile == "" && poolName != "" {
		slog.Warn("voucher sync: paket tidak dipilih — address-pool tidak di-set di profil (pilih paket hotspot + offer)",
			"batch_id", batch.ID, "pool", poolName)
	}

	codes, err := d.Store.ListVouchersByBatch(ctx, batch.TenantID, batch.ID, "")
	if err != nil {
		_ = d.Store.UpdateVoucherBatchSync(ctx, batch.TenantID, batch.ID, "failed", 0, err.Error())
		return d.Store.GetVoucherBatch(ctx, batch.TenantID, batch.ID)
	}

	commentBase := ownershipComment(ctx, d, batch.TenantID, "VCH", batch.Name)
	okCount := 0
	var firstErr string
	for _, v := range codes {
		code := strings.TrimSpace(v.Code)
		if code == "" {
			continue
		}
		spec := &provision.ServiceSpec{
			TenantID:     batch.TenantID,
			RouterID:     r.ID,
			Username:     code,
			Password:     code,
			ServiceType:  "hotspot",
			ProfileName:  profile,
			DownloadMbps: download,
			UploadMbps:   upload,
			Comment:      commentBase + " " + code,
		}
		applyPlanHotspotLimits(spec, plan)
		if err := prov.Apply(ctx, spec); err != nil {
			if firstErr == "" {
				firstErr = fmt.Sprintf("%s: %v", code, err)
			}
			slog.Warn("voucher hotspot sync failed", "batch_id", batch.ID, "code", code, "err", err)
			continue
		}
		okCount++
	}

	status := "synced"
	if okCount == 0 && len(codes) > 0 {
		status = "failed"
		if firstErr == "" {
			firstErr = "semua kode gagal di-push ke router"
		}
	} else if firstErr != "" {
		status = "partial"
	}
	_ = d.Store.UpdateVoucherBatchSync(ctx, batch.TenantID, batch.ID, status, okCount, firstErr)
	return d.Store.GetVoucherBatch(ctx, batch.TenantID, batch.ID)
}

func removeVoucherBatchFromRouter(ctx context.Context, d *Deps, batch *store.VoucherBatch) {
	if batch == nil || batch.RouterID == nil {
		return
	}
	r, err := d.Store.GetRouter(ctx, batch.TenantID, *batch.RouterID)
	if err != nil {
		return
	}
	prov, err := d.Provisioner.Get(r.Provisioner)
	if err != nil {
		return
	}
	codes, err := d.Store.ListVouchersByBatch(ctx, batch.TenantID, batch.ID, "")
	if err != nil {
		return
	}
	for _, v := range codes {
		code := strings.TrimSpace(v.Code)
		if code == "" {
			continue
		}
		_ = prov.Remove(ctx, &provision.ServiceSpec{
			TenantID: batch.TenantID, RouterID: r.ID,
			Username: code, Password: code, ServiceType: "hotspot",
		})
	}
}
