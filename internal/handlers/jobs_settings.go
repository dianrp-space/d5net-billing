package handlers

import (
	"context"
	"net/http"
	"sort"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/store"
)

func registerJobsSettings(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-jobs-settings", Method: http.MethodGet, Path: "/api/settings/jobs",
		Summary: "Get worker / cronjob schedule settings", Tags: []string{"Settings"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Schedule store.JobScheduleSettings `json:"schedule"`
			Defaults store.JobScheduleSettings `json:"defaults"`
			Catalog  []jobCatalogItem          `json:"catalog"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		cfg, err := d.Store.GetJobScheduleSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Schedule store.JobScheduleSettings `json:"schedule"`
				Defaults store.JobScheduleSettings `json:"defaults"`
				Catalog  []jobCatalogItem          `json:"catalog"`
			}
		}{}
		out.Body.Schedule = cfg
		out.Body.Defaults = store.DefaultJobScheduleSettings()
		out.Body.Catalog = jobsCatalog()
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-jobs-settings", Method: http.MethodPut, Path: "/api/settings/jobs",
		Summary: "Update worker / cronjob schedule settings", Tags: []string{"Settings"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body store.JobScheduleSettings
	}) (*struct {
		Body store.JobScheduleSettings
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		cfg := store.NormalizeJobScheduleSettings(input.Body)
		if err := d.Store.UpsertJobScheduleSettings(ctx, tid, cfg); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.JobScheduleSettings }{Body: cfg}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-job-runs", Method: http.MethodGet, Path: "/api/settings/jobs/runs",
		Summary: "Recent idempotent job runs for this tenant", Tags: []string{"Settings"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Limit int `query:"limit"`
	}) (*struct {
		Body struct {
			Data []store.JobRun `json:"data"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListJobRuns(ctx, tid, input.Limit)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Data []store.JobRun `json:"data"`
			}
		}{}
		out.Body.Data = list
		return out, nil
	})
}

type jobCatalogItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func jobsCatalog() []jobCatalogItem {
	items := []jobCatalogItem{
		{ID: "billing", Label: "Generate tagihan", Description: "Buat invoice untuk langganan yang jatuh tempo (tiap siklus worker ~1 menit)."},
		{ID: "isolir", Label: "Auto isolir", Description: "Suspend langganan yang tagihannya lewat jatuh tempo + grace paket."},
		{ID: "dunning", Label: "Pengingat tagihan (dunning)", Description: "Kirim reminder WhatsApp/email pada offset hari relatif jatuh tempo."},
		{ID: "odp_outage", Label: "Deteksi gangguan ODP", Description: "Cek rasio offline pelanggan per ODP dan buat alert."},
		{ID: "weekly_reconcile", Label: "Reconcile mingguan", Description: "Dry-run drift RouterOS vs data billing (sekali per jadwal)."},
		{ID: "monthly_report", Label: "Laporan bulanan", Description: "Email ringkas statistik bisnis ke email tenant."},
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}
