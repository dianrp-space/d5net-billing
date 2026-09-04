package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/invoice"
	"github.com/dianrp/drp-billing/internal/payment"
	"github.com/dianrp/drp-billing/internal/provision"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/skip2/go-qrcode"
	"github.com/xuri/excelize/v2"
)

func registerReports(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "business-report", Method: http.MethodGet, Path: "/api/reports/business",
		Tags: []string{"Reports"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body map[string]any }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stats, err := d.Store.DashboardStats(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		var mrr, arpu float64
		active := toInt64(stats["active_subscriptions"])
		rev := toInt64(stats["monthly_revenue"])
		if active > 0 {
			arpu = float64(rev) / float64(active)
		}
		mrr = float64(rev)
		stats["mrr"] = mrr
		stats["arpu"] = arpu
		aging, _ := d.Store.AgingReceivable(ctx, tid)
		stats["aging"] = aging
		return &struct{ Body map[string]any }{Body: stats}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "export-invoices-csv", Method: http.MethodGet, Path: "/api/reports/invoices.csv",
		Tags: []string{"Reports"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		ContentType string `header:"Content-Type"`
		Body        string
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, _, err := d.Store.ListInvoices(ctx, tid, "", 1000, 0)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := "invoice_number,customer,total,status,due_date\n"
		for _, inv := range list {
			out += fmt.Sprintf("%s,%s,%d,%s,%s\n", inv.InvoiceNumber, inv.CustomerName, inv.TotalAmount, inv.Status, inv.DueDate.Format("2006-01-02"))
		}
		return &struct {
			ContentType string `header:"Content-Type"`
			Body        string
		}{ContentType: "text/csv", Body: out}, nil
	})
}

func registerAccounting(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-accounts", Method: http.MethodGet, Path: "/api/accounting/accounts",
		Tags: []string{"Accounting"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.Account }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListAccounts(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.Account }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-expense", Method: http.MethodPost, Path: "/api/accounting/expenses",
		Tags: []string{"Accounting"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Amount      int64  `json:"amount"`
			Category    string `json:"category"`
			Description string `json:"description,omitempty"`
			ExpenseDate string `json:"expense_date,omitempty"`
		}
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.CreateExpense(ctx, tid, input.Body.Amount, input.Body.Category, input.Body.Description, input.Body.ExpenseDate); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "ok"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "pnl-report", Method: http.MethodGet, Path: "/api/accounting/pnl",
		Tags: []string{"Accounting"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body map[string]any }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		pnl, err := d.Store.ProfitAndLoss(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]any }{Body: pnl}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashflow-report", Method: http.MethodGet, Path: "/api/accounting/cashflow",
		Tags: []string{"Accounting"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Months int `query:"months"`
	}) (*struct{ Body []map[string]any }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		data, err := d.Store.CashFlow(ctx, tid, input.Months)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []map[string]any }{Body: data}, nil
	})
}

