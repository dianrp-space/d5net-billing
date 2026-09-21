package handlers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	ros "github.com/dianrp-space/d5net-billing/internal/provision/routeros"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

var trafficMonthRE = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// pppLiveQuerier dipenuhi client RouterOS API (bukan RADIUS).
type pppLiveQuerier interface {
	PPPLiveTraffic(ctx context.Context, tenantID, routerID xid.ID, username string) (*ros.PPPLive, error)
}

func queryPPPLive(ctx context.Context, d *Deps, tid, routerID xid.ID, username string) (*ros.PPPLive, error) {
	prov, err := d.Provisioner.Get("routeros")
	if err != nil {
		return nil, err
	}
	q, ok := prov.(pppLiveQuerier)
	if !ok {
		return nil, fmt.Errorf("provisioner tidak mendukung live traffic")
	}
	return q.PPPLiveTraffic(ctx, tid, routerID, username)
}

type liveTrafficRow struct {
	SubscriptionID xid.ID `json:"subscription_id"`
	Username       string `json:"username"`
	RouterID       xid.ID `json:"router_id"`
	RouterName     string `json:"router_name,omitempty"`
	Status         string `json:"status"`
	Online         bool   `json:"online"`
	IPAddress      string `json:"ip_address,omitempty"`
	Uptime         string `json:"uptime,omitempty"`
	RxBps          int64  `json:"rx_bps"`
	TxBps          int64  `json:"tx_bps"`
	Error          string `json:"error,omitempty"`
}

func registerTraffic(api huma.API, d *Deps) {
	// Live traffic PPPoE satu pelanggan (khusus admin). Di-poll tiap
	// beberapa detik dari dialog admin; tiap baris = satu langganan aktif.
	huma.Register(api, huma.Operation{
		OperationID: "customer-live-traffic", Method: http.MethodGet, Path: "/api/customers/{id}/live-traffic",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body struct {
			Data []liveTrafficRow `json:"data"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := d.Store.GetCustomer(ctx, tid, input.ID); err != nil {
			return nil, httpx.NotFound("pelanggan tidak ditemukan")
		}
		subs, err := d.Store.CustomerLiveSubs(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Data []liveTrafficRow `json:"data"`
			}
		}{}
		out.Body.Data = []liveTrafficRow{}
		for _, sub := range subs {
			row := liveTrafficRow{
				SubscriptionID: sub.SubscriptionID,
				Username:       sub.Username,
				RouterID:       sub.RouterID,
				RouterName:     sub.RouterName,
				Status:         sub.Status,
			}
			router, rerr := d.Store.GetRouter(ctx, tid, sub.RouterID)
			if rerr != nil {
				row.Error = "router tidak ditemukan"
				out.Body.Data = append(out.Body.Data, row)
				continue
			}
			row.RouterName = router.Name
			if pt := strings.ToLower(strings.TrimSpace(router.Provisioner)); pt != "" && pt != "routeros" {
				row.Error = "live traffic hanya untuk router RouterOS API"
				out.Body.Data = append(out.Body.Data, row)
				continue
			}
			live, lerr := queryPPPLive(ctx, d, tid, sub.RouterID, sub.Username)
			if lerr != nil {
				row.Error = "gagal membaca router"
				out.Body.Data = append(out.Body.Data, row)
				continue
			}
			row.Online = live.Online
			row.IPAddress = live.IPAddress
			row.Uptime = live.Uptime
			row.RxBps = live.RxBps
			row.TxBps = live.TxBps
			out.Body.Data = append(out.Body.Data, row)
		}
		return out, nil
	})

	// Pemakaian bulanan akun portal sendiri ("Pemakaian Bulan xxx").
	huma.Register(api, huma.Operation{
		OperationID: "portal-usage", Method: http.MethodGet, Path: "/api/portal/usage",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization"`
		Month         string `query:"month"`
	}) (*struct {
		Body struct {
			Month string               `json:"month"`
			Rx    int64                `json:"rx_bytes"`
			Tx    int64                `json:"tx_bytes"`
			Total int64                `json:"total_bytes"`
			Rows  []store.MonthlyUsage `json:"rows"`
		}
	}, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		month := time.Now()
		if m := strings.TrimSpace(input.Month); m != "" {
			if !trafficMonthRE.MatchString(m) {
				return nil, httpx.BadRequest("format bulan harus YYYY-MM")
			}
			if parsed, perr := time.Parse("2006-01", m); perr == nil {
				month = parsed
			}
		}
		monthKey := store.TrafficMonthKey(month)
		ids := make([]xid.ID, 0, len(custs))
		for _, c := range custs {
			if c != nil && !xid.IsNil(c.ID) {
				ids = append(ids, c.ID)
			}
		}
		rows, err := d.Store.MonthlyUsageMany(ctx, ten.ID, ids, monthKey)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Month string               `json:"month"`
				Rx    int64                `json:"rx_bytes"`
				Tx    int64                `json:"tx_bytes"`
				Total int64                `json:"total_bytes"`
				Rows  []store.MonthlyUsage `json:"rows"`
			}
		}{}
		out.Body.Month = monthKey.Format("2006-01")
		out.Body.Rows = rows
		for _, r := range rows {
			out.Body.Rx += r.RxBytes
			out.Body.Tx += r.TxBytes
		}
		out.Body.Total = out.Body.Rx + out.Body.Tx
		return out, nil
	})
}
