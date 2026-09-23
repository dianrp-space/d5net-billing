package store

import (
	"context"
	"errors"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

var errNotifyNum = errors.New("invalid number")

const jobsSettingKey = "jobs.schedule"

// JobScheduleSettings configures which worker tasks run for a tenant and when.
type JobScheduleSettings struct {
	BillingEnabled         bool   `json:"billing_enabled"`
	IsolirEnabled          bool   `json:"isolir_enabled"`
	DunningEnabled         bool   `json:"dunning_enabled"`
	DunningOffsets         []int  `json:"dunning_offsets,omitempty"`
	// DunningTime is the earliest local time (HH:MM) reminder notifications are
	// sent on a matching dunning day. Worker cycles before this time skip.
	DunningTime string `json:"dunning_time,omitempty"`
	// InvoiceIssuedTime is the local time (HH:MM) "tagihan terbit" notifications
	// are sent. Invoices created before this time queue the notif for this time
	// today; invoices created after send immediately.
	InvoiceIssuedTime        string `json:"invoice_issued_time,omitempty"`
	OdpOutageEnabled         bool   `json:"odp_outage_enabled,omitempty"`
	WeeklyReconcileEnabled   bool   `json:"weekly_reconcile_enabled"`
	WeeklyReconcileWeekday   int    `json:"weekly_reconcile_weekday"` // 0=Sunday … 6=Saturday
	WeeklyReconcileHour      int    `json:"weekly_reconcile_hour"`    // 0–23 local
	MonthlyReportEnabled     bool   `json:"monthly_report_enabled"`
	MonthlyReportDay         int    `json:"monthly_report_day"`  // 1–28
	MonthlyReportHour        int    `json:"monthly_report_hour"` // 0–23 local
	NotifyBatchSize          int    `json:"notify_batch_size"`
	// NotifLogRetentionDays adalah retensi log notifikasi (riwayat) dalam hari.
	// 0 = nonaktif (jangan hapus otomatis). Worker menghapus otomatis tiap hari
	// hanya untuk log final (sent/failed); antrean pending tidak pernah dihapus.
	NotifLogRetentionDays int `json:"notif_log_retention_days,omitempty"`
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
		DunningTime:            "08:00",
		InvoiceIssuedTime:      "08:00",
		OdpOutageEnabled:       false,
		WeeklyReconcileEnabled: true,
		WeeklyReconcileWeekday: 0,
		WeeklyReconcileHour:    3,
		MonthlyReportEnabled:   true,
		MonthlyReportDay:       1,
		MonthlyReportHour:      8,
		NotifyBatchSize:        100,
		NotifLogRetentionDays:  0, // nonaktif secara bawaan; user mengaktifkan dari Riwayat notifikasi
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
	cfg.DunningTime = NormalizeNotifyTime(cfg.DunningTime, def.DunningTime)
	cfg.InvoiceIssuedTime = NormalizeNotifyTime(cfg.InvoiceIssuedTime, def.InvoiceIssuedTime)
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
	// 0 = retensi otomatis nonaktif; selain itu clamp 1–365 hari.
	if cfg.NotifLogRetentionDays < 0 {
		cfg.NotifLogRetentionDays = def.NotifLogRetentionDays
	}
	if cfg.NotifLogRetentionDays > 365 {
		cfg.NotifLogRetentionDays = 365
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

// NormalizeNotifyTime validates an "HH:MM" (24h) notify time, falling back to
// fallback (then "08:00") when invalid.
func NormalizeNotifyTime(raw, fallback string) string {
	if h, m, ok := ParseNotifyTime(raw); ok {
		return formatNotifyTime(h, m)
	}
	if h, m, ok := ParseNotifyTime(fallback); ok {
		return formatNotifyTime(h, m)
	}
	return "08:00"
}

// ParseNotifyTime parses "HH:MM" (also accepts "H:MM", "HH.MM", "HHMM").
func ParseNotifyTime(raw string) (hour, minute int, ok bool) {
	s := ""
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			s += string(r)
		} else if r == ':' || r == '.' {
			s += ":"
		}
	}
	parts := splitNotifyTime(s)
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, herr := atoiNotify(parts[0])
	m, merr := atoiNotify(parts[1])
	if herr != nil || merr != nil {
		return 0, 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

func formatNotifyTime(h, m int) string {
	return twoDigits(h) + ":" + twoDigits(m)
}

func splitNotifyTime(s string) []string {
	var parts []string
	cur := ""
	for _, r := range s {
		if r == ':' {
			parts = append(parts, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	parts = append(parts, cur)
	return parts
}

func atoiNotify(s string) (int, error) {
	if s == "" {
		return 0, errNotifyNum
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errNotifyNum
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// ScheduledAtForNotifyTime returns today at HH:MM (local) when now is still
// before that time, so the caller can delay queueing until then. Returns nil
// when notifications may be sent immediately.
func ScheduledAtForNotifyTime(now time.Time, hhmm string) *time.Time {
	h, m, ok := ParseNotifyTime(hhmm)
	if !ok {
		return nil
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if now.Before(at) {
		return &at
	}
	return nil
}

// PastNotifyTime reports whether local now has reached today's HH:MM.
func PastNotifyTime(now time.Time, hhmm string) bool {
	return ScheduledAtForNotifyTime(now, hhmm) == nil
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
