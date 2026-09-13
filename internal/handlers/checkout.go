package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/payment"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// paymentCustomerFee mengembalikan biaya admin yang dibebankan ke customer untuk
// provider+channel tertentu (0 bila merchant menanggung / tidak berlaku).
func paymentCustomerFee(ctx context.Context, d *Deps, tid xid.ID, providerName, channel string, base int64) int64 {
	if providerName != payment.ProviderDoku || base <= 0 {
		return 0
	}
	cfg, err := loadDokuIntegration(ctx, d, tid)
	if err != nil {
		return 0
	}
	if ch := strings.TrimSpace(channel); ch != "" {
		return dokuCustomerFeeForChannel(cfg, ch, base)
	}
	return dokuCustomerFee(cfg, base)
}

// metaInt64 membaca angka dari metadata intent (JSON → biasanya float64).
func metaInt64(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	}
	return 0
}

func metaString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

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

// intentCallbackURL membaca callback webhook PG yang dipakai saat intent dibuat.
func intentCallbackURL(pi *store.PaymentIntent) string {
	if pi == nil || len(pi.Metadata) == 0 {
		return ""
	}
	if s, ok := pi.Metadata["callback_url"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func checkoutInvoice(ctx context.Context, d *Deps, tid xid.ID, inv *store.Invoice, providerName, channel, returnURL, origin string) (*store.PaymentIntent, error) {
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
	channel = strings.ToLower(strings.TrimSpace(channel))
	if providerName == payment.ProviderDoku {
		if channel == "" {
			return nil, httpx.BadRequest("pilih metode pembayaran DOKU (QRIS / VA / e-wallet / retail)")
		}
		if _, ok := payment.LookupDokuChannel(channel); !ok {
			return nil, httpx.BadRequest("channel DOKU tidak dikenali")
		}
	}
	// Biaya admin yang dibebankan ke customer (khusus DOKU per channel).
	baseAmount := amount
	feeAmount := paymentCustomerFee(ctx, d, tid, providerName, channel, baseAmount)
	chargeAmount := baseAmount + feeAmount
	// Target callback webhook PG untuk order ini. Dipakai untuk memutuskan
	// reuse intent: callback yang berubah wajib order baru ke PG.
	wantCallback := ""
	if origin != "" {
		wantCallback = strings.TrimRight(origin, "/") + paymentWebhookPathFor(providerName)
	}
	if existing, err := d.Store.GetLatestPendingPaymentIntent(ctx, tid, inv.ID, providerName); err == nil && existing != nil && existing.Amount == chargeAmount {
		sameChannel := channel == "" || metaString(existing.Metadata, "doku_channel") == channel || metaString(existing.Metadata, "channel") == channel
		hasPay := existing.QRString != "" || strings.TrimSpace(existing.CheckoutURL) != "" || metaString(existing.Metadata, "va_number") != "" || metaString(existing.Metadata, "payment_code") != ""
		if sameChannel && hasPay {
			if storedCB := intentCallbackURL(existing); wantCallback == "" || storedCB == "" || storedCB == wantCallback {
				_ = d.Store.CancelPendingPaymentIntentsExcept(ctx, tid, inv.ID, existing.ExternalID)
				return existing, nil
			}
			slog.Info("payment intent not reused: callback changed",
				"invoice", inv.InvoiceNumber, "provider", providerName)
		}
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, httpx.Internal(err)
	}

	prov, err := resolvePaymentProvider(ctx, d, tid, providerName)
	if err != nil {
		return nil, err
	}
	if providerName == payment.ProviderDoku && channel != "" {
		if cat, ok := payment.LookupDokuChannel(channel); ok && cat.NeedsVABin {
			if doku, ok := prov.(*payment.DokuProvider); ok {
				cfg, _ := loadDokuIntegration(ctx, d, tid)
				bin := dokuVABinForChannel(cfg, channel)
				if bin == "" {
					return nil, httpx.BadRequest("BIN VA untuk " + cat.Label + " belum diisi di Integrasi → DOKU")
				}
				doku.WithPartnerServiceID(bin)
			}
		}
	}
	req := payment.IntentRequest{
		TenantID: tid, CustomerID: inv.CustomerID, InvoiceID: inv.ID,
		Amount: chargeAmount, ReturnURL: returnURL, Channel: channel,
		ProductDetails: "Tagihan " + strings.TrimSpace(inv.InvoiceNumber),
	}
	var ten *store.Tenant
	if t, terr := d.Store.GetTenant(ctx, tid); terr == nil {
		ten = t
	}
	if origin != "" {
		req.CallbackURL = wantCallback
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
	if cb := strings.TrimSpace(req.CallbackURL); cb != "" {
		meta["callback_url"] = cb
	}
	// Simpan tagihan asli & biaya admin agar pelunasan invoice tetap sebesar
	// base (fee tidak dobel dihitung sebagai pembayaran) dan bisa ditelusuri.
	meta["base_amount"] = baseAmount
	if feeAmount > 0 {
		meta["fee_amount"] = feeAmount
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
		Amount:        chargeAmount,
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
	// Direct QRIS: poll via QueryQR, bukan Checkout status API.
	if pi.Provider == payment.ProviderDoku && metaString(pi.Metadata, "doku_kind") == payment.DokuKindQR {
		if doku, ok := func() (*payment.DokuProvider, bool) {
			prov, err := resolvePaymentProvider(ctx, d, tid, pi.Provider)
			if err != nil {
				return nil, false
			}
			p, ok := prov.(*payment.DokuProvider)
			return p, ok
		}(); ok {
			ref := metaString(pi.Metadata, "transaction_id")
			if ref == "" {
				ref = metaString(pi.Metadata, "reference")
			}
			st, amt, qerr := doku.QueryQR(ctx, ref, pi.ExternalID)
			if qerr == nil {
				if st != "" && st != pi.Status {
					_ = d.Store.UpdatePaymentIntentStatus(ctx, pi.ExternalID, st)
					pi.Status = st
				}
				if payment.WebhookIsPaid(st) {
					ev := &payment.WebhookEvent{
						ExternalID: pi.ExternalID,
						Status:     "paid",
						Amount:     amt,
						Reference:  ref,
					}
					if err := completePaidWebhook(ctx, d, pi.Provider, ev); err != nil {
						return nil, httpx.Internal(err)
					}
					pi.Status = "paid"
				}
			}
		}
		return pi, nil
	}
	prov, rerr := resolvePaymentProvider(ctx, d, tid, pi.Provider)
	if rerr != nil {
		slog.Warn("payment status check skipped", "provider", pi.Provider, "external_id", pi.ExternalID, "err", rerr)
	}
	if rerr == nil {
		if checker, ok := prov.(payment.StatusChecker); ok && pi.ExternalID != "" {
			remote, serr := checker.CheckStatus(ctx, pi.ExternalID)
			if serr != nil {
				slog.Warn("payment status check failed", "provider", pi.Provider, "external_id", pi.ExternalID, "err", serr)
			}
			if serr == nil && remote != nil {
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

func sandboxSimExternalID(inv *store.Invoice) string {
	base := ""
	if inv != nil {
		base = strings.TrimSpace(inv.InvoiceNumber)
	}
	if base == "" {
		base = "SIM"
	}
	if len(base) > 28 {
		base = base[:28]
	}
	return base + "-SIM-" + xid.New().String()
}

// simulateSandboxInvoicePayment marks an unpaid invoice paid through the same
// webhook path as Duitku. Only allowed while Duitku sandbox is active — the
// Duitku dashboard has no "mark paid" control.
func simulateSandboxInvoicePayment(ctx context.Context, d *Deps, tid xid.ID, inv *store.Invoice) (*store.PaymentIntent, error) {
	if inv == nil {
		return nil, httpx.NotFound("invoice not found")
	}
	if !duitkuSandboxReady(ctx, d, tid) {
		return nil, httpx.BadRequest("simulasi hanya tersedia saat Duitku sandbox aktif")
	}
	st := strings.ToLower(strings.TrimSpace(inv.Status))
	if st == "paid" || st == "void" || st == "cancelled" {
		return nil, httpx.BadRequest("invoice sudah lunas / tidak bisa dibayar")
	}
	amount := invoiceRemaining(inv)
	if amount <= 0 {
		return nil, httpx.BadRequest("tidak ada sisa tagihan")
	}

	var pi *store.PaymentIntent
	existing, err := d.Store.GetLatestPendingPaymentIntent(ctx, tid, inv.ID, payment.ProviderDuitku)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, httpx.Internal(err)
	}
	if existing != nil && !payment.WebhookIsPaid(existing.Status) {
		pi = existing
	}
	if pi == nil {
		latest, lerr := d.Store.GetLatestPaymentIntentForInvoice(ctx, tid, inv.ID, payment.ProviderDuitku)
		if lerr != nil && !errors.Is(lerr, store.ErrNotFound) {
			return nil, httpx.Internal(lerr)
		}
		if latest != nil && !payment.WebhookIsPaid(latest.Status) {
			status := strings.ToLower(strings.TrimSpace(latest.Status))
			if status != "cancelled" && status != "canceled" && status != "expired" {
				pi = latest
			}
		}
	}
	if pi == nil {
		created, ierr := d.Store.InsertPaymentIntent(ctx, &store.PaymentIntent{
			TenantID:   tid,
			CustomerID: inv.CustomerID,
			InvoiceID:  &inv.ID,
			Provider:   payment.ProviderDuitku,
			ExternalID: sandboxSimExternalID(inv),
			Amount:     amount,
			Status:     "pending",
			Metadata:   map[string]any{"sandbox_sim": true, "duitku_sandbox": true},
		})
		if ierr != nil {
			return nil, httpx.Internal(ierr)
		}
		pi = created
	}

	ev := &payment.WebhookEvent{
		ExternalID: pi.ExternalID,
		Status:     "paid",
		Amount:     amount,
		Reference:  "sandbox-sim",
	}
	if err := completePaidWebhook(ctx, d, payment.ProviderDuitku, ev); err != nil {
		return nil, httpx.Internal(err)
	}
	pi.Status = "paid"
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
