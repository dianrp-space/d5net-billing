package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

type Lead struct {
	ID             xid.ID     `json:"id"`
	TenantID       xid.ID     `json:"tenant_id"`
	FullName       string     `json:"full_name"`
	Phone          string     `json:"phone"`
	Email          *string    `json:"email,omitempty"`
	Address        *string    `json:"address,omitempty"`
	Latitude       *float64   `json:"latitude,omitempty"`
	Longitude      *float64   `json:"longitude,omitempty"`
	ODPID          *xid.ID    `json:"odp_id,omitempty"`
	ODPCode        string     `json:"odp_code,omitempty"`
	ODPName        string     `json:"odp_name,omitempty"`
	Status         string     `json:"status"`
	Notes          *string    `json:"notes,omitempty"`
	IdentityType   *string    `json:"identity_type,omitempty"`
	IdentityNumber *string    `json:"identity_number,omitempty"`
	ResellerID     *xid.ID    `json:"reseller_id,omitempty"`
	ResellerName   string     `json:"reseller_name,omitempty"`
	SalesUserID    *xid.ID    `json:"sales_user_id,omitempty"`
	SalesUserName  string     `json:"sales_user_name,omitempty"`
	AssignedTo     *xid.ID    `json:"assigned_to,omitempty"`
	AssignedToName string     `json:"assigned_to_name,omitempty"`
	CustomerID     *xid.ID    `json:"customer_id,omitempty"`
	ConvertedAt    *time.Time `json:"converted_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

var leadStatuses = map[string]bool{
	"new": true, "contacted": true, "survey": true, "qualified": true, "converted": true, "lost": true,
}

// LeadStatusNeedsAssignee: dihubungi → proses pasang harus punya teknisi.
func LeadStatusNeedsAssignee(status string) bool {
	switch NormalizeLeadStatus(status) {
	case "contacted", "survey", "qualified":
		return true
	default:
		return false
	}
}

func NormalizeLeadStatus(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "new"
	}
	if leadStatuses[s] {
		return s
	}
	return s
}

func (s *Store) CreateLead(ctx context.Context, l *Lead) error {
	l.Status = NormalizeLeadStatus(l.Status)
	if !leadStatuses[l.Status] {
		l.Status = "new"
	}
	if !LeadStatusNeedsAssignee(l.Status) {
		l.AssignedTo = nil
	} else if l.AssignedTo == nil || xid.IsNil(*l.AssignedTo) {
		return fmt.Errorf("teknisi wajib di-assign mulai status dihubungi")
	}
	l.ResellerID, l.SalesUserID = NormalizeAttribution(l.ResellerID, l.SalesUserID)
	return s.Pool.QueryRow(ctx, `
		INSERT INTO leads (tenant_id, full_name, phone, email, address, latitude, longitude, odp_id, status, notes, identity_type, identity_number, reseller_id, sales_user_id, assigned_to)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id, created_at
	`, l.TenantID, l.FullName, l.Phone, l.Email, l.Address, l.Latitude, l.Longitude, l.ODPID, l.Status, l.Notes, l.IdentityType, l.IdentityNumber, l.ResellerID, l.SalesUserID, l.AssignedTo).
		Scan(&l.ID, &l.CreatedAt)
}

func scanLead(row pgx.Row) (*Lead, error) {
	var l Lead
	err := row.Scan(
		&l.ID, &l.TenantID, &l.FullName, &l.Phone, &l.Email, &l.Address,
		&l.Latitude, &l.Longitude, &l.ODPID, &l.ODPCode, &l.ODPName,
		&l.Status, &l.Notes, &l.IdentityType, &l.IdentityNumber,
		&l.ResellerID, &l.ResellerName, &l.SalesUserID, &l.SalesUserName,
		&l.AssignedTo, &l.AssignedToName, &l.CustomerID, &l.ConvertedAt, &l.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

const leadSelect = `
	SELECT l.id, l.tenant_id, l.full_name, l.phone, l.email, l.address, l.latitude, l.longitude,
	       l.odp_id, COALESCE(o.code, ''), COALESCE(o.name, ''),
	       l.status, l.notes, l.identity_type, l.identity_number,
	       l.reseller_id, COALESCE(r.name, ''), l.sales_user_id, COALESCE(u.full_name, ''),
	       l.assigned_to, COALESCE(ua.full_name, ''),
	       l.customer_id, l.converted_at, l.created_at
	FROM leads l
	LEFT JOIN odps o ON o.id = l.odp_id
	LEFT JOIN resellers r ON r.id = l.reseller_id
	LEFT JOIN users u ON u.id = l.sales_user_id
	LEFT JOIN users ua ON ua.id = l.assigned_to
`

// convertedHideAfterDays: converted leads older than this are hidden from the
// default pipeline view (data is kept; pass hideConverted=false for history).
const convertedHideAfterDays = 7

func (s *Store) ListLeads(ctx context.Context, tenantID xid.ID, status string, assignedTo *xid.ID, hideConverted bool, limit, offset int) ([]Lead, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	where := `WHERE l.tenant_id=$1`
	args := []any{tenantID}
	if st := NormalizeLeadStatus(status); status != "" && leadStatuses[st] {
		args = append(args, st)
		where += fmt.Sprintf(` AND l.status=$%d`, len(args))
	}
	if assignedTo != nil && !xid.IsNil(*assignedTo) {
		args = append(args, *assignedTo)
		where += fmt.Sprintf(` AND l.assigned_to=$%d`, len(args))
	}
	if hideConverted {
		where += fmt.Sprintf(` AND NOT (l.status='converted' AND (l.converted_at IS NULL OR l.converted_at < NOW() - INTERVAL '%d days'))`, convertedHideAfterDays)
	}
	var total int64
	countQ := `SELECT COUNT(*) FROM leads l ` + where
	if err := s.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	lim := len(args) - 1
	off := len(args)
	q := leadSelect + ` ` + where + ` ORDER BY l.id DESC LIMIT $` + itoa(lim) + ` OFFSET $` + itoa(off)
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Lead
	for rows.Next() {
		var l Lead
		if err := rows.Scan(
			&l.ID, &l.TenantID, &l.FullName, &l.Phone, &l.Email, &l.Address,
			&l.Latitude, &l.Longitude, &l.ODPID, &l.ODPCode, &l.ODPName,
			&l.Status, &l.Notes, &l.IdentityType, &l.IdentityNumber,
			&l.ResellerID, &l.ResellerName, &l.SalesUserID, &l.SalesUserName,
			&l.AssignedTo, &l.AssignedToName, &l.CustomerID, &l.ConvertedAt, &l.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, l)
	}
	return list, total, rows.Err()
}

func (s *Store) GetLead(ctx context.Context, tenantID, id xid.ID) (*Lead, error) {
	return scanLead(s.Pool.QueryRow(ctx, leadSelect+`
		WHERE l.tenant_id=$1 AND l.id=$2
	`, tenantID, id))
}

type LeadComment struct {
	ID        xid.ID    `json:"id"`
	LeadID    xid.ID    `json:"lead_id"`
	UserID    *xid.ID   `json:"user_id,omitempty"`
	UserName  string    `json:"user_name,omitempty"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
	Kind      string    `json:"kind"` // comment | status_change
	Message   string    `json:"message"`
	ImageURLs []string  `json:"image_urls"`
	CreatedAt time.Time `json:"created_at"`
}

