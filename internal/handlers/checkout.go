package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/payment"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

func invoiceRemaining(inv *store.Invoice) int64 {
	if inv == nil {
		return 0
	}
	amount := inv.TotalAmount - inv.PaidAmount
	if amount < 0 {
		return 0
	}
	return amount
}

func checkoutInvoice(ctx context.Context, d *Deps, tid xid.ID, inv *store.Invoice, providerName, returnURL string) (*store.PaymentIntent, error) {
	if inv == nil {
		return nil, httpx.NotFound("invoice not found")
	}
	st := strings.ToLower(strings.TrimSpace(inv.Status))
	if st == "paid" || st == "void" || st == "cancelled" {
		return nil, httpx.BadRequest("invoice sudah lunas / tidak bisa dibayar")
	}
	amount := invoiceRemaining(inv)
	if amount <= 0 {
		return nil, httpx.BadRequest("tidak ada sisa tagihan")
	}
	providerName = normalizePaymentProviderName(providerName)
	if providerName == payment.ProviderManual {
		return nil, httpx.BadRequest("gunakan pembayaran QRIS")
	}
	if existing, err := d.Store.GetLatestPendingPaymentIntent(ctx, tid, inv.ID, payment.ProviderDRP); err == nil && existing != nil && existing.QRString != "" && existing.Amount == amount {
		return existing, nil
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, httpx.Internal(err)
	}

	prov, err := resolvePaymentProvider(ctx, d, tid, providerName)
	if err != nil {
		return nil, err
	}
	res, err := prov.CreateIntent(ctx, payment.IntentRequest{
		TenantID: tid, CustomerID: inv.CustomerID, InvoiceID: inv.ID,
		Amount: amount, ReturnURL: returnURL,
	})
	if err != nil {
		return nil, httpx.BadRequest(err.Error())
	}
	meta := map[string]any{}
	if res.TransactionID != "" {
		meta["transaction_id"] = res.TransactionID
	}
	pi := &store.PaymentIntent{
		TenantID:      tid,
		CustomerID:    inv.CustomerID,
		InvoiceID:     &inv.ID,
		Provider:      payment.ProviderDRP,
		ExternalID:    res.ExternalID,
		Amount:        amount,
		Status:        res.Status,
		CheckoutURL:   res.CheckoutURL,
		ExpiresAt:     res.ExpiresAt,
		QRString:      res.QRString,
		QRImageBase64: res.QRImageBase64,
		PayableAmount: res.PayableAmount,
		UniqueDigit:   res.UniqueDigit,
		Metadata:      meta,
	}
	out, err := d.Store.InsertPaymentIntent(ctx, pi)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	return out, nil
}

func latestInvoicePaymentIntent(ctx context.Context, d *Deps, tid xid.ID, inv *store.Invoice) (*store.PaymentIntent, error) {
	if inv == nil {
		return nil, httpx.NotFound("invoice not found")
	}
	pi, err := d.Store.GetLatestPaymentIntentForInvoice(ctx, tid, inv.ID, payment.ProviderDRP)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("belum ada QRIS")
		}
		return nil, httpx.Internal(err)
	}
	if payment.WebhookIsPaid(pi.Status) {
		return pi, nil
	}
	prov, rerr := resolvePaymentProvider(ctx, d, tid, payment.ProviderDRP)
	if rerr == nil {
		if checker, ok := prov.(payment.StatusChecker); ok && pi.ExternalID != "" {
			if remote, serr := checker.CheckStatus(ctx, pi.ExternalID); serr == nil && remote != nil {
				status := strings.ToLower(strings.TrimSpace(remote.Status))
				if status != "" && status != pi.Status {
					_ = d.Store.UpdatePaymentIntentStatus(ctx, pi.ExternalID, status)
					pi.Status = status
				}
				if payment.WebhookIsPaid(status) {
					ev := &payment.WebhookEvent{
						ExternalID: pi.ExternalID,
						Status:     "paid",
						Amount:     pi.Amount,
						Reference:  remote.TransactionID,
					}
					if err := completePaidWebhook(ctx, d, payment.ProviderDRP, ev); err != nil {
						return nil, httpx.Internal(err)
					}
					pi.Status = "paid"
				}
			}
		}
	}
	return pi, nil
}

func cancelInvoicePaymentIntent(ctx context.Context, d *Deps, tid xid.ID, inv *store.Invoice) (*store.PaymentIntent, error) {
	if inv == nil {
		return nil, httpx.NotFound("invoice not found")
	}
	pi, err := d.Store.GetLatestPaymentIntentForInvoice(ctx, tid, inv.ID, payment.ProviderDRP)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("belum ada QRIS")
		}
		return nil, httpx.Internal(err)
	}
	if payment.WebhookIsPaid(pi.Status) {
		return nil, httpx.BadRequest("tagihan sudah lunas")
	}
	st := strings.ToLower(strings.TrimSpace(pi.Status))
	if st == "cancelled" || st == "canceled" || st == "expired" {
		return pi, nil
	}
	prov, err := resolvePaymentProvider(ctx, d, tid, payment.ProviderDRP)
	if err != nil {
		return nil, err
	}
	canceller, ok := prov.(payment.Canceller)
	if !ok {
		return nil, httpx.BadRequest("provider tidak mendukung batal QRIS")
	}
	if pi.ExternalID == "" {
		return nil, httpx.BadRequest("QRIS tidak memiliki referenceId")
	}
	if cerr := canceller.Cancel(ctx, pi.ExternalID); cerr != nil {
		if strings.Contains(cerr.Error(), "422") {
			synced, serr := latestInvoicePaymentIntent(ctx, d, tid, inv)
			if serr == nil {
				return synced, nil
			}
		}
		return nil, httpx.BadRequest(cerr.Error())
	}
	_ = d.Store.UpdatePaymentIntentStatus(ctx, pi.ExternalID, "cancelled")
	pi.Status = "cancelled"
	return pi, nil
}
