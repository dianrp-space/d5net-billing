package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dianrp/drp-billing/internal/xid"
	"time"

	"github.com/jackc/pgx/v5"
)

type Lead struct {
	ID        xid.ID    `json:"id"`
	TenantID  xid.ID    `json:"tenant_id"`
	FullName  string    `json:"full_name"`
	Phone     string    `json:"phone"`
	Email     *string   `json:"email,omitempty"`
	Address   *string   `json:"address,omitempty"`
	Latitude  *float64  `json:"latitude,omitempty"`
	Longitude *float64  `json:"longitude,omitempty"`
	ODPID     *xid.ID   `json:"odp_id,omitempty"`
	Status    string    `json:"status"`
	Notes     *string   `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) CreateLead(ctx context.Context, l *Lead) error {
	if l.Status == "" {
		l.Status = "new"
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO leads (tenant_id, full_name, phone, email, address, latitude, longitude, odp_id, status, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id, created_at
	`, l.TenantID, l.FullName, l.Phone, l.Email, l.Address, l.Latitude, l.Longitude, l.ODPID, l.Status, l.Notes).
		Scan(&l.ID, &l.CreatedAt)
}

func (s *Store) ListLeads(ctx context.Context, tenantID xid.ID, limit, offset int) ([]Lead, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM leads WHERE tenant_id=$1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, full_name, phone, email, address, latitude, longitude, odp_id, status, notes, created_at
		FROM leads WHERE tenant_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Lead
	for rows.Next() {
		var l Lead
		if err := rows.Scan(&l.ID, &l.TenantID, &l.FullName, &l.Phone, &l.Email, &l.Address,
			&l.Latitude, &l.Longitude, &l.ODPID, &l.Status, &l.Notes, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, l)
	}
	return list, total, rows.Err()
}

