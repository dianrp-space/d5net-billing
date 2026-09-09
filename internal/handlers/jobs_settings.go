package handlers

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/job"
	"github.com/dianrp/drp-billing/internal/store"
)

func registerJobsSettings(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-jobs-settings", Method: http.MethodGet, Path: "/api/settings/jobs",
		Summary: "Get worker / cronjob schedule settings", Tags: []string{"Settings"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Schedule        store.JobScheduleSettings `json:"schedule"`
			Defaults        store.JobScheduleSettings `json:"defaults"`
			Catalog         []jobCatalogItem          `json:"catalog"`
			IsolirGraceDays int                       `json:"isolir_grace_days"`
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
		gen, err := d.Store.GetGeneralSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Schedule        store.JobScheduleSettings `json:"schedule"`
				Defaults        store.JobScheduleSettings `json:"defaults"`
				Catalog         []jobCatalogItem          `json:"catalog"`
				IsolirGraceDays int                       `json:"isolir_grace_days"`
			}
		}{}
		out.Body.Schedule = cfg
		out.Body.Defaults = store.DefaultJobScheduleSettings()
		out.Body.Catalog = jobsCatalog()
		out.Body.IsolirGraceDays = gen.IsolirGraceDays
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-jobs-settings", Method: http.MethodPut, Path: "/api/settings/jobs",
		Summary: "Update worker / cronjob schedule settings", Tags: []string{"Settings"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body jobsSchedulePayload
	}) (*struct {
		Body jobsSchedulePayload
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		cfg := store.NormalizeJobScheduleSettings(input.Body.JobScheduleSettings)
		if err := d.Store.UpsertJobScheduleSettings(ctx, tid, cfg); err != nil {
			return nil, httpx.Internal(err)
		}
		gen, err := d.Store.GetGeneralSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		gen.IsolirGraceDays = input.Body.IsolirGraceDays
		if err := d.Store.UpsertGeneralSettings(ctx, tid, gen); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body jobsSchedulePayload }{Body: jobsSchedulePayload{
			JobScheduleSettings: cfg,
			IsolirGraceDays:     store.NormalizeGeneralSettings(gen).IsolirGraceDays,
		}}, nil
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

	huma.Register(api, huma.Operation{
		OperationID: "run-jobs-now", Method: http.MethodPost, Path: "/api/settings/jobs/run",
		Summary: "Run worker cycle now for this tenant", Tags: []string{"Settings"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body job.TenantCycleResult
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		if d.Jobs == nil {
			return nil, httpx.BadRequest("worker tidak tersedia di proses API")
		}
		runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		res, err := d.Jobs.RunTenantNow(runCtx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body job.TenantCycleResult }{Body: res}, nil
	})
}

type jobsSchedulePayload struct {
	store.JobScheduleSettings
	IsolirGraceDays int `json:"isolir_grace_days"`
}

type jobCatalogItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func jobsCatalog() []jobCatalogItem {
	items := []jobCatalogItem{
		{ID: "billing", Label: "Generate tagihan", Description: "Buat invoice untuk langganan yang jatuh tempo (mengikuti interval worker di halaman ini)."},
		{ID: "isolir", Label: "Auto isolir", Description: "Suspend langganan yang tagihannya lewat jatuh tempo + masa tenggang isolir (Pengaturan → Umum). Setelah lunas, retry resume ke profil paket jika router sempat gagal/offline."},
		{ID: "dunning", Label: "Pengingat tagihan (dunning)", Description: "Kirim reminder WhatsApp/email pada offset hari relatif jatuh tempo."},
		{ID: "weekly_reconcile", Label: "Reconcile mingguan", Description: "Dry-run drift RouterOS vs billing (sekali per jadwal). Alert Telegram ops jika ada drift."},
		{ID: "monthly_report", Label: "Laporan bulanan", Description: "Email ringkas statistik bisnis ke email tenant."},
		{ID: "poller", Label: "Poller router", Description: "Login API MikroTik untuk sesi PPPoE aktif dan metrik CPU/memori. Terpisah dari interval worker (tagihan/isolir)."},
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}
