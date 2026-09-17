package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/billing"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/payment"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// autoSettleCustomerWallet melunasi tagihan menunggak pelanggan dari saldo
// (dipakai setelah topup). Kirim konfirmasi per tagihan lunas & pulihkan layanan.
func autoSettleCustomerWallet(ctx context.Context, d *Deps, tid, customerID xid.ID) []billing.WalletAutoPayResult {
	if d == nil || d.Billing == nil {
		return nil
	}
	results, err := d.Billing.AutoSettleCustomer(ctx, tid, customerID)
	if err != nil {
		return nil
	}
	for _, r := range results {
		if !r.Paid || r.Invoice == nil {
			continue
		}
		if r.Customer != nil && r.Payment != nil && d.Notify != nil {
			planName := d.Store.PlanNameForSubscription(ctx, tid, r.Invoice.SubscriptionID)
			itemName := store.NotificationItemName(planName, r.Items)
			_ = d.Notify.SendPaymentConfirmation(ctx, tid, r.Customer.Phone, r.Customer.FullName, planName, itemName, r.Invoice.InvoiceNumber, r.Payment.Amount)
		}
		resumeAfterInvoicePaid(ctx, d, tid, r.Invoice)
	}
	return results
}

// checkoutWalletTopup membuat payment intent topup saldo (tanpa invoice).
func checkoutWalletTopup(ctx context.Context, d *Deps, tid, customerID xid.ID, amount int64, providerName, returnURL, origin string) (*store.PaymentIntent, error) {
	if amount <= 0 {
		return nil, httpx.BadRequest("nominal topup tidak valid")
	}
	if min := d.Store.WalletMinTopup(ctx, tid); amount < min {
		return nil, httpx.BadRequest("minimal topup Rp " + strconv.FormatInt(min, 10))
	}
	providerName = normalizePaymentProviderName(providerName)
	if providerName == payment.ProviderManual {
		return nil, httpx.BadRequest("gunakan pembayaran online")
	}
	prov, err := resolvePaymentProvider(ctx, d, tid, providerName)
	if err != nil {
		return nil, err
	}
	cust, err := d.Store.GetCustomer(ctx, tid, customerID)
	if err != nil {
		return nil, httpx.NotFound("pelanggan tidak ditemukan")
	}
	wantCallback := ""
	if origin != "" {
		wantCallback = strings.TrimRight(origin, "/") + paymentWebhookPathFor(providerName)
	}
	req := payment.IntentRequest{
		TenantID:        tid,
		CustomerID:      customerID,
		Amount:          amount,
		ReturnURL:       returnURL,
		ProductDetails:  "Topup saldo",
		CustomerName:    strings.TrimSpace(cust.FullName),
		Phone:           strings.TrimSpace(cust.Phone),
		MerchantOrderID: "TOPUP-" + strings.TrimSpace(cust.CustomerCode) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10),
	}
	if origin != "" {
		req.CallbackURL = wantCallback
		if req.ReturnURL == "" {
			req.ReturnURL = origin
		}
	}
	if cust.Email != nil {
		req.Email = strings.TrimSpace(*cust.Email)
	}
	if req.Email == "" {
		if ten, terr := d.Store.GetTenant(ctx, tid); terr == nil && ten.Email != nil {
			req.Email = strings.TrimSpace(*ten.Email)
		}
	}
	if providerName == payment.ProviderDuitku && req.CallbackURL == "" {
		return nil, httpx.BadRequest("callback Duitku membutuhkan URL publik")
	}
	res, err := prov.CreateIntent(ctx, req)
	if err != nil {
		return nil, httpx.BadRequest(err.Error())
	}
	meta := map[string]any{
		"purpose":      "wallet_topup",
		"base_amount":  amount,
		"topup_amount": amount,
	}
	if res.TransactionID != "" {
		meta["transaction_id"] = res.TransactionID
	}
	if cb := strings.TrimSpace(req.CallbackURL); cb != "" {
		meta["callback_url"] = cb
	}
	for k, v := range res.Metadata {
		meta[k] = v
	}
	pi := &store.PaymentIntent{
		TenantID:      tid,
		CustomerID:    customerID,
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
	return d.Store.InsertPaymentIntent(ctx, pi)
}

// completeWalletTopup menambah saldo dari webhook topup yang sudah lunas,
// mengirim notifikasi topup, lalu melunasi tagihan menunggak dari saldo.
func completeWalletTopup(ctx context.Context, d *Deps, pi *store.PaymentIntent, event *payment.WebhookEvent) error {
	amount := event.Amount
	if amount <= 0 {
		amount = metaInt64(pi.Metadata, "topup_amount")
	}
	if amount <= 0 {
		amount = pi.Amount
	}
	ref := event.Reference
	if ref == "" {
		ref = event.ExternalID
	}
	balance, err := d.Store.TopupWallet(ctx, pi.TenantID, pi.CustomerID, amount, "topup", ref, "Topup saldo via "+pi.Provider)
	if err != nil {
		return err
	}
	if cust, cerr := d.Store.GetCustomer(ctx, pi.TenantID, pi.CustomerID); cerr == nil && d.Notify != nil {
		_ = d.Notify.SendWalletTopup(ctx, pi.TenantID, cust.Phone, cust.FullName, amount, balance)
	}
	autoSettleCustomerWallet(ctx, d, pi.TenantID, pi.CustomerID)
	return nil
}

