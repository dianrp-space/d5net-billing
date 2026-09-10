package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/notify"
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
	}, func(ctx context.Context, input *struct {
		Status string `query:"status"`
		Search string `query:"search"`
	}) (*struct {
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               []byte
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, _, err := d.Store.ListInvoices(ctx, tid, strings.TrimSpace(input.Status), strings.TrimSpace(input.Search), false, 100000, 0)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write([]string{"invoice_number", "customer", "due_date", "total", "paid_amount", "outstanding", "status"})
		for _, inv := range list {
			outstanding := inv.TotalAmount - inv.PaidAmount
			_ = w.Write([]string{
				inv.InvoiceNumber, inv.CustomerName, inv.DueDate.Format("2006-01-02"),
				strconv.FormatInt(inv.TotalAmount, 10), strconv.FormatInt(inv.PaidAmount, 10),
				strconv.FormatInt(outstanding, 10), inv.Status,
			})
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct {
			ContentType        string `header:"Content-Type"`
			ContentDisposition string `header:"Content-Disposition"`
			Body               []byte
		}{
			ContentType:        "text/csv; charset=utf-8",
			ContentDisposition: `attachment; filename="invoices.csv"`,
			Body:               buf.Bytes(),
		}, nil
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
		if input.Body.Amount <= 0 {
			return nil, httpx.BadRequest("nominal beban harus > 0")
		}
		if err := d.Store.CreateExpense(ctx, tid, input.Body.Amount, input.Body.Category, input.Body.Description, input.Body.ExpenseDate); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "ok"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-expenses", Method: http.MethodGet, Path: "/api/accounting/expenses",
		Tags: []string{"Accounting"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Limit  int `query:"limit"`
		Offset int `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.Expense `json:"data"`
			Total int64           `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, total, err := d.Store.ListExpenses(ctx, tid, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.Expense{}
		}
		return &struct {
			Body struct {
				Data  []store.Expense `json:"data"`
				Total int64           `json:"total"`
			}
		}{Body: struct {
			Data  []store.Expense `json:"data"`
			Total int64           `json:"total"`
		}{Data: list, Total: total}}, nil
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
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               []byte
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		inv, items, err := d.Store.GetInvoiceIncludingDeleted(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		return &struct {
			ContentType        string `header:"Content-Type"`
			ContentDisposition string `header:"Content-Disposition"`
			Body               []byte
		}{
			ContentType:        "application/pdf",
			ContentDisposition: fmt.Sprintf(`attachment; filename="%s.pdf"`, inv.InvoiceNumber),
			Body:               renderInvoicePDF(ctx, d, tid, inv, items),
		}, nil
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
		OperationID: "list-technicians", Method: http.MethodGet, Path: "/api/technicians",
		Tags: []string{"Technicians"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ActiveOnly bool `query:"active_only"`
	}) (*struct{ Body []store.Technician }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListTechnicians(ctx, tid, input.ActiveOnly)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.Technician{}
		}
		return &struct{ Body []store.Technician }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-technician", Method: http.MethodPost, Path: "/api/technicians",
		Tags: []string{"Technicians"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			FullName string  `json:"full_name"`
			Phone    string  `json:"phone"`
			UserID   *xid.ID `json:"user_id,omitempty"`
			IsActive *bool   `json:"is_active,omitempty"`
		}
	}) (*struct{ Body store.Technician }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.FullName)
		phone := strings.TrimSpace(input.Body.Phone)
		if name == "" || phone == "" {
			return nil, httpx.BadRequest("nama dan telepon wajib")
		}
		active := true
		if input.Body.IsActive != nil {
			active = *input.Body.IsActive
		}
		t := store.Technician{
			TenantID: tid, UserID: input.Body.UserID, FullName: name, Phone: phone, IsActive: active,
		}
		if err := d.Store.CreateTechnician(ctx, &t); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Technician }{Body: t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-technician", Method: http.MethodPatch, Path: "/api/technicians/{id}",
		Tags: []string{"Technicians"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			FullName string  `json:"full_name"`
			Phone    string  `json:"phone"`
			UserID   *xid.ID `json:"user_id,omitempty"`
			IsActive bool    `json:"is_active"`
		}
	}) (*struct{ Body store.Technician }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.FullName)
		phone := strings.TrimSpace(input.Body.Phone)
		if name == "" || phone == "" {
			return nil, httpx.BadRequest("nama dan telepon wajib")
		}
		t := store.Technician{
			ID: input.ID, TenantID: tid, UserID: input.Body.UserID,
			FullName: name, Phone: phone, IsActive: input.Body.IsActive,
		}
		if err := d.Store.UpdateTechnician(ctx, &t); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("technician not found")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Technician }{Body: t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-work-orders", Method: http.MethodGet, Path: "/api/work-orders",
		Tags: []string{"WorkOrders"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status       string `query:"status"`
		TechnicianID string `query:"technician_id"`
		Limit        int    `query:"limit"`
		Offset       int    `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.WorkOrder `json:"data"`
			Total int64             `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		mineOnly, myTechID, err := fieldOpsTechnicianFilter(ctx, d, tid)
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				Data  []store.WorkOrder `json:"data"`
				Total int64             `json:"total"`
			}
		}{}
		if mineOnly && myTechID == nil {
			out.Body.Data = []store.WorkOrder{}
			return out, nil
		}
		techID, err := optionalQueryID(input.TechnicianID)
		if err != nil {
			return nil, httpx.BadRequest("technician_id tidak valid")
		}
		if mineOnly {
			techID = myTechID
		}
		list, total, err := d.Store.ListWorkOrders(ctx, tid, input.Status, techID, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.WorkOrder{}
		}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-work-order", Method: http.MethodGet, Path: "/api/work-orders/{id}",
		Tags: []string{"WorkOrders"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.WorkOrder }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		wo, err := d.Store.GetWorkOrder(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("work order not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceWorkOrderAssigned(ctx, d, tid, wo); err != nil {
			return nil, err
		}
		return &struct{ Body store.WorkOrder }{Body: *wo}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-work-order", Method: http.MethodPost, Path: "/api/work-orders",
		Tags: []string{"WorkOrders"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Type         string     `json:"type"`
			Notes        *string    `json:"notes,omitempty"`
			CustomerID   *xid.ID    `json:"customer_id,omitempty"`
			TechnicianID *xid.ID    `json:"technician_id,omitempty"`
			ScheduledAt  *time.Time `json:"scheduled_at,omitempty"`
			Status       string     `json:"status,omitempty"`
		}
	}) (*struct{ Body store.WorkOrder }, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		typ := store.NormalizeWorkOrderType(input.Body.Type)
		if typ == "" {
			return nil, httpx.BadRequest("type wajib: installation, repair, survey, maintenance, other")
		}
		status := store.NormalizeWorkOrderStatus(input.Body.Status)
		wo := store.WorkOrder{
			TenantID: tid, CustomerID: input.Body.CustomerID, TechnicianID: input.Body.TechnicianID,
			Type: typ, Notes: input.Body.Notes, Status: status, ScheduledAt: input.Body.ScheduledAt,
		}
		if err := d.Store.CreateWorkOrder(ctx, &wo); err != nil {
			return nil, httpx.Internal(err)
		}
		full, err := d.Store.GetWorkOrder(ctx, tid, wo.ID)
		if err != nil {
			_ = d.Notify.QueueTenantTelegram(ctx, tid, notify.OpsMsg("wo", wo.Type, "Status: "+wo.Status))
			return &struct{ Body store.WorkOrder }{Body: wo}, nil
		}
		who := strings.TrimSpace(full.CustomerName)
		if who == "" {
			who = "tanpa pelanggan"
		}
		tech := strings.TrimSpace(full.TechnicianName)
		if tech == "" {
			tech = "belum ditugaskan"
		}
		_ = d.Notify.QueueTenantTelegram(ctx, tid, notify.OpsMsg("wo", full.Type,
			"Status: "+full.Status,
			"Pelanggan: "+who,
			"Teknisi: "+tech,
		))
		return &struct{ Body store.WorkOrder }{Body: *full}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-work-order", Method: http.MethodPatch, Path: "/api/work-orders/{id}",
		Tags: []string{"WorkOrders"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			TechnicianID *xid.ID    `json:"technician_id"`
			ClearTech    bool       `json:"clear_technician,omitempty"`
			Status       *string    `json:"status,omitempty"`
			Notes        *string    `json:"notes,omitempty"`
			ScheduledAt  *time.Time `json:"scheduled_at,omitempty"`
			ClearSched   bool       `json:"clear_scheduled_at,omitempty"`
		}
	}) (*struct{ Body store.WorkOrder }, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		var techPtr **xid.ID
		if input.Body.ClearTech {
			var nilID *xid.ID
			techPtr = &nilID
		} else if input.Body.TechnicianID != nil {
			id := input.Body.TechnicianID
			techPtr = &id
		}
		var schedPtr **time.Time
		if input.Body.ClearSched {
			var nilT *time.Time
			schedPtr = &nilT
		} else if input.Body.ScheduledAt != nil {
			t := input.Body.ScheduledAt
			schedPtr = &t
		}
		wo, err := d.Store.UpdateWorkOrder(ctx, tid, input.ID, techPtr, input.Body.Status, input.Body.Notes, schedPtr)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("work order not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.WorkOrder }{Body: *wo}, nil
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
	}) (*struct{ Body store.WorkOrder }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := d.Store.GetWorkOrder(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("work order not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceWorkOrderAssigned(ctx, d, tid, cur); err != nil {
			return nil, err
		}
		wo, err := d.Store.CheckInWorkOrder(ctx, tid, input.ID, input.Body.Lat, input.Body.Lng)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("work order not found atau sudah selesai")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.WorkOrder }{Body: *wo}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "complete-work-order", Method: http.MethodPost, Path: "/api/work-orders/{id}/complete",
		Tags: []string{"WorkOrders"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Notes *string `json:"notes,omitempty"`
		}
	}) (*struct{ Body store.WorkOrder }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := d.Store.GetWorkOrder(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("work order not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceWorkOrderAssigned(ctx, d, tid, cur); err != nil {
			return nil, err
		}
		wo, err := d.Store.CompleteWorkOrder(ctx, tid, input.ID, input.Body.Notes)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("work order not found")
		}
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body store.WorkOrder }{Body: *wo}, nil
	})
}