func LeadStatusLabel(status string) string {
	switch NormalizeLeadStatus(status) {
	case "new":
		return "Baru"
	case "contacted":
		return "Dihubungi"
	case "survey":
		return "Survey"
	case "qualified":
		return "Proses pasang"
	case "converted":
		return "Converted"
	case "lost":
		return "Lost"
	default:
		return status
	}
}

func (s *Store) ListLeadComments(ctx context.Context, tenantID, leadID xid.ID) ([]LeadComment, error) {
	if _, err := s.GetLead(ctx, tenantID, leadID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT c.id, c.lead_id, c.user_id, COALESCE(u.full_name, ''), u.avatar_url, COALESCE(c.kind, 'comment'),
		       c.message, COALESCE(c.image_urls, '[]'::jsonb), c.created_at
		FROM lead_comments c
		LEFT JOIN users u ON u.id = c.user_id
		WHERE c.tenant_id=$1 AND c.lead_id=$2
		ORDER BY c.created_at ASC
	`, tenantID, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []LeadComment
	for rows.Next() {
		var c LeadComment
		var raw []byte
		if err := rows.Scan(&c.ID, &c.LeadID, &c.UserID, &c.UserName, &c.AvatarURL, &c.Kind, &c.Message, &raw, &c.CreatedAt); err != nil {
			return nil, err
		}
		if c.Kind == "" {
			c.Kind = "comment"
		}
		c.ImageURLs = []string{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &c.ImageURLs)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) AddLeadComment(ctx context.Context, tenantID, leadID xid.ID, userID *xid.ID, message string, imageURLs []string) (*LeadComment, error) {
	return s.AddLeadActivity(ctx, tenantID, leadID, userID, "comment", message, imageURLs)
}

func (s *Store) AddLeadActivity(ctx context.Context, tenantID, leadID xid.ID, userID *xid.ID, kind, message string, imageURLs []string) (*LeadComment, error) {
	if _, err := s.GetLead(ctx, tenantID, leadID); err != nil {
		return nil, err
	}
	kind = strings.TrimSpace(strings.ToLower(kind))
	if kind != "status_change" {
		kind = "comment"
	}
	message = strings.TrimSpace(message)
	if imageURLs == nil {
		imageURLs = []string{}
	}
	if kind == "comment" && message == "" && len(imageURLs) == 0 {
		return nil, fmt.Errorf("pesan atau gambar wajib")
	}
	if kind == "status_change" && message == "" {
		return nil, fmt.Errorf("pesan status wajib")
	}
	raw, _ := json.Marshal(imageURLs)
	var c LeadComment
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO lead_comments (tenant_id, lead_id, user_id, kind, message, image_urls)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb)
		RETURNING id, lead_id, user_id, kind, message, COALESCE(image_urls, '[]'::jsonb), created_at
	`, tenantID, leadID, userID, kind, message, raw).Scan(&c.ID, &c.LeadID, &c.UserID, &c.Kind, &c.Message, &raw, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	c.ImageURLs = []string{}
	_ = json.Unmarshal(raw, &c.ImageURLs)
	if userID != nil {
		_ = s.Pool.QueryRow(ctx, `SELECT COALESCE(full_name,''), avatar_url FROM users WHERE id=$1`, *userID).Scan(&c.UserName, &c.AvatarURL)
	}
	return &c, nil
}

func (s *Store) UpdateLead(ctx context.Context, l *Lead) error {
	l.Status = NormalizeLeadStatus(l.Status)
	if !leadStatuses[l.Status] {
		return fmt.Errorf("status lead tidak valid")
	}
	if LeadStatusNeedsAssignee(l.Status) {
		if l.AssignedTo == nil || xid.IsNil(*l.AssignedTo) {
			return fmt.Errorf("teknisi wajib di-assign mulai status dihubungi")
		}
	} else {
		l.AssignedTo = nil
	}
	l.ResellerID, l.SalesUserID = NormalizeAttribution(l.ResellerID, l.SalesUserID)
	tag, err := s.Pool.Exec(ctx, `
		UPDATE leads SET full_name=$3, phone=$4, email=$5, address=$6, latitude=$7, longitude=$8,
		                 odp_id=$9, status=$10, notes=$11, identity_type=$12, identity_number=$13,
		                 reseller_id=$14, sales_user_id=$15,
		                 assigned_to=$16, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2 AND status <> 'converted'
	`, l.TenantID, l.ID, l.FullName, l.Phone, l.Email, l.Address, l.Latitude, l.Longitude, l.ODPID, l.Status, l.Notes, l.IdentityType, l.IdentityNumber, l.ResellerID, l.SalesUserID, l.AssignedTo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateLeadStatus moves a non-converted lead between pipeline columns (kanban).
// Status dihubungi / survey / proses pasang requires assignedTo.
// Status baru / lost clears assignment.
func (s *Store) UpdateLeadStatus(ctx context.Context, tenantID, id xid.ID, status string, assignedTo *xid.ID) error {
	status = NormalizeLeadStatus(status)
	if !leadStatuses[status] || status == "converted" {
		return fmt.Errorf("status lead tidak valid")
	}
	if LeadStatusNeedsAssignee(status) {
		if assignedTo == nil || xid.IsNil(*assignedTo) {
			cur, err := s.GetLead(ctx, tenantID, id)
			if err != nil {
				return err
			}
			if cur.AssignedTo != nil && !xid.IsNil(*cur.AssignedTo) {
				assignedTo = cur.AssignedTo
			} else {
				return fmt.Errorf("teknisi wajib di-assign mulai status dihubungi")
			}
		}
	} else {
		assignedTo = nil
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE leads SET status=$3, assigned_to=$4, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2 AND status <> 'converted'
	`, tenantID, id, status, assignedTo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignLead sets the technician user for a lead from dihubungi through proses pasang.
func (s *Store) AssignLead(ctx context.Context, tenantID, id xid.ID, assignedTo *xid.ID) error {
	if assignedTo == nil || xid.IsNil(*assignedTo) {
		return fmt.Errorf("teknisi wajib")
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE leads SET assigned_to=$3, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2 AND status IN ('contacted','survey','qualified')
	`, tenantID, id, assignedTo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteLead(ctx context.Context, tenantID, id xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM leads WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ConvertLeadToCustomer creates a customer from the lead and marks the lead converted.
// Optional resellerID/salesUserID override lead attribution (reseller wins if both set).
// commissionBasis selects flat amount type (new_customer_flat | acquisition_flat).
func (s *Store) ConvertLeadToCustomer(ctx context.Context, tenantID, leadID xid.ID, clusterID *xid.ID, customerCode string, resellerID, salesUserID *xid.ID, commissionBasis string) (*Customer, *Lead, error) {
	lead, err := s.GetLead(ctx, tenantID, leadID)
	if err != nil {
		return nil, nil, err
	}
	if lead.Status == "converted" || lead.CustomerID != nil {
		return nil, nil, fmt.Errorf("lead sudah dikonversi")
	}
	phone := strings.TrimSpace(lead.Phone)
	if phone == "" {
		return nil, nil, fmt.Errorf("telepon lead wajib untuk konversi")
	}
	code := strings.TrimSpace(customerCode)
	if code == "" {
		if clusterID != nil {
			code, err = s.NextCustomerCodeForCluster(ctx, tenantID, *clusterID)
			if err != nil {
				return nil, nil, err
			}
		} else {
			code, err = s.NextCustomerCode(ctx, tenantID)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	rID, sID := NormalizeAttribution(resellerID, salesUserID)
	if rID == nil && sID == nil {
		rID, sID = NormalizeAttribution(lead.ResellerID, lead.SalesUserID)
	}
	c := &Customer{
		TenantID: tenantID, ClusterID: clusterID, CustomerCode: code,
		FullName: lead.FullName, Email: lead.Email, Phone: phone, Address: lead.Address,
		Latitude: lead.Latitude, Longitude: lead.Longitude,
		IdentityType: lead.IdentityType, IdentityNumber: lead.IdentityNumber,
		IsActive: true, PortalEnabled: true,
		ResellerID: rID, SalesUserID: sID,
	}
	if err := s.CreateCustomer(ctx, c); err != nil {
		return nil, nil, err
	}
	_, _ = s.CopyLeadDocumentsToCustomer(ctx, tenantID, leadID, c.ID)
	now := time.Now()
	_, err = s.Pool.Exec(ctx, `
		UPDATE leads SET status='converted', customer_id=$3, converted_at=$4,
		                 reseller_id=$5, sales_user_id=$6, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, leadID, c.ID, now, rID, sID)
	if err != nil {
		return nil, nil, err
	}
	_, _ = s.CreditCommission(ctx, tenantID, c.ID, &leadID, rID, sID, commissionBasis)
	lead.Status = "converted"
	lead.CustomerID = &c.ID
	lead.ConvertedAt = &now
	lead.ResellerID = rID
	lead.SalesUserID = sID
	return c, lead, nil
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

func (s *Store) GetReseller(ctx context.Context, tenantID, id xid.ID) (*Reseller, error) {
	var r Reseller
	err := s.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, user_id, name, phone, commission_percent, balance, is_active, created_at
		FROM resellers WHERE tenant_id=$1 AND id=$2
	`, tenantID, id).Scan(&r.ID, &r.TenantID, &r.UserID, &r.Name, &r.Phone, &r.CommissionPercent, &r.Balance, &r.IsActive, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) UpdateReseller(ctx context.Context, r *Reseller) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE resellers SET name=$3, phone=$4, commission_percent=$5, is_active=$6
		WHERE tenant_id=$1 AND id=$2
	`, r.TenantID, r.ID, r.Name, r.Phone, r.CommissionPercent, r.IsActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteReseller(ctx context.Context, tenantID, id xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM resellers WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
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
	ID            xid.ID         `json:"id"`
	TenantID      xid.ID         `json:"tenant_id"`
	CustomerID    xid.ID         `json:"customer_id"`
	InvoiceID     *xid.ID        `json:"invoice_id,omitempty"`
	Provider      string         `json:"provider"`
	ExternalID    string         `json:"external_id"`
	Amount        int64          `json:"amount"`
	Status        string         `json:"status"`
	CheckoutURL   string         `json:"checkout_url,omitempty"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	QRString      string         `json:"qr_string,omitempty"`
	QRImageBase64 string         `json:"qr_image_base64,omitempty"`
	PayableAmount int64          `json:"payable_amount,omitempty"`
	UniqueDigit   int64          `json:"unique_digit,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func hydratePaymentIntentQR(pi *PaymentIntent) {
	if pi == nil || pi.Metadata == nil {
		return
	}
	if s, ok := pi.Metadata["qr_string"].(string); ok {
		pi.QRString = s
	}
	if s, ok := pi.Metadata["qr_image_base64"].(string); ok {
		pi.QRImageBase64 = s
	}
	if n := jsonNumber(pi.Metadata["payable_amount"]); n != 0 {
		pi.PayableAmount = n
	}
	if n := jsonNumber(pi.Metadata["unique_digit"]); n != 0 {
		pi.UniqueDigit = n
	}
}

func jsonNumber(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		n, _ := t.Int64()
		return n
	}
	return 0
}

func scanPaymentIntent(row interface {
	Scan(dest ...any) error
}) (*PaymentIntent, error) {
	var pi PaymentIntent
	var meta []byte
	err := row.Scan(&pi.ID, &pi.TenantID, &pi.CustomerID, &pi.InvoiceID, &pi.Provider, &pi.ExternalID,
		&pi.Amount, &pi.Status, &pi.CheckoutURL, &pi.ExpiresAt, &meta, &pi.CreatedAt, &pi.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &pi.Metadata)
	}
	hydratePaymentIntentQR(&pi)
	return &pi, nil
}

func paymentIntentSelectCols() string {
	return `id, tenant_id, customer_id, invoice_id, provider, COALESCE(external_id,''), amount, status,
		COALESCE(checkout_url,''), expires_at, COALESCE(metadata,'{}'::jsonb), created_at, updated_at`
}

func (s *Store) InsertPaymentIntent(ctx context.Context, pi *PaymentIntent) (*PaymentIntent, error) {
	if pi.Status == "" {
		pi.Status = "pending"
	}
	pi.Status = strings.ToLower(strings.TrimSpace(pi.Status))
	if pi.Metadata == nil {
		pi.Metadata = map[string]any{}
	}
	if pi.QRString != "" {
		pi.Metadata["qr_string"] = pi.QRString
	}
	if pi.QRImageBase64 != "" {
		pi.Metadata["qr_image_base64"] = pi.QRImageBase64
	}
	if pi.PayableAmount != 0 {
		pi.Metadata["payable_amount"] = pi.PayableAmount
	}
	if pi.UniqueDigit != 0 {
		pi.Metadata["unique_digit"] = pi.UniqueDigit
	}
	meta, err := json.Marshal(pi.Metadata)
	if err != nil {
		return nil, err
	}
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO payment_intents (tenant_id, customer_id, invoice_id, provider, external_id, amount, status, checkout_url, expires_at, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10)
		RETURNING `+paymentIntentSelectCols(),
		pi.TenantID, pi.CustomerID, pi.InvoiceID, pi.Provider, pi.ExternalID, pi.Amount, pi.Status, pi.CheckoutURL, pi.ExpiresAt, meta)
	out, err := scanPaymentIntent(row)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetLatestPaymentIntentForInvoice(ctx context.Context, tenantID, invoiceID xid.ID, provider string) (*PaymentIntent, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+paymentIntentSelectCols()+`
		FROM payment_intents
		WHERE tenant_id=$1 AND invoice_id=$2 AND provider=$3
		ORDER BY created_at DESC LIMIT 1
	`, tenantID, invoiceID, provider)
	pi, err := scanPaymentIntent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return pi, err
}

func (s *Store) GetLatestPaymentIntentForInvoiceAny(ctx context.Context, tenantID, invoiceID xid.ID) (*PaymentIntent, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+paymentIntentSelectCols()+`
		FROM payment_intents
		WHERE tenant_id=$1 AND invoice_id=$2
		ORDER BY created_at DESC LIMIT 1
	`, tenantID, invoiceID)
	pi, err := scanPaymentIntent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return pi, err
}

func (s *Store) GetLatestPendingPaymentIntent(ctx context.Context, tenantID, invoiceID xid.ID, provider string) (*PaymentIntent, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+paymentIntentSelectCols()+`
		FROM payment_intents
		WHERE tenant_id=$1 AND invoice_id=$2 AND provider=$3 AND status='pending'
		  AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC LIMIT 1
	`, tenantID, invoiceID, provider)
	pi, err := scanPaymentIntent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return pi, err
}

func (s *Store) ListCustomerOpenPaymentIntents(ctx context.Context, tenantID, customerID xid.ID, limit int) ([]PaymentIntent, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT `+paymentIntentSelectCols()+`
		FROM payment_intents
		WHERE tenant_id=$1 AND customer_id=$2
		  AND LOWER(status) NOT IN ('paid','success')
		ORDER BY created_at DESC
		LIMIT $3
	`, tenantID, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PaymentIntent
	for rows.Next() {
		pi, err := scanPaymentIntent(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *pi)
	}
	return list, rows.Err()
}

func (s *Store) InvoiceNumberByID(ctx context.Context, tenantID, invoiceID xid.ID) string {
	var num string
	_ = s.Pool.QueryRow(ctx, `SELECT invoice_number FROM invoices WHERE tenant_id=$1 AND id=$2`, tenantID, invoiceID).Scan(&num)
	return num
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
	var dismantled, active int64
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM customers
		WHERE tenant_id=$1 AND dismantled_at >= NOW() - INTERVAL '30 days'
	`, tenantID).Scan(&dismantled)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM customers
		WHERE tenant_id=$1 AND is_active=true AND dismantled_at IS NULL
	`, tenantID).Scan(&active)
	ratio := 0.0
	denom := dismantled + active
	if denom > 0 {
		ratio = float64(dismantled) / float64(denom)
	}
	return map[string]any{
		"dismantled_30d": dismantled,
		"canceled_30d":   dismantled, // alias lama (sebelumnya dari langganan)
		"active":         active,
		"churn_ratio":    ratio,
		"window_days":    30,
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