func registerAdvanced(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "backup-router", Method: http.MethodPost, Path: "/api/routers/{id}/backup",
		Tags: []string{"Routers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		r, err := d.Store.GetRouter(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("router not found")
		}
		prov, err := d.Provisioner.Get(r.Provisioner)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		content, err := prov.BackupConfig(ctx, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		sum := sha256.Sum256([]byte(content))
		filename := fmt.Sprintf("%s-%d.rsc", r.Name, time.Now().Unix())
		if err := d.Store.SaveRouterBackup(ctx, tid, input.ID, filename, content, hex.EncodeToString(sum[:])); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"filename": filename}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "invoice-pdf", Method: http.MethodGet, Path: "/api/invoices/{id}/pdf",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		ContentType string `header:"Content-Type"`
		Body        []byte
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		inv, items, err := d.Store.GetInvoice(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		return &struct {
			ContentType string `header:"Content-Type"`
			Body        []byte
		}{ContentType: "application/pdf", Body: invoice.RenderPDF(inv, items)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-api-key", Method: http.MethodPost, Path: "/api/settings/api-keys",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name string `json:"name"`
		}
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		raw := fmt.Sprintf("drp_%d_%d", tid, time.Now().UnixNano())
		sum := sha256.Sum256([]byte(raw))
		_, err = d.Store.Pool.Exec(ctx, `INSERT INTO api_keys (tenant_id, name, key_hash) VALUES ($1,$2,$3)`, tid, input.Body.Name, hex.EncodeToString(sum[:]))
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"key": raw}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-work-order", Method: http.MethodPost, Path: "/api/work-orders",
		Tags: []string{"WorkOrders"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Type         string  `json:"type"`
			Notes        *string `json:"notes,omitempty"`
			CustomerID   *xid.ID `json:"customer_id,omitempty"`
			TechnicianID *xid.ID `json:"technician_id,omitempty"`
			Status       string  `json:"status,omitempty"`
		}
	}) (*struct{ Body store.WorkOrder }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		typ := strings.TrimSpace(input.Body.Type)
		if typ == "" {
			return nil, httpx.BadRequest("type is required")
		}
		wo := store.WorkOrder{
			TenantID: tid, CustomerID: input.Body.CustomerID, TechnicianID: input.Body.TechnicianID,
			Type: typ, Notes: input.Body.Notes, Status: input.Body.Status,
		}
		if wo.Status == "" {
			wo.Status = "pending"
		}
		if err := d.Store.CreateWorkOrder(ctx, &wo); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.WorkOrder }{Body: wo}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "checkin-work-order", Method: http.MethodPost, Path: "/api/work-orders/{id}/check-in",
		Tags: []string{"WorkOrders"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		}
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.CheckInWorkOrder(ctx, tid, input.ID, input.Body.Lat, input.Body.Lng); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "in_progress"}}, nil
	})
}

