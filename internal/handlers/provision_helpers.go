package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dianrp/drp-billing/internal/payment"
	"github.com/dianrp/drp-billing/internal/provision"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

func ownershipComment(ctx context.Context, d *Deps, tenantID xid.ID, code, name string) string {
	app, err := d.Store.EffectiveAppName(ctx, tenantID)
	if err != nil || app == "" {
		app = "drp-billing"
	}
	return provision.CommentTag(app, code, name)
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
	profile := ""
	if plan.ProfileName != nil {
		profile = *plan.ProfileName
	}
	spec := &provision.ServiceSpec{
		TenantID:       tenantID,
		SubscriptionID: subID,
		RouterID:       *sub.RouterID,
		Username:       sub.Username,
		ServiceType:    sub.ServiceType,
		ProfileName:    profile,
		Comment:        ownershipComment(ctx, d, tenantID, sub.CustomerCode, sub.CustomerName),
	}
	if err := prov.Resume(ctx, spec); err != nil {
		slog.Error("resume subscription after payment", "sub_id", subID, "err", err)
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

	amount := event.Amount
	if amount <= 0 {
		amount = pi.Amount
	}

	var invoiceID *xid.ID
	var inv *store.Invoice
	if pi.InvoiceID != nil {
		invoiceID = pi.InvoiceID
		inv, _, err = d.Store.GetInvoice(ctx, pi.TenantID, *pi.InvoiceID)
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
		Method:     provider,
		Reference:  &ref,
	}
	if err := d.Store.RecordPayment(ctx, p); err != nil {
		return err
	}
	_ = d.Store.UpdatePaymentIntentStatus(ctx, event.ExternalID, "paid")

	if inv != nil && inv.SubscriptionID != nil {
		_ = d.Store.UpdateSubscriptionStatus(ctx, pi.TenantID, *inv.SubscriptionID, "active")
		resumeSubscription(ctx, d, pi.TenantID, *inv.SubscriptionID)
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
			_ = d.Notify.SendPaymentConfirmation(ctx, pi.TenantID, cust.Phone, inv.InvoiceNumber, amount)
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