func registerOpsExtra(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-leads", Method: http.MethodGet, Path: "/api/leads",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status        string `query:"status"`
		HideConverted bool   `query:"hide_converted"`
		Limit         int    `query:"limit"`
		Offset        int    `query:"offset"`
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
		var assignedTo *xid.ID
		if isFieldOps(ctx, d) {
			uid := userIDFromCtx(ctx)
			assignedTo = &uid
		}
		list, total, err := d.Store.ListLeads(ctx, tid, input.Status, assignedTo, input.HideConverted, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.Lead{}
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
			FullName       string   `json:"full_name,omitempty"`
			Name           string   `json:"name,omitempty"` // alias from UI
			Phone          string   `json:"phone,omitempty"`
			Email          *string  `json:"email,omitempty"`
			Address        *string  `json:"address,omitempty"`
			Latitude       *float64 `json:"latitude,omitempty"`
			Longitude      *float64 `json:"longitude,omitempty"`
			IdentityType   *string  `json:"identity_type,omitempty"`
			IdentityNumber *string  `json:"identity_number,omitempty"`
			ODPID          *xid.ID  `json:"odp_id,omitempty"`
			Status         string   `json:"status,omitempty"`
			Notes          *string  `json:"notes,omitempty"`
			ResellerID     *xid.ID  `json:"reseller_id,omitempty"`
			SalesUserID    *xid.ID  `json:"sales_user_id,omitempty"`
			AssignedTo     *xid.ID  `json:"assigned_to,omitempty"`
		}
	}) (*struct{ Body store.Lead }, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		fullName := strings.TrimSpace(input.Body.FullName)
		if fullName == "" {
			fullName = strings.TrimSpace(input.Body.Name)
		}
		if fullName == "" {
			return nil, httpx.BadRequest("nama wajib")
		}
		phone := strings.TrimSpace(input.Body.Phone)
		if phone == "" {
			return nil, httpx.BadRequest("telepon wajib")
		}
		status := store.NormalizeLeadStatus(input.Body.Status)
		if store.LeadStatusNeedsAssignee(status) && (input.Body.AssignedTo == nil || xid.IsNil(*input.Body.AssignedTo)) {
			return nil, httpx.BadRequest("teknisi wajib di-assign mulai status dihubungi")
		}
		rID, sID := store.NormalizeAttribution(input.Body.ResellerID, input.Body.SalesUserID)
		l := store.Lead{
			TenantID: tid, FullName: fullName, Phone: phone,
			Email: emptyToNil(input.Body.Email), Address: emptyToNil(input.Body.Address),
			Latitude: input.Body.Latitude, Longitude: input.Body.Longitude,
			IdentityType:   normalizeIdentityType(input.Body.IdentityType),
			IdentityNumber: emptyToNil(input.Body.IdentityNumber),
			ODPID:          input.Body.ODPID, Status: status, Notes: emptyToNil(input.Body.Notes),
			ResellerID: rID, SalesUserID: sID, AssignedTo: input.Body.AssignedTo,
		}
		if err := d.Store.CreateLead(ctx, &l); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		out, _ := d.Store.GetLead(ctx, tid, l.ID)
		if out != nil {
			l = *out
		}
		return &struct{ Body store.Lead }{Body: l}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-lead", Method: http.MethodPut, Path: "/api/leads/{id}",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			FullName       string   `json:"full_name"`
			Phone          string   `json:"phone"`
			Email          *string  `json:"email,omitempty"`
			Address        *string  `json:"address,omitempty"`
			Latitude       *float64 `json:"latitude,omitempty"`
			Longitude      *float64 `json:"longitude,omitempty"`
			IdentityType   *string  `json:"identity_type,omitempty"`
			IdentityNumber *string  `json:"identity_number,omitempty"`
			ODPID          *xid.ID  `json:"odp_id,omitempty"`
			Status         string   `json:"status,omitempty"`
			Notes          *string  `json:"notes,omitempty"`
			ResellerID     *xid.ID  `json:"reseller_id,omitempty"`
			SalesUserID    *xid.ID  `json:"sales_user_id,omitempty"`
			AssignedTo     *xid.ID  `json:"assigned_to,omitempty"`
		}
	}) (*struct{ Body store.Lead }, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		fullName := strings.TrimSpace(input.Body.FullName)
		phone := strings.TrimSpace(input.Body.Phone)
		if fullName == "" || phone == "" {
			return nil, httpx.BadRequest("nama dan telepon wajib")
		}
		status := store.NormalizeLeadStatus(input.Body.Status)
		rID, sID := store.NormalizeAttribution(input.Body.ResellerID, input.Body.SalesUserID)
		l := &store.Lead{
			ID: input.ID, TenantID: tid, FullName: fullName, Phone: phone,
			Email: emptyToNil(input.Body.Email), Address: emptyToNil(input.Body.Address),
			Latitude: input.Body.Latitude, Longitude: input.Body.Longitude,
			IdentityType:   normalizeIdentityType(input.Body.IdentityType),
			IdentityNumber: emptyToNil(input.Body.IdentityNumber),
			ODPID:          input.Body.ODPID, Status: status, Notes: emptyToNil(input.Body.Notes),
			ResellerID: rID, SalesUserID: sID, AssignedTo: input.Body.AssignedTo,
		}
		if err := d.Store.UpdateLead(ctx, l); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("lead tidak ditemukan atau sudah dikonversi")
			}
			return nil, httpx.BadRequest(err.Error())
		}
		out, err := d.Store.GetLead(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Lead }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "patch-lead-status", Method: http.MethodPatch, Path: "/api/leads/{id}/status",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Status     string  `json:"status"`
			AssignedTo *xid.ID `json:"assigned_to,omitempty"`
		}
	}) (*struct{ Body store.Lead }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := d.Store.GetLead(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("lead tidak ditemukan atau sudah dikonversi")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		fromStatus := cur.Status
		newStatus := store.NormalizeLeadStatus(input.Body.Status)
		assignedTo := input.Body.AssignedTo

		if isFieldOps(ctx, d) {
			if err := enforceLeadAssigned(ctx, d, cur); err != nil {
				return nil, err
			}
			// Teknisi assigned boleh pindah status, tapi tidak assign orang lain / convert / balik ke Baru.
			switch newStatus {
			case "converted", "new":
				return nil, httpx.Forbidden("teknisi tidak bisa memindah lead ke status ini")
			}
			assignedTo = cur.AssignedTo
		} else if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}

		if err := d.Store.UpdateLeadStatus(ctx, tid, input.ID, newStatus, assignedTo); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("lead tidak ditemukan atau sudah dikonversi")
			}
			return nil, httpx.BadRequest(err.Error())
		}
		if fromStatus != newStatus {
			var actor *xid.ID
			if uid := userIDFromCtx(ctx); !xid.IsNil(uid) {
				actor = &uid
			}
			msg := fmt.Sprintf("Status: %s → %s", store.LeadStatusLabel(fromStatus), store.LeadStatusLabel(newStatus))
			_, _ = d.Store.AddLeadActivity(ctx, tid, input.ID, actor, "status_change", msg, nil)
		}
		out, err := d.Store.GetLead(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Lead }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "assign-lead", Method: http.MethodPatch, Path: "/api/leads/{id}/assign",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			AssignedTo *xid.ID `json:"assigned_to"`
		}
	}) (*struct{ Body store.Lead }, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.AssignLead(ctx, tid, input.ID, input.Body.AssignedTo); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("lead tidak ditemukan atau belum dihubungi/proses pasang")
			}
			return nil, httpx.BadRequest(err.Error())
		}
		out, err := d.Store.GetLead(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Lead }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-lead", Method: http.MethodDelete, Path: "/api/leads/{id}",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{}, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteLead(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("lead tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "convert-lead", Method: http.MethodPost, Path: "/api/leads/{id}/convert",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			ClusterID       *xid.ID  `json:"cluster_id,omitempty"`
			CustomerCode    string   `json:"customer_code,omitempty"`
			Latitude        *float64 `json:"latitude,omitempty"`
			Longitude       *float64 `json:"longitude,omitempty"`
			IdentityType    *string  `json:"identity_type,omitempty"`
			IdentityNumber  *string  `json:"identity_number,omitempty"`
			ResellerID      *xid.ID  `json:"reseller_id,omitempty"`
			SalesUserID     *xid.ID  `json:"sales_user_id,omitempty"`
			CommissionBasis string   `json:"commission_basis,omitempty"`
		}
	}) (*struct {
		Body struct {
			Customer store.Customer `json:"customer"`
			Lead     store.Lead     `json:"lead"`
		}
	}, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		// Optional final touch-up from the convert dialog (coords & identity),
		// saved onto the lead so the new customer picks them up automatically.
		if input.Body.Latitude != nil || input.Body.Longitude != nil ||
			input.Body.IdentityType != nil || input.Body.IdentityNumber != nil {
			lead, gerr := d.Store.GetLead(ctx, tid, input.ID)
			if errors.Is(gerr, store.ErrNotFound) {
				return nil, httpx.NotFound("lead tidak ditemukan")
			} else if gerr != nil {
				return nil, httpx.Internal(gerr)
			}
			if input.Body.Latitude != nil {
				lead.Latitude = input.Body.Latitude
			}
			if input.Body.Longitude != nil {
				lead.Longitude = input.Body.Longitude
			}
			if input.Body.IdentityType != nil {
				lead.IdentityType = normalizeIdentityType(input.Body.IdentityType)
			}
			if input.Body.IdentityNumber != nil {
				lead.IdentityNumber = emptyToNil(input.Body.IdentityNumber)
			}
			if err := d.Store.UpdateLead(ctx, lead); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return nil, httpx.NotFound("lead tidak ditemukan atau sudah dikonversi")
				}
				return nil, httpx.BadRequest(err.Error())
			}
		}
		cust, lead, err := d.Store.ConvertLeadToCustomer(ctx, tid, input.ID, input.Body.ClusterID, input.Body.CustomerCode, input.Body.ResellerID, input.Body.SalesUserID, input.Body.CommissionBasis)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("lead tidak ditemukan")
			}
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct {
			Body struct {
				Customer store.Customer `json:"customer"`
				Lead     store.Lead     `json:"lead"`
			}
		}{Body: struct {
			Customer store.Customer `json:"customer"`
			Lead     store.Lead     `json:"lead"`
		}{Customer: *cust, Lead: *lead}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-lead-comments", Method: http.MethodGet, Path: "/api/leads/{id}/comments",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body []store.LeadComment }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		lead, err := d.Store.GetLead(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("lead tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceLeadAssigned(ctx, d, lead); err != nil {
			return nil, err
		}
		list, err := d.Store.ListLeadComments(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("lead tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.LeadComment{}
		}
		return &struct{ Body []store.LeadComment }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "add-lead-comment", Method: http.MethodPost, Path: "/api/leads/{id}/comments",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Message   string   `json:"message"`
			ImageURLs []string `json:"image_urls,omitempty"`
		}
	}) (*struct{ Body store.LeadComment }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		lead, err := d.Store.GetLead(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("lead tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceLeadAssigned(ctx, d, lead); err != nil {
			return nil, err
		}
		var senderID *xid.ID
		if uid := userIDFromCtx(ctx); !xid.IsNil(uid) {
			senderID = &uid
		}
		c, err := d.Store.AddLeadComment(ctx, tid, input.ID, senderID, input.Body.Message, input.Body.ImageURLs)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("lead tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body store.LeadComment }{Body: *c}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-lead-documents", Method: http.MethodGet, Path: "/api/leads/{id}/documents",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body []store.LeadDocument }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		lead, err := d.Store.GetLead(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("lead tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceLeadAssigned(ctx, d, lead); err != nil {
			return nil, err
		}
		list, err := d.Store.ListLeadDocuments(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.LeadDocument{}
		}
		return &struct{ Body []store.LeadDocument }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "add-lead-document", Method: http.MethodPost, Path: "/api/leads/{id}/documents",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Kind    string `json:"kind"`
			URL     string `json:"url"`
			Caption string `json:"caption,omitempty"`
		}
	}) (*struct{ Body store.LeadDocument }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		lead, err := d.Store.GetLead(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("lead tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceLeadAssigned(ctx, d, lead); err != nil {
			return nil, err
		}
		st := store.NormalizeLeadStatus(lead.Status)
		if st != "survey" && st != "qualified" {
			return nil, httpx.BadRequest("dokumen PSB hanya bisa ditambah saat status survey / proses pasang")
		}
		var uploader *xid.ID
		if uid := userIDFromCtx(ctx); !xid.IsNil(uid) {
			uploader = &uid
		}
		doc, err := d.Store.AddLeadDocument(ctx, tid, input.ID, uploader, input.Body.Kind, input.Body.URL, input.Body.Caption)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("lead tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body store.LeadDocument }{Body: *doc}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-lead-document", Method: http.MethodDelete, Path: "/api/leads/{id}/documents/{doc_id}",
		Tags: []string{"Leads"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID    xid.ID `path:"id"`
		DocID xid.ID `path:"doc_id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		lead, err := d.Store.GetLead(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("lead tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceLeadAssigned(ctx, d, lead); err != nil {
			return nil, err
		}
		if err := d.Store.DeleteLeadDocument(ctx, tid, input.ID, input.DocID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("dokumen tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
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
		OperationID: "update-reseller", Method: http.MethodPut, Path: "/api/resellers/{id}",
		Tags: []string{"Resellers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name              string  `json:"name"`
			Phone             string  `json:"phone"`
			CommissionPercent float64 `json:"commission_percent"`
			IsActive          bool    `json:"is_active"`
		}
	}) (*struct{ Body store.Reseller }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		if name == "" {
			return nil, httpx.BadRequest("nama wajib")
		}
		phone := strings.TrimSpace(input.Body.Phone)
		r := &store.Reseller{
			ID: input.ID, TenantID: tid, Name: name, Phone: &phone,
			CommissionPercent: input.Body.CommissionPercent, IsActive: input.Body.IsActive,
		}
		if err := d.Store.UpdateReseller(ctx, r); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("reseller tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		out, err := d.Store.GetReseller(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Reseller }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-reseller", Method: http.MethodDelete, Path: "/api/resellers/{id}",
		Tags: []string{"Resellers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteReseller(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("reseller tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-commissions", Method: http.MethodGet, Path: "/api/commissions",
		Tags: []string{"Commissions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status string `query:"status"`
		Limit  int    `query:"limit"`
		Offset int    `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.CommissionEntry `json:"data"`
			Total int64                   `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, total, err := d.Store.ListCommissionEntries(ctx, tid, input.Status, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.CommissionEntry{}
		}
		out := &struct {
			Body struct {
				Data  []store.CommissionEntry `json:"data"`
				Total int64                   `json:"total"`
			}
		}{}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "mark-commission-paid", Method: http.MethodPost, Path: "/api/commissions/{id}/paid",
		Tags: []string{"Commissions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.CommissionEntry }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.MarkCommissionPaid(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("komisi tidak ditemukan atau bukan pending")
			}
			return nil, httpx.Internal(err)
		}
		e, err := d.Store.GetCommissionEntry(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.CommissionEntry }{Body: *e}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "void-commission", Method: http.MethodPost, Path: "/api/commissions/{id}/void",
		Tags: []string{"Commissions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.CommissionEntry }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.VoidCommissionEntry(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("komisi tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		e, err := d.Store.GetCommissionEntry(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.CommissionEntry }{Body: *e}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-commission-settings", Method: http.MethodGet, Path: "/api/settings/commission",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body store.CommissionSettings }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cfg, err := d.Store.GetCommissionSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.CommissionSettings }{Body: cfg}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-commission-settings", Method: http.MethodPut, Path: "/api/settings/commission",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body store.CommissionSettings
	}) (*struct{ Body store.CommissionSettings }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.SetCommissionSettings(ctx, tid, input.Body); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body store.CommissionSettings }{Body: input.Body}, nil
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
		ID              xid.ID `path:"id"`
		Host            string `header:"Host"`
		Origin          string `header:"Origin"`
		Referer         string `header:"Referer"`
		XForwardedHost  string `header:"X-Forwarded-Host"`
		XForwardedProto string `header:"X-Forwarded-Proto"`
		Body            struct {
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
		origin := appPublicOrigin(ctx, d, tid, input.Origin, input.Referer, input.XForwardedProto, input.XForwardedHost, input.Host)
		returnURL := strings.TrimSpace(input.Body.ReturnURL)
		if returnURL == "" {
			returnURL = origin
		}
		pi, err := checkoutInvoice(ctx, d, tid, inv, input.Body.Provider, returnURL, origin)
		if err != nil {
			return nil, err
		}
		return &struct{ Body store.PaymentIntent }{Body: *pi}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "invoice-payment-intent", Method: http.MethodGet, Path: "/api/invoices/{id}/payment-intent",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.PaymentIntent }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		inv, _, err := d.Store.GetInvoice(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		pi, err := latestInvoicePaymentIntent(ctx, d, tid, inv)
		if err != nil {
			return nil, err
		}
		return &struct{ Body store.PaymentIntent }{Body: *pi}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "invoice-payment-intent-cancel", Method: http.MethodPost, Path: "/api/invoices/{id}/payment-intent/cancel",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.PaymentIntent }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		inv, _, err := d.Store.GetInvoice(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		pi, err := cancelInvoicePaymentIntent(ctx, d, tid, inv)
		if err != nil {
			return nil, err
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
	}, func(ctx context.Context, input *struct {
		Body struct {
			URL    string   `json:"url"`
			Secret *string  `json:"secret,omitempty"`
			Events []string `json:"events"`
		}
	}) (*struct{ Body store.OutboundWebhook }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		w := store.OutboundWebhook{
			TenantID: tid,
			URL:      strings.TrimSpace(input.Body.URL),
			Secret:   input.Body.Secret,
			Events:   input.Body.Events,
		}
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
		list, _, err := d.Store.ListInvoices(ctx, tid, "", "", false, 100000, 0)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		f := excelize.NewFile()
		sheet := "Invoices"
		_ = f.SetSheetName("Sheet1", sheet)
		_ = f.SetSheetRow(sheet, "A1", &[]any{"invoice_number", "customer", "due_date", "total", "paid_amount", "outstanding", "status"})
		for i, inv := range list {
			cell := fmt.Sprintf("A%d", i+2)
			_ = f.SetSheetRow(sheet, cell, &[]any{
				inv.InvoiceNumber, inv.CustomerName, inv.DueDate.Format("2006-01-02"),
				inv.TotalAmount, inv.PaidAmount, inv.TotalAmount - inv.PaidAmount, inv.Status,
			})
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
		OperationID: "sla-report", Method: http.MethodGet, Path: "/api/reports/sla",
		Tags: []string{"Reports"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Days int `query:"days"`
	}) (*struct{ Body store.TicketSLAReport }, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		rep, err := d.Store.TicketSLAReport(ctx, tid, input.Days)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.TicketSLAReport }{Body: *rep}, nil
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