func registerOpsExtra(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-leads", Method: http.MethodGet, Path: "/api/leads",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Limit  int `query:"limit"`
		Offset int `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.Lead `json:"data"`
			Total int64        `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, total, err := d.Store.ListLeads(ctx, tid, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Data  []store.Lead `json:"data"`
				Total int64        `json:"total"`
			}
		}{}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-lead", Method: http.MethodPost, Path: "/api/leads",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			FullName  string   `json:"full_name,omitempty"`
			Name      string   `json:"name,omitempty"` // alias from UI
			Phone     string   `json:"phone,omitempty"`
			Email     *string  `json:"email,omitempty"`
			Address   *string  `json:"address,omitempty"`
			Latitude  *float64 `json:"latitude,omitempty"`
			Longitude *float64 `json:"longitude,omitempty"`
			ODPID     *xid.ID  `json:"odp_id,omitempty"`
			Status    string   `json:"status,omitempty"`
			Notes     *string  `json:"notes,omitempty"`
		}
	}) (*struct{ Body store.Lead }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		fullName := strings.TrimSpace(input.Body.FullName)
		if fullName == "" {
			fullName = strings.TrimSpace(input.Body.Name)
		}
		if fullName == "" {
			return nil, httpx.BadRequest("full_name is required")
		}
		l := store.Lead{
			TenantID: tid, FullName: fullName, Phone: strings.TrimSpace(input.Body.Phone),
			Email: input.Body.Email, Address: input.Body.Address,
			Latitude: input.Body.Latitude, Longitude: input.Body.Longitude,
			ODPID: input.Body.ODPID, Status: input.Body.Status, Notes: input.Body.Notes,
		}
		if err := d.Store.CreateLead(ctx, &l); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Lead }{Body: l}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-resellers", Method: http.MethodGet, Path: "/api/resellers",
		Tags: []string{"Resellers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.Reseller }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListResellers(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.Reseller }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-reseller", Method: http.MethodPost, Path: "/api/resellers",
		Tags: []string{"Resellers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name              string  `json:"name"`
			Phone             string  `json:"phone"`
			CommissionPercent float64 `json:"commission_percent"`
		}
	}) (*struct{ Body store.Reseller }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		phone := input.Body.Phone
		r := store.Reseller{TenantID: tid, Name: input.Body.Name, Phone: &phone, CommissionPercent: input.Body.CommissionPercent}
		if err := d.Store.CreateResellerFull(ctx, &r); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Reseller }{Body: r}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-alerts", Method: http.MethodGet, Path: "/api/alerts",
		Tags: []string{"Alerts"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Limit int `query:"limit"`
	}) (*struct{ Body []store.Alert }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListAlerts(ctx, tid, input.Limit)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.Alert }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "reconcile-router", Method: http.MethodPost, Path: "/api/routers/{id}/reconcile",
		Tags: []string{"Routers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID    xid.ID `path:"id"`
		Apply bool   `query:"apply"`
	}) (*struct{ Body []provision.Drift }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		r, err := d.Store.GetRouter(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("router not found")
		}
		prov, err := d.Provisioner.Get(r.Provisioner)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		drifts, err := provision.Reconcile(ctx, d.Store, prov, tid, input.ID, input.Apply)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if drifts == nil {
			drifts = []provision.Drift{}
		}
		return &struct{ Body []provision.Drift }{Body: drifts}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "invoice-checkout", Method: http.MethodPost, Path: "/api/invoices/{id}/checkout",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Provider  string `json:"provider"`
			ReturnURL string `json:"return_url"`
		}
	}) (*struct{ Body store.PaymentIntent }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		inv, _, err := d.Store.GetInvoice(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		providerName := input.Body.Provider
		if providerName == "" {
			providerName = "manual"
		}
		prov, err := resolvePaymentProvider(ctx, d, tid, providerName)
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		amount := inv.TotalAmount - inv.PaidAmount
		if amount < 0 {
			amount = 0
		}
		res, err := prov.CreateIntent(ctx, payment.IntentRequest{
			TenantID: tid, CustomerID: inv.CustomerID, InvoiceID: inv.ID,
			Amount: amount, ReturnURL: input.Body.ReturnURL,
		})
		if err != nil {
			return nil, httpx.Internal(err)
		}
		pi, err := d.Store.InsertPaymentIntent(ctx, tid, inv.CustomerID, &inv.ID, providerName, res.ExternalID, amount, res.Status, res.CheckoutURL)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.PaymentIntent }{Body: *pi}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-outbound-webhooks", Method: http.MethodGet, Path: "/api/outbound-webhooks",
		Tags: []string{"Webhooks"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.OutboundWebhook }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListOutboundWebhooks(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.OutboundWebhook }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-outbound-webhook", Method: http.MethodPost, Path: "/api/outbound-webhooks",
		Tags: []string{"Webhooks"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct{ Body store.OutboundWebhook }) (*struct{ Body store.OutboundWebhook }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		w := input.Body
		w.TenantID = tid
		if err := d.Store.CreateOutboundWebhook(ctx, &w); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.OutboundWebhook }{Body: w}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-outbound-webhook", Method: http.MethodDelete, Path: "/api/outbound-webhooks/{id}",
		Tags: []string{"Webhooks"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteOutboundWebhook(ctx, tid, input.ID); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "export-invoices-xlsx", Method: http.MethodGet, Path: "/api/reports/invoices.xlsx",
		Tags: []string{"Reports"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               []byte
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, _, err := d.Store.ListInvoices(ctx, tid, "", 5000, 0)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		f := excelize.NewFile()
		sheet := "Invoices"
		_ = f.SetSheetName("Sheet1", sheet)
		_ = f.SetSheetRow(sheet, "A1", &[]any{"invoice_number", "customer", "total", "status", "due_date"})
		for i, inv := range list {
			cell := fmt.Sprintf("A%d", i+2)
			_ = f.SetSheetRow(sheet, cell, &[]any{inv.InvoiceNumber, inv.CustomerName, inv.TotalAmount, inv.Status, inv.DueDate.Format("2006-01-02")})
		}
		buf, err := f.WriteToBuffer()
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct {
			ContentType        string `header:"Content-Type"`
			ContentDisposition string `header:"Content-Disposition"`
			Body               []byte
		}{
			ContentType:        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			ContentDisposition: `attachment; filename="invoices.xlsx"`,
			Body:               buf.Bytes(),
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "voucher-batch-qr", Method: http.MethodGet, Path: "/api/vouchers/batches/{id}/qr",
		Tags: []string{"Vouchers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		ContentType string `header:"Content-Type"`
		Body        []byte
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		codes, err := d.Store.ListVoucherCodes(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if len(codes) == 0 {
			return nil, httpx.NotFound("no vouchers in batch")
		}
		png, err := qrcode.Encode(codes[0], qrcode.Medium, 256)
		if err != nil {
			// Fallback: plain text list of codes
			text := strings.Join(codes, "\n")
			return &struct {
				ContentType string `header:"Content-Type"`
				Body        []byte
			}{ContentType: "text/plain", Body: []byte(text)}, nil
		}
		return &struct {
			ContentType string `header:"Content-Type"`
			Body        []byte
		}{ContentType: "image/png", Body: png}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "churn-report", Method: http.MethodGet, Path: "/api/reports/churn",
		Tags: []string{"Reports"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body map[string]any }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		rep, err := d.Store.ChurnReport(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]any }{Body: rep}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "subscription-usage", Method: http.MethodGet, Path: "/api/subscriptions/{id}/usage",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID    xid.ID `path:"id"`
		Limit int    `query:"limit"`
	}) (*struct{ Body []store.TrafficSample }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := d.Store.GetSubscription(ctx, tid, input.ID); err != nil {
			return nil, httpx.NotFound("subscription not found")
		}
		samples, err := d.Store.ListTrafficSamples(ctx, tid, input.ID, input.Limit)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if samples == nil {
			samples = []store.TrafficSample{}
		}
		return &struct{ Body []store.TrafficSample }{Body: samples}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "bulk-config-push", Method: http.MethodPost, Path: "/api/routers/bulk-push",
		Tags: []string{"Routers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			RouterIDs []xid.ID `json:"router_ids"`
			Command   string   `json:"command"`
			DryRun    bool     `json:"dry_run"`
		}
	}) (*struct{ Body map[string]any }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cmd := strings.TrimSpace(input.Body.Command)
		if cmd == "" {
			return nil, httpx.BadRequest("command required")
		}
		results := make([]map[string]any, 0, len(input.Body.RouterIDs))
		for _, id := range input.Body.RouterIDs {
			r, err := d.Store.GetRouter(ctx, tid, id)
			item := map[string]any{"router_id": id}
			if err != nil {
				item["ok"] = false
				item["error"] = "router not found"
				results = append(results, item)
				continue
			}
			item["name"] = r.Name
			if input.Body.DryRun {
				item["ok"] = true
				item["preview"] = cmd
				results = append(results, item)
				continue
			}
			prov, err := d.Provisioner.Get(r.Provisioner)
			if err != nil {
				item["ok"] = false
				item["error"] = err.Error()
				results = append(results, item)
				continue
			}
			_, err = prov.TestConnection(ctx, id)
			if err != nil {
				item["ok"] = false
				item["error"] = err.Error()
			} else {
				item["ok"] = true
				item["message"] = "connected; command queued for operator review (not auto-exec raw CLI)"
			}
			_ = d.Store.LogRouterCommand(ctx, tid, id, nil, cmd, fmt.Sprintf("bulk dry=%v", input.Body.DryRun), err == nil)
			results = append(results, item)
		}
		return &struct{ Body map[string]any }{Body: map[string]any{"results": results, "dry_run": input.Body.DryRun}}, nil
	})
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	default:
		return 0
	}
}
