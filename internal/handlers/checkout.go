package handlers

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

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

func checkoutInvoice(ctx context.Context, d *Deps, tid xid.ID, inv *store.Invoice, providerName, returnURL, origin string) (*store.PaymentIntent, error) {
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
		return nil, httpx.BadRequest("gunakan pembayaran online (Duitku)")
	}
	if existing, err := d.Store.GetLatestPendingPaymentIntent(ctx, tid, inv.ID, providerName); err == nil && existing != nil && existing.Amount == amount {
		if existing.QRString != "" || strings.TrimSpace(existing.CheckoutURL) != "" {
			// Reuse the current checkout for this provider and cancel any other
			// pending intents (e.g. a different gateway picked earlier).
			_ = d.Store.CancelPendingPaymentIntentsExcept(ctx, tid, inv.ID, existing.ExternalID)
			return existing, nil
		}
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, httpx.Internal(err)
	}

	prov, err := resolvePaymentProvider(ctx, d, tid, providerName)
	if err != nil {
		return nil, err
	}
	req := payment.IntentRequest{
		TenantID: tid, CustomerID: inv.CustomerID, InvoiceID: inv.ID,
		Amount: amount, ReturnURL: returnURL,
		ProductDetails: "Tagihan " + strings.TrimSpace(inv.InvoiceNumber),
	}
	var ten *store.Tenant
	if t, terr := d.Store.GetTenant(ctx, tid); terr == nil {
		ten = t
	}
	if origin != "" {
		slug := ""
		if ten != nil {
			slug = ten.Slug
		}
		req.CallbackURL = strings.TrimRight(origin, "/") + paymentWebhookPathWithTenant(providerName, slug)
		if req.ReturnURL == "" {
			req.ReturnURL = origin
		}
	}
	if ten != nil && ten.Email != nil {
		req.Email = strings.TrimSpace(*ten.Email)
	}
	if cust, cerr := d.Store.GetCustomer(ctx, tid, inv.CustomerID); cerr == nil && cust != nil {
		req.CustomerName = strings.TrimSpace(cust.FullName)
		req.Phone = strings.TrimSpace(cust.Phone)
		if cust.Email != nil && strings.TrimSpace(*cust.Email) != "" {
			req.Email = strings.TrimSpace(*cust.Email)
		}
	}
	// Nomor invoice dipakai langsung sebagai ref merchantOrderId ke payment gateway.
	req.MerchantOrderID = strings.TrimSpace(inv.InvoiceNumber)
	if req.MerchantOrderID == "" {
		req.MerchantOrderID = store.FormatInvoiceNumber("", time.Now())
	}
	if req.CustomerName == "" {
		req.CustomerName = strings.TrimSpace(inv.CustomerName)
	}
	if req.Email == "" {
		if smtp, serr := loadSMTPIntegration(ctx, d, tid); serr == nil {
			req.Email = strings.TrimSpace(smtp.From)
		}
	}
	if req.Email == "" && origin != "" {
		if u, err := url.Parse(origin); err == nil {
			host := strings.TrimSpace(u.Hostname())
			if host != "" && host != "localhost" && !strings.HasPrefix(host, "127.") {
				req.Email = "billing@" + host
			}
		}
	}
	if providerName == payment.ProviderDuitku && req.CallbackURL == "" {
		return nil, httpx.BadRequest("callback Duitku membutuhkan URL publik (atur Portal URL di Isolir atau buka dari domain publik)")
	}

	res, err := prov.CreateIntent(ctx, req)
	if err != nil {
		return nil, httpx.BadRequest(err.Error())
	}
	meta := map[string]any{}
	if res.TransactionID != "" {
		meta["transaction_id"] = res.TransactionID
	}
	for k, v := range res.Metadata {
		meta[k] = v
	}
	pi := &store.PaymentIntent{
		TenantID:      tid,
		CustomerID:    inv.CustomerID,
		InvoiceID:     &inv.ID,
		Provider:      providerName,
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
	// Only one active checkout per invoice: cancel any older pending intents
	// (e.g. a QR from a different gateway the customer started but abandoned).
	_ = d.Store.CancelPendingPaymentIntentsExcept(ctx, tid, inv.ID, out.ExternalID)
	return out, nil
}

func latestInvoicePaymentIntent(ctx context.Context, d *Deps, tid xid.ID, inv *store.Invoice) (*store.PaymentIntent, error) {
	if inv == nil {
		return nil, httpx.NotFound("invoice not found")
	}
	pi, err := d.Store.GetLatestPaymentIntentForInvoiceAny(ctx, tid, inv.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("belum ada pembayaran")
		}
		return nil, httpx.Internal(err)
	}
	if payment.WebhookIsPaid(pi.Status) {
		return pi, nil
	}
	prov, rerr := resolvePaymentProvider(ctx, d, tid, pi.Provider)
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
					if err := completePaidWebhook(ctx, d, pi.Provider, ev); err != nil {
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
	pi, err := d.Store.GetLatestPaymentIntentForInvoiceAny(ctx, tid, inv.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("belum ada pembayaran")
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
	prov, err := resolvePaymentProvider(ctx, d, tid, pi.Provider)
	if err != nil {
		return nil, err
	}
	canceller, ok := prov.(payment.Canceller)
	if !ok {
		_ = d.Store.UpdatePaymentIntentStatus(ctx, pi.ExternalID, "cancelled")
		pi.Status = "cancelled"
		return pi, nil
	}
	if pi.ExternalID == "" {
		return nil, httpx.BadRequest("pembayaran tidak memiliki reference")
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
