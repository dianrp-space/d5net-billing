package store

import (
	"context"
	"errors"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
)

const jobsSettingKey = "jobs.schedule"

// JobScheduleSettings configures which worker tasks run for a tenant and when.
type JobScheduleSettings struct {
	BillingEnabled         bool  `json:"billing_enabled"`
	IsolirEnabled          bool  `json:"isolir_enabled"`
	DunningEnabled         bool  `json:"dunning_enabled"`
	DunningOffsets         []int `json:"dunning_offsets,omitempty"`
	OdpOutageEnabled       bool  `json:"odp_outage_enabled,omitempty"`
	WeeklyReconcileEnabled bool  `json:"weekly_reconcile_enabled"`
	WeeklyReconcileWeekday int   `json:"weekly_reconcile_weekday"` // 0=Sunday … 6=Saturday
	WeeklyReconcileHour    int   `json:"weekly_reconcile_hour"`    // 0–23 local
	MonthlyReportEnabled   bool  `json:"monthly_report_enabled"`
	MonthlyReportDay       int   `json:"monthly_report_day"`  // 1–28
	MonthlyReportHour      int   `json:"monthly_report_hour"` // 0–23 local
	NotifyBatchSize        int   `json:"notify_batch_size"`
	// CycleIntervalSeconds is how often this tenant's worker tasks run (billing, isolir, dunning).
	CycleIntervalSeconds int `json:"cycle_interval_seconds"`
	// PollerIntervalSeconds is how often the worker logs into MikroTik API for sessions/metrics.
	PollerIntervalSeconds int `json:"poller_interval_seconds"`
}

func DefaultJobScheduleSettings() JobScheduleSettings {
	return JobScheduleSettings{
		BillingEnabled:         true,
		IsolirEnabled:          true,
		DunningEnabled:         true,
		DunningOffsets:         []int{-7, -3, 0, 1, 3},
		OdpOutageEnabled:       false,
		WeeklyReconcileEnabled: true,
		WeeklyReconcileWeekday: 0,
		WeeklyReconcileHour:    3,
		MonthlyReportEnabled:   true,
		MonthlyReportDay:       1,
		MonthlyReportHour:      8,
		NotifyBatchSize:        100,
		CycleIntervalSeconds:   60,
		PollerIntervalSeconds:  300,
	}
}

func (s *Store) GetJobScheduleSettings(ctx context.Context, tenantID xid.ID) (JobScheduleSettings, error) {
	cfg := DefaultJobScheduleSettings()
	err := s.GetSettingJSON(ctx, tenantID, jobsSettingKey, &cfg)
	if errors.Is(err, ErrNotFound) {
		return DefaultJobScheduleSettings(), nil
	}
	if err != nil {
		return DefaultJobScheduleSettings(), err
	}
	return NormalizeJobScheduleSettings(cfg), nil
}

func (s *Store) UpsertJobScheduleSettings(ctx context.Context, tenantID xid.ID, cfg JobScheduleSettings) error {
	return s.UpsertSettingJSON(ctx, tenantID, jobsSettingKey, NormalizeJobScheduleSettings(cfg))
}

func NormalizeJobScheduleSettings(cfg JobScheduleSettings) JobScheduleSettings {
	def := DefaultJobScheduleSettings()
	if len(cfg.DunningOffsets) == 0 {
		cfg.DunningOffsets = def.DunningOffsets
	}
	if cfg.WeeklyReconcileWeekday < 0 || cfg.WeeklyReconcileWeekday > 6 {
		cfg.WeeklyReconcileWeekday = def.WeeklyReconcileWeekday
	}
	if cfg.WeeklyReconcileHour < 0 || cfg.WeeklyReconcileHour > 23 {
		cfg.WeeklyReconcileHour = def.WeeklyReconcileHour
	}
	if cfg.MonthlyReportDay < 1 || cfg.MonthlyReportDay > 28 {
		cfg.MonthlyReportDay = def.MonthlyReportDay
	}
	if cfg.MonthlyReportHour < 0 || cfg.MonthlyReportHour > 23 {
		cfg.MonthlyReportHour = def.MonthlyReportHour
	}
	if cfg.NotifyBatchSize < 1 {
		cfg.NotifyBatchSize = def.NotifyBatchSize
	}
	if cfg.NotifyBatchSize > 500 {
		cfg.NotifyBatchSize = 500
	}
	if cfg.CycleIntervalSeconds < 60 {
		cfg.CycleIntervalSeconds = def.CycleIntervalSeconds
	}
	if cfg.CycleIntervalSeconds > 3600 {
		cfg.CycleIntervalSeconds = 3600
	}
	// Snap to whole minutes so the Jobs UI (menit) round-trips cleanly.
	if rest := cfg.CycleIntervalSeconds % 60; rest != 0 {
		cfg.CycleIntervalSeconds -= rest
		if cfg.CycleIntervalSeconds < 60 {
			cfg.CycleIntervalSeconds = 60
		}
	}
	if cfg.PollerIntervalSeconds < 60 {
		cfg.PollerIntervalSeconds = def.PollerIntervalSeconds
	}
	if cfg.PollerIntervalSeconds > 3600 {
		cfg.PollerIntervalSeconds = 3600
	}
	if rest := cfg.PollerIntervalSeconds % 60; rest != 0 {
		cfg.PollerIntervalSeconds -= rest
		if cfg.PollerIntervalSeconds < 60 {
			cfg.PollerIntervalSeconds = def.PollerIntervalSeconds
		}
	}
	// Dedup / clamp dunning offsets to [-30, 30]
	seen := map[int]struct{}{}
	offsets := make([]int, 0, len(cfg.DunningOffsets))
	for _, d := range cfg.DunningOffsets {
		if d < -30 || d > 30 {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		offsets = append(offsets, d)
	}
	if len(offsets) == 0 {
		offsets = def.DunningOffsets
	}
	cfg.DunningOffsets = offsets
	cfg.OdpOutageEnabled = false
	return cfg
}

type JobRun struct {
	ID        xid.ID    `json:"id"`
	JobName   string    `json:"job_name"`
	JobKey    string    `json:"job_key"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListJobRuns(ctx context.Context, tenantID xid.ID, limit int) ([]JobRun, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, job_name, job_key, status, created_at
		FROM job_runs
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []JobRun
	for rows.Next() {
		var r JobRun
		if err := rows.Scan(&r.ID, &r.JobName, &r.JobKey, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	if list == nil {
		list = []JobRun{}
	}
	return list, rows.Err()
}
