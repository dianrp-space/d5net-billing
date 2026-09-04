package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

func registerIPAM(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-ip-pools", Method: "GET", Path: "/api/ip-pools",
		Tags: []string{"IP Pool"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.IPPool }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListIPPools(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.IPPool{}
		}
		return &struct{ Body []store.IPPool }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-ip-pool", Method: "POST", Path: "/api/ip-pools",
		Tags: []string{"IP Pool"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name       string   `json:"name"`
			Network    string   `json:"network"`
			Gateway    *string  `json:"gateway,omitempty"`
			RouterID   *xid.ID  `json:"router_id,omitempty"`
			DNSServers []string `json:"dns_servers,omitempty"`
		}
	}) (*struct{ Body store.IPPool }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		network := strings.TrimSpace(input.Body.Network)
		if name == "" || network == "" {
			return nil, httpx.BadRequest("name dan network wajib")
		}
		p := &store.IPPool{
			TenantID: tid, Name: name, Network: network,
			Gateway: emptyToNil(input.Body.Gateway), RouterID: input.Body.RouterID, DNSServers: cleanDNS(input.Body.DNSServers),
		}
		if err := d.Store.CreateIPPool(ctx, p); err != nil {
			return nil, httpx.Internal(err)
		}
		out, err := d.Store.GetIPPool(ctx, tid, p.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.IPPool }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-ip-pool", Method: "PUT", Path: "/api/ip-pools/{id}",
		Tags: []string{"IP Pool"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name       string   `json:"name"`
			Network    string   `json:"network"`
			Gateway    *string  `json:"gateway,omitempty"`
			RouterID   *xid.ID  `json:"router_id,omitempty"`
			DNSServers []string `json:"dns_servers,omitempty"`
		}
	}) (*struct{ Body store.IPPool }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		network := strings.TrimSpace(input.Body.Network)
		if name == "" || network == "" {
			return nil, httpx.BadRequest("name dan network wajib")
		}
		p := &store.IPPool{
			ID: input.ID, TenantID: tid, Name: name, Network: network,
			Gateway: emptyToNil(input.Body.Gateway), RouterID: input.Body.RouterID, DNSServers: cleanDNS(input.Body.DNSServers),
		}
		if err := d.Store.UpdateIPPool(ctx, p); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("pool tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		out, err := d.Store.GetIPPool(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.IPPool }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-ip-pool", Method: "DELETE", Path: "/api/ip-pools/{id}",
		Tags: []string{"IP Pool"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteIPPool(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("pool tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-ip-assignments", Method: "GET", Path: "/api/ip-pools/{id}/assignments",
		Tags: []string{"IP Pool"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body []store.IPAssignment }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListIPAssignments(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.IPAssignment{}
		}
		return &struct{ Body []store.IPAssignment }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "assign-ip", Method: "POST", Path: "/api/ip-pools/{id}/assignments",
		Tags: []string{"IP Pool"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			IPAddress  string  `json:"ip_address"`
			CustomerID *xid.ID `json:"customer_id,omitempty"`
			MACAddress *string `json:"mac_address,omitempty"`
			Status     string  `json:"status"`
		}
	}) (*struct{ Body store.IPAssignment }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		ip := strings.TrimSpace(input.Body.IPAddress)
		if ip == "" {
			return nil, httpx.BadRequest("ip_address wajib")
		}
		status := input.Body.Status
		if status == "" {
			status = "assigned"
		}
		a := &store.IPAssignment{
			TenantID: tid, PoolID: input.ID, CustomerID: input.Body.CustomerID,
			IPAddress: ip, MACAddress: emptyToNil(input.Body.MACAddress), Status: status,
		}
		if err := d.Store.AssignIP(ctx, a); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.IPAssignment }{Body: *a}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-ip-assignment", Method: "DELETE", Path: "/api/ip-pools/{id}/assignments/{assignment_id}",
		Tags: []string{"IP Pool"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID           xid.ID `path:"id"`
		AssignmentID xid.ID `path:"assignment_id"`
	}) (*struct{}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteIPAssignment(ctx, tid, input.ID, input.AssignmentID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("assignment tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{}{}, nil
	})
}

func emptyToNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

func cleanDNS(in []string) []string {
	var out []string
	for _, d := range in {
		d = strings.TrimSpace(d)
		if d != "" {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