func walletEnabledOrForbidden(ctx context.Context, d *Deps, tid xid.ID) error {
	if !d.Store.WalletEnabled(ctx, tid) {
		return httpx.BadRequest("fitur saldo tidak aktif")
	}
	return nil
}

type walletView struct {
	Enabled      bool              `json:"enabled"`
	Balance      int64             `json:"balance"`
	MinTopup     int64             `json:"min_topup"`
	Transactions []store.WalletTxn `json:"transactions"`
}

// portalWalletOwner memilih pemilik saldo portal (akun utama login).
func portalWalletOwner(custs []*store.Customer) *store.Customer {
	if len(custs) == 0 {
		return nil
	}
	return custs[0]
}

func registerWallet(api huma.API, d *Deps) {
	// Flag fitur untuk UI admin/pelanggan (tanpa permission khusus settings).
	huma.Register(api, huma.Operation{
		OperationID: "features", Method: http.MethodGet, Path: "/api/features",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			WalletEnabled  bool  `json:"wallet_enabled"`
			WalletMinTopup int64 `json:"wallet_min_topup"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				WalletEnabled  bool  `json:"wallet_enabled"`
				WalletMinTopup int64 `json:"wallet_min_topup"`
			}
		}{}
		out.Body.WalletEnabled = d.Store.WalletEnabled(ctx, tid)
		out.Body.WalletMinTopup = d.Store.WalletMinTopup(ctx, tid)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-wallet", Method: http.MethodGet, Path: "/api/portal/wallet",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization"`
	}) (*struct{ Body walletView }, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		owner := portalWalletOwner(custs)
		if owner == nil {
			return nil, httpx.NotFound("akun tidak ditemukan")
		}
		balance, _ := d.Store.GetWallet(ctx, ten.ID, owner.ID)
		txns, _, _ := d.Store.ListWalletTxns(ctx, ten.ID, owner.ID, 50, 0)
		return &struct{ Body walletView }{Body: walletView{
			Enabled:      d.Store.WalletEnabled(ctx, ten.ID),
			Balance:      balance,
			MinTopup:     d.Store.WalletMinTopup(ctx, ten.ID),
			Transactions: txns,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-wallet-topup", Method: http.MethodPost, Path: "/api/portal/wallet/topup",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Authorization   string `header:"Authorization"`
		Host            string `header:"Host"`
		Origin          string `header:"Origin"`
		Referer         string `header:"Referer"`
		XForwardedHost  string `header:"X-Forwarded-Host"`
		XForwardedProto string `header:"X-Forwarded-Proto"`
		Body            *struct {
			Amount    int64  `json:"amount"`
			Provider  string `json:"provider,omitempty"`
			ReturnURL string `json:"return_url,omitempty"`
		}
	}) (*struct{ Body store.PaymentIntent }, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		if err := walletEnabledOrForbidden(ctx, d, ten.ID); err != nil {
			return nil, err
		}
		owner := portalWalletOwner(custs)
		if owner == nil {
			return nil, httpx.NotFound("akun tidak ditemukan")
		}
		var amount int64
		providerName := ""
		returnURL := ""
		if input.Body != nil {
			amount = input.Body.Amount
			providerName = strings.TrimSpace(input.Body.Provider)
			returnURL = strings.TrimSpace(input.Body.ReturnURL)
		}
		if providerName == "" {
			opts := listEnabledPayOptions(ctx, d, ten.ID)
			if len(opts) == 0 {
				return nil, httpx.BadRequest("belum ada metode pembayaran online yang aktif")
			}
			providerName = opts[0].Provider
		}
		origin := appPublicOrigin(ctx, d, ten.ID, input.Origin, input.Referer, input.XForwardedProto, input.XForwardedHost, input.Host)
		if returnURL == "" {
			returnURL = origin
		}
		pi, err := checkoutWalletTopup(ctx, d, ten.ID, owner.ID, amount, providerName, returnURL, origin)
		if err != nil {
			return nil, err
		}
		return &struct{ Body store.PaymentIntent }{Body: *pi}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-wallet-topup-status", Method: http.MethodGet, Path: "/api/portal/wallet/topup/{external_id}",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ExternalID    string `path:"external_id"`
		Authorization string `header:"Authorization"`
	}) (*struct{ Body store.PaymentIntent }, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		pi, err := d.Store.GetPaymentIntentByExternalID(ctx, strings.TrimSpace(input.ExternalID))
		if err != nil {
			return nil, httpx.NotFound("pembayaran tidak ditemukan")
		}
		allowed := false
		for _, c := range custs {
			if c.ID == pi.CustomerID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, httpx.NotFound("pembayaran tidak ditemukan")
		}
		synced, err := syncIntentStatus(ctx, d, ten.ID, pi)
		if err != nil {
			return nil, err
		}
		return &struct{ Body store.PaymentIntent }{Body: *synced}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "customer-wallet", Method: http.MethodGet, Path: "/api/customers/{id}/wallet",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body walletView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := d.Store.GetCustomer(ctx, tid, input.ID); err != nil {
			return nil, httpx.NotFound("pelanggan tidak ditemukan")
		}
		balance, _ := d.Store.GetWallet(ctx, tid, input.ID)
		txns, _, _ := d.Store.ListWalletTxns(ctx, tid, input.ID, 50, 0)
		return &struct{ Body walletView }{Body: walletView{
			Enabled:      d.Store.WalletEnabled(ctx, tid),
			Balance:      balance,
			MinTopup:     d.Store.WalletMinTopup(ctx, tid),
			Transactions: txns,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "customer-wallet-topup", Method: http.MethodPost, Path: "/api/customers/{id}/wallet/topup",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Amount int64  `json:"amount"`
			Note   string `json:"note,omitempty"`
		}
	}) (*struct {
		Body struct {
			Balance int64 `json:"balance"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cust, err := d.Store.GetCustomer(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("pelanggan tidak ditemukan")
		}
		if input.Body.Amount <= 0 {
			return nil, httpx.BadRequest("nominal topup harus > 0")
		}
		note := strings.TrimSpace(input.Body.Note)
		if note == "" {
			note = "Topup manual admin"
		}
		balance, err := d.Store.TopupWallet(ctx, tid, input.ID, input.Body.Amount, "topup_admin", "ADMIN", note)
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		if d.Notify != nil {
			_ = d.Notify.SendWalletTopup(ctx, tid, cust.Phone, cust.FullName, input.Body.Amount, balance)
		}
		autoSettleCustomerWallet(ctx, d, tid, input.ID)
		if bal, berr := d.Store.GetWallet(ctx, tid, input.ID); berr == nil {
			balance = bal
		}
		out := &struct {
			Body struct {
				Balance int64 `json:"balance"`
			}
		}{}
		out.Body.Balance = balance
		return out, nil
	})

	// Pelanggan membayar tagihan (mis. tagihan manual) memakai saldo.
	huma.Register(api, huma.Operation{
		OperationID: "portal-invoice-pay-wallet", Method: http.MethodPost, Path: "/api/portal/invoices/{id}/pay-with-wallet",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
	}) (*struct {
		Body struct {
			Paid    bool  `json:"paid"`
			Balance int64 `json:"balance"`
		}
	}, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		if err := walletEnabledOrForbidden(ctx, d, ten.ID); err != nil {
			return nil, err
		}
		inv, items, err := d.Store.GetInvoice(ctx, ten.ID, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		allowed := false
		for _, c := range custs {
			if c.ID == inv.CustomerID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, httpx.NotFound("invoice not found")
		}
		payment, paid, err := d.Store.PayInvoiceFromWallet(ctx, ten.ID, inv.CustomerID, inv.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Paid    bool  `json:"paid"`
				Balance int64 `json:"balance"`
			}
		}{}
		if bal, berr := d.Store.GetWallet(ctx, ten.ID, inv.CustomerID); berr == nil {
			out.Body.Balance = bal
		}
		if !paid || payment == nil {
			return out, nil
		}
		out.Body.Paid = true
		if cust, cerr := d.Store.GetCustomer(ctx, ten.ID, inv.CustomerID); cerr == nil && d.Notify != nil {
			planName := d.Store.PlanNameForSubscription(ctx, ten.ID, inv.SubscriptionID)
			itemName := store.NotificationItemName(planName, items)
			_ = d.Notify.SendPaymentConfirmation(ctx, ten.ID, cust.Phone, cust.FullName, planName, itemName, inv.InvoiceNumber, payment.Amount)
		}
		resumeAfterInvoicePaid(ctx, d, ten.ID, inv)
		return out, nil
	})

	// Riwayat topup saldo (isi saldo) lintas pelanggan untuk halaman Pembayaran.
	huma.Register(api, huma.Operation{
		OperationID: "list-wallet-topups", Method: http.MethodGet, Path: "/api/wallet/topups",
		Tags: []string{"Payments"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Search string `query:"search"`
		Limit  int    `query:"limit"`
		Offset int    `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.WalletTopup `json:"data"`
			Total int64               `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				Data  []store.WalletTopup `json:"data"`
				Total int64               `json:"total"`
			}
		}{}
		if !d.Store.WalletEnabled(ctx, tid) {
			out.Body.Data = []store.WalletTopup{}
			return out, nil
		}
		list, total, err := d.Store.ListWalletTopups(ctx, tid, input.Search, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})
}
