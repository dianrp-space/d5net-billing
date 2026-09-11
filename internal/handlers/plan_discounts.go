package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

type planDiscountBody struct {
	Name        string   `json:"name"`
	PlanID      *xid.ID  `json:"plan_id,omitempty"`
	Kind        string   `json:"kind"`
	Value       int64    `json:"value"`
	StartsOn    string   `json:"starts_on"`
	EndsOn      string   `json:"ends_on"`
	Audience    string   `json:"audience"`
	IsActive    *bool    `json:"is_active,omitempty"`
	CustomerIDs []xid.ID `json:"customer_ids,omitempty"`
}

func registerPlanDiscounts(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-plan-discounts", Method: http.MethodGet, Path: "/api/plan-discounts",
		Tags: []string{"Discounts"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.PlanDiscount }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListPlanDiscounts(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.PlanDiscount }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-plan-discount", Method: http.MethodGet, Path: "/api/plan-discounts/{id}",
		Tags: []string{"Discounts"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.PlanDiscount }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		row, err := d.Store.GetPlanDiscount(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("diskon tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.PlanDiscount }{Body: *row}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-plan-discount", Method: http.MethodPost, Path: "/api/plan-discounts",
		Tags: []string{"Discounts"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body planDiscountBody
	}) (*struct{ Body store.PlanDiscount }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		row, err := discountFromBody(ctx, d, tid, xid.Nil(), input.Body, true)
		if err != nil {
			return nil, err
		}
		created, err := d.Store.CreatePlanDiscount(ctx, row)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.PlanDiscount }{Body: *created}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-plan-discount", Method: http.MethodPut, Path: "/api/plan-discounts/{id}",
		Tags: []string{"Discounts"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body planDiscountBody
	}) (*struct{ Body store.PlanDiscount }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		existing, err := d.Store.GetPlanDiscount(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("diskon tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		row, err := discountFromBody(ctx, d, tid, input.ID, input.Body, existing.IsActive)
		if err != nil {
			return nil, err
		}
		updated, err := d.Store.UpdatePlanDiscount(ctx, row)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("diskon tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.PlanDiscount }{Body: *updated}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-plan-discount", Method: http.MethodDelete, Path: "/api/plan-discounts/{id}",
		Tags: []string{"Discounts"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeletePlanDiscount(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("diskon tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})
}

func discountFromBody(ctx context.Context, d *Deps, tenantID, id xid.ID, body planDiscountBody, defaultActive bool) (*store.PlanDiscount, error) {
	row := store.NormalizePlanDiscount(store.PlanDiscount{
		ID:          id,
		TenantID:    tenantID,
		Name:        body.Name,
		PlanID:      normalizeOptionalID(body.PlanID),
		Kind:        body.Kind,
		Value:       body.Value,
		StartsOn:    body.StartsOn,
		EndsOn:      body.EndsOn,
		Audience:    body.Audience,
		IsActive:    defaultActive,
		CustomerIDs: body.CustomerIDs,
	})
	if body.IsActive != nil {
		row.IsActive = *body.IsActive
	}
	if row.Name == "" {
		return nil, httpx.BadRequest("nama diskon wajib diisi")
	}
	if row.Value <= 0 {
		return nil, httpx.BadRequest("nilai diskon harus lebih dari 0")
	}
	if row.Kind == store.DiscountKindPercent && row.Value > 100 {
		return nil, httpx.BadRequest("diskon persen maksimal 100")
	}
	start, err := parseDiscountDate(row.StartsOn)
	if err != nil {
		return nil, httpx.BadRequest("tanggal mulai tidak valid")
	}
	end, err := parseDiscountDate(row.EndsOn)
	if err != nil {
		return nil, httpx.BadRequest("tanggal selesai tidak valid")
	}
	if end.Before(start) {
		return nil, httpx.BadRequest("tanggal selesai harus sama atau setelah tanggal mulai")
	}
	row.StartsOn = start.Format("2006-01-02")
	row.EndsOn = end.Format("2006-01-02")
	if row.PlanID != nil {
		if _, err := d.Store.GetPlan(ctx, tenantID, *row.PlanID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.BadRequest("paket tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
	}
	if row.Audience == store.DiscountAudiencePick {
		if len(row.CustomerIDs) == 0 {
			return nil, httpx.BadRequest("pilih minimal satu pelanggan")
		}
		seen := map[xid.ID]struct{}{}
		var ids []xid.ID
		for _, cid := range row.CustomerIDs {
			if xid.IsNil(cid) {
				continue
			}
			if _, ok := seen[cid]; ok {
				continue
			}
			if _, err := d.Store.GetCustomer(ctx, tenantID, cid); errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("pelanggan tidak ditemukan")
			} else if err != nil {
				return nil, httpx.Internal(err)
			}
			seen[cid] = struct{}{}
			ids = append(ids, cid)
		}
		if len(ids) == 0 {
			return nil, httpx.BadRequest("pilih minimal satu pelanggan")
		}
		row.CustomerIDs = ids
	}
	return &row, nil
}

func normalizeOptionalID(id *xid.ID) *xid.ID {
	if id == nil || xid.IsNil(*id) {
		return nil
	}
	return id
}

func parseDiscountDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(s))
}
