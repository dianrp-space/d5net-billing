package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
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
			ClusterID  *xid.ID  `json:"cluster_id,omitempty"`
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
		clusterID, routerID, err := bindIPPoolCluster(ctx, d, tid, input.Body.ClusterID, input.Body.RouterID)
		if err != nil {
			return nil, err
		}
		p := &store.IPPool{
			TenantID: tid, Name: name, Network: network,
			Gateway: emptyToNil(input.Body.Gateway), ClusterID: clusterID, RouterID: routerID, DNSServers: cleanDNS(input.Body.DNSServers),
		}
		if err := d.Store.CreateIPPool(ctx, p); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return nil, httpx.BadRequest("nama pool sudah dipakai di cluster ini")
			}
			return nil, httpx.Internal(err)
		}
		out, err := d.Store.GetIPPool(ctx, tid, p.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := syncIPPoolToRouter(ctx, d, out); err != nil {
			return nil, httpx.BadRequest("pool tersimpan, tapi gagal sync ke RouterOS: " + err.Error())
		}
		resyncPoolAssignments(ctx, d, out)
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
			ClusterID  *xid.ID  `json:"cluster_id,omitempty"`
			RouterID   *xid.ID  `json:"router_id,omitempty"`
			DNSServers []string `json:"dns_servers,omitempty"`
		}
	}) (*struct{ Body store.IPPool }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		old, err := d.Store.GetIPPool(ctx, tid, input.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("pool tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		name := strings.TrimSpace(input.Body.Name)
		network := strings.TrimSpace(input.Body.Network)
		if name == "" || network == "" {
			return nil, httpx.BadRequest("name dan network wajib")
		}
		clusterID, routerID, err := bindIPPoolCluster(ctx, d, tid, input.Body.ClusterID, input.Body.RouterID)
		if err != nil {
			return nil, err
		}
		p := &store.IPPool{
			ID: input.ID, TenantID: tid, Name: name, Network: network,
			Gateway: emptyToNil(input.Body.Gateway), ClusterID: clusterID, RouterID: routerID, DNSServers: cleanDNS(input.Body.DNSServers),
		}
		if err := d.Store.UpdateIPPool(ctx, p); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("pool tidak ditemukan")
			}
			if errors.Is(err, store.ErrConflict) {
				return nil, httpx.BadRequest("nama pool sudah dipakai di cluster ini")
			}
			return nil, httpx.Internal(err)
		}
		out, err := d.Store.GetIPPool(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		// Remove from previous router if unlinked / renamed / moved.
		if old.RouterID != nil {
			moved := out.RouterID == nil || *old.RouterID != *out.RouterID
			renamed := old.Name != out.Name
			if moved || renamed {
				removeIPPoolFromRouter(ctx, d, tid, *old.RouterID, old.Name)
			}
		}
		if err := syncIPPoolToRouter(ctx, d, out); err != nil {
			return nil, httpx.BadRequest("pool tersimpan, tapi gagal sync ke RouterOS: " + err.Error())
		}
		resyncPoolAssignments(ctx, d, out)
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
		old, err := d.Store.GetIPPool(ctx, tid, input.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("pool tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		if err := d.Store.DeleteIPPool(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("pool tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		if old.RouterID != nil {
			removeIPPoolFromRouter(ctx, d, tid, *old.RouterID, old.Name)
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
		pool, err := d.Store.GetIPPool(ctx, tid, input.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("pool tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		if err := validateIPAssignmentCustomer(ctx, d, tid, pool, input.Body.CustomerID); err != nil {
			return nil, err
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
		if a.CustomerID != nil {
			if err := syncCustomerIPAssignment(ctx, d, pool, *a.CustomerID); err != nil {
				return nil, httpx.BadRequest("IP tersimpan, tapi gagal sync ke RouterOS: " + err.Error())
			}
		} else if pool.RouterID != nil {
			if err := syncIPPoolToRouter(ctx, d, pool); err != nil {
				return nil, httpx.BadRequest("IP tersimpan, tapi gagal sync pool ke RouterOS: " + err.Error())
			}
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
		pool, err := d.Store.GetIPPool(ctx, tid, input.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("pool tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		asg, err := d.Store.GetIPAssignment(ctx, tid, input.ID, input.AssignmentID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("assignment tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		if err := d.Store.DeleteIPAssignment(ctx, tid, input.ID, input.AssignmentID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("assignment tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		if asg.CustomerID != nil {
			clearCustomerStaticIPOnRouter(ctx, d, pool, *asg.CustomerID)
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

func bindIPPoolCluster(ctx context.Context, d *Deps, tid xid.ID, clusterID, routerID *xid.ID) (*xid.ID, *xid.ID, error) {
	if routerID != nil {
		r, err := d.Store.GetRouter(ctx, tid, *routerID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, nil, httpx.BadRequest("router tidak ditemukan")
			}
			return nil, nil, httpx.Internal(err)
		}
		if clusterID != nil && r.SiteID != nil && *clusterID != *r.SiteID {
			return nil, nil, httpx.BadRequest("cluster pool harus sama dengan cluster router")
		}
		if clusterID == nil {
			clusterID = r.SiteID
		}
	}
	if clusterID != nil {
		if _, err := d.Store.GetCluster(ctx, tid, *clusterID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, nil, httpx.BadRequest("cluster tidak ditemukan")
			}
			return nil, nil, httpx.Internal(err)
		}
	}
	return clusterID, routerID, nil
}

func validateIPAssignmentCustomer(ctx context.Context, d *Deps, tid xid.ID, pool *store.IPPool, customerID *xid.ID) error {
	if customerID == nil || pool == nil || pool.ClusterID == nil {
		return nil
	}
	cust, err := d.Store.GetCustomer(ctx, tid, *customerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return httpx.BadRequest("pelanggan tidak ditemukan")
		}
		return httpx.Internal(err)
	}
	if cust.ClusterID != nil && *cust.ClusterID != *pool.ClusterID {
		return httpx.BadRequest("pelanggan bukan dari cluster pool ini")
	}
	return nil
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

func resyncPoolAssignments(ctx context.Context, d *Deps, pool *store.IPPool) {
	if pool == nil || pool.RouterID == nil {
		return
	}
	list, err := d.Store.ListIPAssignments(ctx, pool.TenantID, pool.ID)
	if err != nil {
		return
	}
	for _, a := range list {
		if a.CustomerID == nil || a.Status != "assigned" {
			continue
		}
		_ = syncCustomerIPAssignment(ctx, d, pool, *a.CustomerID)
	}
}