type Reseller struct {
	ID                xid.ID    `json:"id"`
	TenantID          xid.ID    `json:"tenant_id"`
	UserID            *xid.ID   `json:"user_id,omitempty"`
	Name              string    `json:"name"`
	Phone             *string   `json:"phone,omitempty"`
	CommissionPercent float64   `json:"commission_percent"`
	Balance           int64     `json:"balance"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
}

func (s *Store) ListResellers(ctx context.Context, tenantID xid.ID) ([]Reseller, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, user_id, name, phone, commission_percent, balance, is_active, created_at
		FROM resellers WHERE tenant_id=$1 ORDER BY name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Reseller
	for rows.Next() {
		var r Reseller
		if err := rows.Scan(&r.ID, &r.TenantID, &r.UserID, &r.Name, &r.Phone, &r.CommissionPercent, &r.Balance, &r.IsActive, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *Store) CreateResellerFull(ctx context.Context, r *Reseller) error {
	r.IsActive = true
	return s.Pool.QueryRow(ctx, `
		INSERT INTO resellers (tenant_id, name, phone, commission_percent, is_active)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, balance, is_active, created_at
	`, r.TenantID, r.Name, r.Phone, r.CommissionPercent, r.IsActive).Scan(&r.ID, &r.Balance, &r.IsActive, &r.CreatedAt)
}

type Alert struct {
	ID         xid.ID    `json:"id"`
	TenantID   xid.ID    `json:"tenant_id"`
	Severity   string    `json:"severity"`
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Message    string    `json:"message"`
	EntityType *string   `json:"entity_type,omitempty"`
	EntityID   *xid.ID   `json:"entity_id,omitempty"`
	IsAcked    bool      `json:"is_acked"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) CreateAlert(ctx context.Context, a *Alert) error {
	if a.Severity == "" {
		a.Severity = "warn"
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO alerts (tenant_id, severity, kind, title, message, entity_type, entity_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at
	`, a.TenantID, a.Severity, a.Kind, a.Title, a.Message, a.EntityType, a.EntityID).Scan(&a.ID, &a.CreatedAt)
}

func (s *Store) ListAlerts(ctx context.Context, tenantID xid.ID, limit int) ([]Alert, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, severity, kind, title, message, entity_type, entity_id, is_acked, created_at
		FROM alerts WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Severity, &a.Kind, &a.Title, &a.Message,
			&a.EntityType, &a.EntityID, &a.IsAcked, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

type PaymentIntent struct {
	ID          xid.ID    `json:"id"`
	TenantID    xid.ID    `json:"tenant_id"`
	CustomerID  xid.ID    `json:"customer_id"`
	InvoiceID   *xid.ID   `json:"invoice_id,omitempty"`
	Provider    string    `json:"provider"`
	ExternalID  string    `json:"external_id"`
	Amount      int64     `json:"amount"`
	Status      string    `json:"status"`
	CheckoutURL string    `json:"checkout_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Store) InsertPaymentIntent(ctx context.Context, tenantID xid.ID, customerID xid.ID, invoiceID *xid.ID, provider, externalID string, amount int64, status, checkoutURL string) (*PaymentIntent, error) {
	if status == "" {
		status = "pending"
	}
	var pi PaymentIntent
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO payment_intents (tenant_id, customer_id, invoice_id, provider, external_id, amount, status, checkout_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')) RETURNING id, tenant_id, customer_id, invoice_id, provider,
			COALESCE(external_id,''), amount, status, COALESCE(checkout_url,''), created_at, updated_at
	`, tenantID, customerID, invoiceID, provider, externalID, amount, status, checkoutURL).
		Scan(&pi.ID, &pi.TenantID, &pi.CustomerID, &pi.InvoiceID, &pi.Provider, &pi.ExternalID,
			&pi.Amount, &pi.Status, &pi.CheckoutURL, &pi.CreatedAt, &pi.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &pi, nil
}

// ClaimJob inserts a unique job_runs row; returns true if this caller claimed the job.
func (s *Store) ClaimJob(ctx context.Context, tenantID xid.ID, jobName, jobKey string) (bool, error) {
	var id xid.ID
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO job_runs (tenant_id, job_name, job_key, status)
		VALUES ($1,$2,$3,'done')
		ON CONFLICT (tenant_id, job_name, job_key) DO NOTHING
		RETURNING id
	`, tenantID, jobName, jobKey).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !xid.IsNil(id), nil
}

func (s *Store) FindCashAndRevenueAccounts(ctx context.Context, tenantID xid.ID) (cashID, revenueID xid.ID, err error) {
	cashID, _ = s.FindAccountByTypeOrCode(ctx, tenantID, "asset", "1110")
	if xid.IsNil(cashID) {
		cashID, _ = s.FindAccountByTypeOrCode(ctx, tenantID, "cash", "1000")
	}
	revenueID, _ = s.FindAccountByTypeOrCode(ctx, tenantID, "revenue", "4110")
	if xid.IsNil(revenueID) {
		revenueID, _ = s.FindAccountByTypeOrCode(ctx, tenantID, "income", "4000")
	}
	return cashID, revenueID, nil
}

type OutboundWebhook struct {
	ID        xid.ID    `json:"id"`
	TenantID  xid.ID    `json:"tenant_id"`
	URL       string    `json:"url"`
	Secret    *string   `json:"secret,omitempty"`
	Events    []string  `json:"events"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListOutboundWebhooks(ctx context.Context, tenantID xid.ID) ([]OutboundWebhook, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, url, secret, events, is_active, created_at
		FROM outbound_webhooks WHERE tenant_id=$1 ORDER BY id
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []OutboundWebhook
	for rows.Next() {
		var w OutboundWebhook
		if err := rows.Scan(&w.ID, &w.TenantID, &w.URL, &w.Secret, &w.Events, &w.IsActive, &w.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

func (s *Store) CreateOutboundWebhook(ctx context.Context, w *OutboundWebhook) error {
	if w.Events == nil {
		w.Events = []string{}
	}
	w.IsActive = true
	return s.Pool.QueryRow(ctx, `
		INSERT INTO outbound_webhooks (tenant_id, url, secret, events, is_active)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, is_active, created_at
	`, w.TenantID, w.URL, w.Secret, w.Events, w.IsActive).Scan(&w.ID, &w.IsActive, &w.CreatedAt)
}

func (s *Store) DeleteOutboundWebhook(ctx context.Context, tenantID xid.ID, id xid.ID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM outbound_webhooks WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	return err
}

func (s *Store) EnqueueOutboundDelivery(ctx context.Context, tenantID xid.ID, webhookID xid.ID, event string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO outbound_webhook_deliveries (tenant_id, webhook_id, event, payload, status)
		VALUES ($1,$2,$3,$4,'pending')
	`, tenantID, webhookID, event, b)
	return err
}

type TrafficSample struct {
	ID             xid.ID    `json:"id"`
	TenantID       xid.ID    `json:"tenant_id"`
	RouterID       *xid.ID   `json:"router_id,omitempty"`
	SubscriptionID *xid.ID   `json:"subscription_id,omitempty"`
	RxBytes        int64     `json:"rx_bytes"`
	TxBytes        int64     `json:"tx_bytes"`
	RecordedAt     time.Time `json:"recorded_at"`
}

func (s *Store) ListTrafficSamples(ctx context.Context, tenantID xid.ID, subscriptionID xid.ID, limit int) ([]TrafficSample, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, router_id, subscription_id, rx_bytes, tx_bytes, recorded_at
		FROM traffic_samples
		WHERE tenant_id=$1 AND subscription_id=$2
		ORDER BY recorded_at DESC LIMIT $3
	`, tenantID, subscriptionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TrafficSample
	for rows.Next() {
		var t TrafficSample
		if err := rows.Scan(&t.ID, &t.TenantID, &t.RouterID, &t.SubscriptionID, &t.RxBytes, &t.TxBytes, &t.RecordedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) ListVoucherCodes(ctx context.Context, tenantID xid.ID, batchID xid.ID) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT code FROM vouchers WHERE tenant_id=$1 AND batch_id=$2 ORDER BY id
	`, tenantID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		codes = append(codes, c)
	}
	return codes, rows.Err()
}

func (s *Store) ChurnReport(ctx context.Context, tenantID xid.ID) (map[string]any, error) {
	var canceled, active int64
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM subscriptions
		WHERE tenant_id=$1 AND status IN ('canceled','cancelled')
		  AND updated_at >= NOW() - INTERVAL '30 days'
	`, tenantID).Scan(&canceled)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM subscriptions WHERE tenant_id=$1 AND status='active'
	`, tenantID).Scan(&active)
	ratio := 0.0
	denom := canceled + active
	if denom > 0 {
		ratio = float64(canceled) / float64(denom)
	}
	return map[string]any{
		"canceled_30d": canceled,
		"active":       active,
		"churn_ratio":  ratio,
		"window_days":  30,
	}, nil
}

func (s *Store) FindAccountByTypeOrCode(ctx context.Context, tenantID xid.ID, typ, code string) (xid.ID, error) {
	var id xid.ID
	err := s.Pool.QueryRow(ctx, `
		SELECT id FROM chart_of_accounts
		WHERE tenant_id=$1 AND is_active=true AND (LOWER(type)=LOWER($2) OR code=$3)
		ORDER BY CASE WHEN LOWER(type)=LOWER($2) THEN 0 ELSE 1 END, code
		LIMIT 1
	`, tenantID, typ, code).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return xid.Nil(), ErrNotFound
	}
	return id, err
}
