package store

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

type Ticket struct {
	ID           xid.ID     `json:"id"`
	TenantID     xid.ID     `json:"tenant_id"`
	CustomerID   *xid.ID    `json:"customer_id,omitempty"`
	CustomerName string     `json:"customer_name,omitempty"`
	Subject      string     `json:"subject"`
	Description  *string    `json:"description,omitempty"`
	Category     string     `json:"category"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	AssignedTo   *xid.ID    `json:"assigned_to,omitempty"`
	AssigneeName string     `json:"assignee_name,omitempty"`
	SLADueAt     *time.Time `json:"sla_due_at,omitempty"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type TicketMessage struct {
	ID         xid.ID    `json:"id"`
	TicketID   xid.ID    `json:"ticket_id"`
	SenderType string    `json:"sender_type"`
	SenderID   *xid.ID   `json:"sender_id,omitempty"`
	SenderName string    `json:"sender_name,omitempty"`
	Message    string    `json:"message"`
	ImageURLs  []string  `json:"image_urls"`
	CreatedAt  time.Time `json:"created_at"`
}

var ticketStatuses = map[string]bool{
	"open": true, "in_progress": true, "resolved": true, "closed": true,
}

func NormalizeTicketStatus(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if ticketStatuses[s] {
		return s
	}
	return ""
}

func TicketStatusLabel(status string) string {
	switch NormalizeTicketStatus(status) {
	case "open":
		return "Open"
	case "in_progress":
		return "In progress"
	case "resolved":
		return "Resolved"
	case "closed":
		return "Closed"
	default:
		return status
	}
}

func (s *Store) ListTickets(ctx context.Context, tenantID xid.ID, status, search string, assignedTo *xid.ID, limit, offset int) ([]Ticket, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	where := "WHERE t.tenant_id = $1"
	args := []any{tenantID}
	if st := NormalizeTicketStatus(status); st != "" {
		args = append(args, st)
		where += fmt.Sprintf(" AND t.status = $%d", len(args))
	}
	if assignedTo != nil && !xid.IsNil(*assignedTo) {
		args = append(args, *assignedTo)
		where += fmt.Sprintf(" AND t.assigned_to = $%d", len(args))
	}
	if q := strings.TrimSpace(search); q != "" {
		args = append(args, "%"+q+"%")
		n := len(args)
		where += fmt.Sprintf(" AND (t.subject ILIKE $%d OR COALESCE(c.full_name,'') ILIKE $%d)", n, n)
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tickets t
		LEFT JOIN customers c ON c.id = t.customer_id
		`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit, offset)
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.tenant_id, t.customer_id, COALESCE(c.full_name,''), t.subject, t.description,
		       t.category, t.priority, t.status, t.assigned_to, COALESCE(u.full_name,''),
		       t.sla_due_at, t.resolved_at, t.created_at
		FROM tickets t
		LEFT JOIN customers c ON c.id = t.customer_id
		LEFT JOIN users u ON u.id = t.assigned_to
		`+where+` ORDER BY t.created_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.CustomerID, &t.CustomerName, &t.Subject, &t.Description,
			&t.Category, &t.Priority, &t.Status, &t.AssignedTo, &t.AssigneeName,
			&t.SLADueAt, &t.ResolvedAt, &t.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, t)
	}
	return list, total, rows.Err()
}

func (s *Store) GetTicket(ctx context.Context, tenantID, id xid.ID) (*Ticket, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	var t Ticket
	err := s.Pool.QueryRow(ctx, `
		SELECT t.id, t.tenant_id, t.customer_id, COALESCE(c.full_name,''), t.subject, t.description,
		       t.category, t.priority, t.status, t.assigned_to, COALESCE(u.full_name,''),
		       t.sla_due_at, t.resolved_at, t.created_at
		FROM tickets t
		LEFT JOIN customers c ON c.id = t.customer_id
		LEFT JOIN users u ON u.id = t.assigned_to
		WHERE t.tenant_id=$1 AND t.id=$2
	`, tenantID, id).Scan(
		&t.ID, &t.TenantID, &t.CustomerID, &t.CustomerName, &t.Subject, &t.Description,
		&t.Category, &t.Priority, &t.Status, &t.AssignedTo, &t.AssigneeName,
		&t.SLADueAt, &t.ResolvedAt, &t.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) CreateTicket(ctx context.Context, t *Ticket) error {
	if err := s.SetTenantContext(ctx, t.TenantID); err != nil {
		return err
	}
	if strings.TrimSpace(t.Subject) == "" {
		return fmt.Errorf("subjek wajib")
	}
	if t.Status == "" {
		t.Status = "open"
	}
	sla := time.Now().Add(24 * time.Hour)
	if t.Priority == "high" || t.Priority == "urgent" {
		sla = time.Now().Add(4 * time.Hour)
	}
	t.SLADueAt = &sla
	return s.Pool.QueryRow(ctx, `
		INSERT INTO tickets (tenant_id, customer_id, subject, description, category, priority, status, sla_due_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at
	`, t.TenantID, t.CustomerID, t.Subject, t.Description, t.Category, t.Priority, t.Status, sla).Scan(&t.ID, &t.CreatedAt)
}

func (s *Store) UpdateTicketStatus(ctx context.Context, tenantID xid.ID, id xid.ID, status string) error {
	status = NormalizeTicketStatus(status)
	if status == "" {
		return fmt.Errorf("status tiket tidak valid")
	}
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE tickets SET status=$3,
		                 resolved_at=CASE WHEN $3 IN ('resolved','closed') THEN COALESCE(resolved_at, NOW()) ELSE NULL END,
		                 updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AssignTicket(ctx context.Context, tenantID, id xid.ID, assignedTo *xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE tickets SET assigned_to=$3, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, assignedTo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListTicketMessages(ctx context.Context, tenantID, ticketID xid.ID) ([]TicketMessage, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT m.id, m.ticket_id, m.sender_type, m.sender_id, COALESCE(u.full_name,''), m.message,
		       COALESCE(m.image_urls, '[]'::jsonb), m.created_at
		FROM ticket_messages m
		LEFT JOIN users u ON u.id = m.sender_id
		WHERE m.tenant_id=$1 AND m.ticket_id=$2
		ORDER BY m.created_at ASC
	`, tenantID, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TicketMessage
	for rows.Next() {
		var m TicketMessage
		var raw []byte
		if err := rows.Scan(&m.ID, &m.TicketID, &m.SenderType, &m.SenderID, &m.SenderName, &m.Message, &raw, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.ImageURLs = []string{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &m.ImageURLs)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *Store) AddTicketMessage(ctx context.Context, tenantID, ticketID xid.ID, senderType string, senderID *xid.ID, message string, imageURLs []string) (*TicketMessage, error) {
	message = strings.TrimSpace(message)
	if imageURLs == nil {
		imageURLs = []string{}
	}
	if message == "" && len(imageURLs) == 0 {
		return nil, fmt.Errorf("pesan atau gambar wajib")
	}
	if senderType == "" {
		senderType = "staff"
	}
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(imageURLs)
	var m TicketMessage
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO ticket_messages (tenant_id, ticket_id, sender_type, sender_id, message, image_urls)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb)
		RETURNING id, ticket_id, sender_type, sender_id, message, COALESCE(image_urls, '[]'::jsonb), created_at
	`, tenantID, ticketID, senderType, senderID, message, raw).Scan(
		&m.ID, &m.TicketID, &m.SenderType, &m.SenderID, &m.Message, &raw, &m.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	m.ImageURLs = []string{}
	_ = json.Unmarshal(raw, &m.ImageURLs)
	if senderID != nil {
		_ = s.Pool.QueryRow(ctx, `SELECT COALESCE(full_name,'') FROM users WHERE id=$1`, *senderID).Scan(&m.SenderName)
	}
	return &m, nil
}

type Technician struct {
	ID        xid.ID    `json:"id"`
	TenantID  xid.ID    `json:"tenant_id"`
	UserID    *xid.ID   `json:"user_id,omitempty"`
	FullName  string    `json:"full_name"`
	Phone     string    `json:"phone"`
	Email     string    `json:"email,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// SyncTechniciansFromRoleUsers creates/updates technician rows for tenant users
// with role slug "teknisi" or permissions ops/tech (field staff).
func (s *Store) SyncTechniciansFromRoleUsers(ctx context.Context, tenantID xid.ID) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO technicians (tenant_id, user_id, full_name, phone, is_active)
		SELECT ut.tenant_id, u.id, u.full_name, COALESCE(NULLIF(TRIM(u.phone), ''), '-'), u.is_active
		FROM user_tenants ut
		JOIN users u ON u.id = ut.user_id
		JOIN roles r ON r.id = ut.role_id
		WHERE ut.tenant_id = $1
		  AND (
			r.slug = 'teknisi'
			OR r.permissions @> '["ops"]'::jsonb
			OR r.permissions @> '["tech"]'::jsonb
		  )
		  AND NOT (r.permissions @> '["*"]'::jsonb)
		  AND NOT EXISTS (
			SELECT 1 FROM technicians t
			WHERE t.tenant_id = ut.tenant_id AND t.user_id = u.id
		  )
	`, tenantID)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		UPDATE technicians AS t SET
			full_name = u.full_name,
			phone = COALESCE(NULLIF(TRIM(u.phone), ''), NULLIF(TRIM(t.phone), ''), '-')
		FROM users u
		JOIN user_tenants ut ON ut.user_id = u.id AND ut.tenant_id = $1
		JOIN roles r ON r.id = ut.role_id
		WHERE t.tenant_id = $1
		  AND t.user_id = u.id
		  AND (
			r.slug = 'teknisi'
			OR r.permissions @> '["ops"]'::jsonb
			OR r.permissions @> '["tech"]'::jsonb
		  )
		  AND NOT (r.permissions @> '["*"]'::jsonb)
	`, tenantID)
	return err
}

func (s *Store) ListTechnicians(ctx context.Context, tenantID xid.ID, activeOnly bool) ([]Technician, error) {
	// Sync is best-effort — never block the list if sync fails.
	_ = s.SyncTechniciansFromRoleUsers(ctx, tenantID)

	q := `
		SELECT t.id, t.tenant_id, t.user_id, t.full_name, t.phone, COALESCE(u.email,''), t.is_active, t.created_at
		FROM technicians t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE t.tenant_id=$1`
	if activeOnly {
		q += ` AND t.is_active=TRUE`
	}
	q += ` ORDER BY t.full_name`
	rows, err := s.Pool.Query(ctx, q, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Technician
	for rows.Next() {
		var t Technician
		if err := rows.Scan(&t.ID, &t.TenantID, &t.UserID, &t.FullName, &t.Phone, &t.Email, &t.IsActive, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) GetTechnicianByUserID(ctx context.Context, tenantID, userID xid.ID) (*Technician, error) {
	_ = s.SyncTechniciansFromRoleUsers(ctx, tenantID)
	var t Technician
	err := s.Pool.QueryRow(ctx, `
		SELECT t.id, t.tenant_id, t.user_id, t.full_name, t.phone, COALESCE(u.email,''), t.is_active, t.created_at
		FROM technicians t
		LEFT JOIN users u ON u.id = t.user_id
		WHERE t.tenant_id=$1 AND t.user_id=$2
		ORDER BY t.created_at ASC
		LIMIT 1
	`, tenantID, userID).Scan(&t.ID, &t.TenantID, &t.UserID, &t.FullName, &t.Phone, &t.Email, &t.IsActive, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) CreateTechnician(ctx context.Context, t *Technician) error {
	if err := s.SetTenantContext(ctx, t.TenantID); err != nil {
		return err
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO technicians (tenant_id, user_id, full_name, phone, is_active)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at
	`, t.TenantID, t.UserID, t.FullName, t.Phone, t.IsActive).Scan(&t.ID, &t.CreatedAt)
}

func (s *Store) UpdateTechnician(ctx context.Context, t *Technician) error {
	if err := s.SetTenantContext(ctx, t.TenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE technicians SET full_name=$3, phone=$4, user_id=$5, is_active=$6
		WHERE tenant_id=$1 AND id=$2
	`, t.TenantID, t.ID, t.FullName, t.Phone, t.UserID, t.IsActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type WorkOrder struct {
	ID              xid.ID     `json:"id"`
	TenantID        xid.ID     `json:"tenant_id"`
	CustomerID      *xid.ID    `json:"customer_id,omitempty"`
	CustomerName    string     `json:"customer_name,omitempty"`
	CustomerCode    string     `json:"customer_code,omitempty"`
	TechnicianID    *xid.ID    `json:"technician_id,omitempty"`
	TechnicianName  string     `json:"technician_name,omitempty"`
	Type            string     `json:"type"`
	Status          string     `json:"status"`
	ScheduledAt     *time.Time `json:"scheduled_at,omitempty"`
	CheckInAt       *time.Time `json:"check_in_at,omitempty"`
	CheckInLat      *float64   `json:"check_in_lat,omitempty"`
	CheckInLng      *float64   `json:"check_in_lng,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Notes           *string    `json:"notes,omitempty"`
	Photos          []string   `json:"photos"`
	CreatedAt       time.Time  `json:"created_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at,omitempty"`
}

var workOrderStatuses = map[string]bool{
	"pending": true, "assigned": true, "in_progress": true, "completed": true, "cancelled": true,
}

var workOrderTypes = map[string]bool{
	"installation": true, "repair": true, "survey": true, "maintenance": true, "other": true,
}

func NormalizeWorkOrderStatus(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if workOrderStatuses[s] {
		return s
	}
	return ""
}

func NormalizeWorkOrderType(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if workOrderTypes[s] {
		return s
	}
	return ""
}

func scanWorkOrder(scan func(dest ...any) error) (*WorkOrder, error) {
	var wo WorkOrder
	var photos []byte
	err := scan(
		&wo.ID, &wo.TenantID, &wo.CustomerID, &wo.CustomerName, &wo.CustomerCode,
		&wo.TechnicianID, &wo.TechnicianName, &wo.Type, &wo.Status, &wo.ScheduledAt,
		&wo.CheckInAt, &wo.CheckInLat, &wo.CheckInLng, &wo.CompletedAt, &wo.Notes, &photos,
		&wo.CreatedAt, &wo.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	wo.Photos = []string{}
	if len(photos) > 0 {
		_ = json.Unmarshal(photos, &wo.Photos)
	}
	return &wo, nil
}

const workOrderSelect = `
	SELECT wo.id, wo.tenant_id, wo.customer_id, COALESCE(c.full_name,''), COALESCE(c.customer_code,''),
	       wo.technician_id, COALESCE(tech.full_name,''), wo.type, wo.status, wo.scheduled_at,
	       wo.check_in_at, wo.check_in_lat, wo.check_in_lng, wo.completed_at, wo.notes,
	       COALESCE(wo.photos, '[]'::jsonb), wo.created_at, wo.updated_at
	FROM work_orders wo
	LEFT JOIN customers c ON c.id = wo.customer_id
	LEFT JOIN technicians tech ON tech.id = wo.technician_id
`

func (s *Store) ListWorkOrders(ctx context.Context, tenantID xid.ID, status string, technicianID *xid.ID, limit, offset int) ([]WorkOrder, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	where := "WHERE wo.tenant_id = $1"
	args := []any{tenantID}
	if st := NormalizeWorkOrderStatus(status); st != "" {
		args = append(args, st)
		where += fmt.Sprintf(" AND wo.status = $%d", len(args))
	}
	if technicianID != nil && !xid.IsNil(*technicianID) {
		args = append(args, *technicianID)
		where += fmt.Sprintf(" AND wo.technician_id = $%d", len(args))
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM work_orders wo "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit, offset)
	rows, err := s.Pool.Query(ctx, workOrderSelect+where+
		fmt.Sprintf(" ORDER BY wo.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []WorkOrder
	for rows.Next() {
		wo, err := scanWorkOrder(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *wo)
	}
	return list, total, rows.Err()
}

func (s *Store) GetWorkOrder(ctx context.Context, tenantID, id xid.ID) (*WorkOrder, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	row := s.Pool.QueryRow(ctx, workOrderSelect+` WHERE wo.tenant_id=$1 AND wo.id=$2`, tenantID, id)
	wo, err := scanWorkOrder(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return wo, err
}

func (s *Store) CreateWorkOrder(ctx context.Context, wo *WorkOrder) error {
	if err := s.SetTenantContext(ctx, wo.TenantID); err != nil {
		return err
	}
	if wo.Status == "" {
		wo.Status = "pending"
	}
	if wo.TechnicianID != nil && !xid.IsNil(*wo.TechnicianID) && wo.Status == "pending" {
		wo.Status = "assigned"
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO work_orders (tenant_id, customer_id, technician_id, type, status, scheduled_at, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at, updated_at
	`, wo.TenantID, wo.CustomerID, wo.TechnicianID, wo.Type, wo.Status, wo.ScheduledAt, wo.Notes).
		Scan(&wo.ID, &wo.CreatedAt, &wo.UpdatedAt)
}

func (s *Store) UpdateWorkOrder(ctx context.Context, tenantID, id xid.ID, technicianID **xid.ID, status, notes *string, scheduledAt **time.Time) (*WorkOrder, error) {
	cur, err := s.GetWorkOrder(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if technicianID != nil {
		cur.TechnicianID = *technicianID
		if cur.TechnicianID != nil && !xid.IsNil(*cur.TechnicianID) && (cur.Status == "pending" || cur.Status == "") {
			cur.Status = "assigned"
		}
	}
	if status != nil {
		if st := NormalizeWorkOrderStatus(*status); st != "" {
			cur.Status = st
		}
	}
	if notes != nil {
		n := strings.TrimSpace(*notes)
		if n == "" {
			cur.Notes = nil
		} else {
			cur.Notes = &n
		}
	}
	if scheduledAt != nil {
		cur.ScheduledAt = *scheduledAt
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE work_orders SET technician_id=$3, status=$4, notes=$5, scheduled_at=$6, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, cur.TechnicianID, cur.Status, cur.Notes, cur.ScheduledAt)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetWorkOrder(ctx, tenantID, id)
}

func (s *Store) CheckInWorkOrder(ctx context.Context, tenantID xid.ID, id xid.ID, lat, lng float64) (*WorkOrder, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE work_orders SET check_in_at=NOW(), check_in_lat=$3, check_in_lng=$4, status='in_progress', updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2 AND status NOT IN ('completed','cancelled')
	`, tenantID, id, lat, lng)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetWorkOrder(ctx, tenantID, id)
}

func (s *Store) CompleteWorkOrder(ctx context.Context, tenantID xid.ID, id xid.ID, notes *string) (*WorkOrder, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	cur, err := s.GetWorkOrder(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if cur.Status == "cancelled" {
		return nil, fmt.Errorf("work order sudah dibatalkan")
	}
	mergedNotes := cur.Notes
	if notes != nil {
		n := strings.TrimSpace(*notes)
		if n != "" {
			if mergedNotes != nil && *mergedNotes != "" {
				combined := *mergedNotes + "\n---\n" + n
				mergedNotes = &combined
			} else {
				mergedNotes = &n
			}
		}
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE work_orders SET status='completed', completed_at=NOW(), notes=$3, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2 AND status <> 'cancelled'
	`, tenantID, id, mergedNotes)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetWorkOrder(ctx, tenantID, id)
}

func (s *Store) AppendWorkOrderPhoto(ctx context.Context, tenantID, id xid.ID, url string) (*WorkOrder, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	b, _ := json.Marshal([]string{url})
	tag, err := s.Pool.Exec(ctx, `
		UPDATE work_orders SET photos = COALESCE(photos, '[]'::jsonb) || $3::jsonb, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2 AND status NOT IN ('cancelled')
	`, tenantID, id, b)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetWorkOrder(ctx, tenantID, id)
}

type VoucherBatch struct {
	ID          xid.ID     `json:"id"`
	TenantID    xid.ID     `json:"tenant_id"`
	PlanID      *xid.ID    `json:"plan_id,omitempty"`
	PlanName    string     `json:"plan_name,omitempty"`
	RouterID    *xid.ID    `json:"router_id,omitempty"`
	RouterName  string     `json:"router_name,omitempty"`
	Name        string     `json:"name"`
	Quantity    int        `json:"quantity"`
	Price       int64      `json:"price"`
	Available   int        `json:"available"`
	Used        int        `json:"used"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	SyncStatus  string     `json:"sync_status,omitempty"`
	SyncError   string     `json:"sync_error,omitempty"`
	SyncedCount int        `json:"synced_count,omitempty"`
	SyncedAt    *time.Time `json:"synced_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at,omitempty"`
}

type Voucher struct {
	ID        xid.ID     `json:"id"`
	BatchID   xid.ID     `json:"batch_id"`
	Code      string     `json:"code"`
	Status    string     `json:"status"`
	UsedBy    *xid.ID    `json:"used_by,omitempty"`
	UsedName  string     `json:"used_by_name,omitempty"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at,omitempty"`
}

func generateVoucherCodes(prefix string, n int) []string {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if prefix == "" {
		prefix = "VCH"
	}
	out := make([]string, 0, n)
	seen := map[string]bool{}
	for len(out) < n {
		code := prefix + randomVoucherSuffix(8)
		if seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out
}

func randomVoucherSuffix(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			b[i] = alphabet[i%len(alphabet)]
			continue
		}
		b[i] = alphabet[v.Int64()]
	}
	return string(b)
}

func (s *Store) CreateVoucherBatchWithPrefix(ctx context.Context, b *VoucherBatch, prefix string) error {
	qty := b.Quantity
	if qty <= 0 {
		qty = 10
	}
	codes := generateVoucherCodes(prefix, qty)
	return s.CreateVoucherBatch(ctx, b, codes)
}

func (s *Store) CreateVoucherBatch(ctx context.Context, b *VoucherBatch, codes []string) error {
	if strings.TrimSpace(b.Name) == "" {
		return fmt.Errorf("nama batch wajib")
	}
	if len(codes) == 0 {
		qty := b.Quantity
		if qty <= 0 {
			qty = 10
		}
		if qty > 5000 {
			return fmt.Errorf("maksimal 5000 kode per batch")
		}
		codes = generateVoucherCodes("VCH", qty)
	}
	if len(codes) > 5000 {
		return fmt.Errorf("maksimal 5000 kode per batch")
	}
	b.Quantity = len(codes)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	syncStatus := strings.TrimSpace(b.SyncStatus)
	if syncStatus == "" {
		if b.RouterID != nil {
			syncStatus = "pending"
		} else {
			syncStatus = "none"
		}
	}
	b.SyncStatus = syncStatus
	err = tx.QueryRow(ctx, `
		INSERT INTO voucher_batches (tenant_id, plan_id, router_id, name, quantity, price, expires_at, sync_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at
	`, b.TenantID, b.PlanID, b.RouterID, b.Name, b.Quantity, b.Price, b.ExpiresAt, syncStatus).Scan(&b.ID, &b.CreatedAt)
	if err != nil {
		return err
	}
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO vouchers (tenant_id, batch_id, code, expires_at) VALUES ($1,$2,$3,$4)
		`, b.TenantID, b.ID, code, b.ExpiresAt)
		if err != nil {
			return err
		}
	}
	b.Available = b.Quantity
	b.Used = 0
	return tx.Commit(ctx)
}

func scanVoucherBatch(
	id, tenantID xid.ID,
	planID *xid.ID, planName string,
	routerID *xid.ID, routerName string,
	name string, quantity int, price int64,
	expiresAt *time.Time, createdAt time.Time,
	available, used int,
	syncStatus, syncError string,
	syncedCount int, syncedAt *time.Time,
) VoucherBatch {
	return VoucherBatch{
		ID: id, TenantID: tenantID, PlanID: planID, PlanName: planName,
		RouterID: routerID, RouterName: routerName, Name: name,
		Quantity: quantity, Price: price, ExpiresAt: expiresAt, CreatedAt: createdAt,
		Available: available, Used: used,
		SyncStatus: syncStatus, SyncError: syncError, SyncedCount: syncedCount, SyncedAt: syncedAt,
	}
}

func (s *Store) ListVoucherBatches(ctx context.Context, tenantID xid.ID) ([]VoucherBatch, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT b.id, b.tenant_id, b.plan_id, COALESCE(p.name,''), b.router_id, COALESCE(r.name,''),
		       b.name, b.quantity, b.price, b.expires_at, b.created_at,
		       COALESCE((SELECT COUNT(*) FROM vouchers v WHERE v.batch_id=b.id AND v.status='available'),0),
		       COALESCE((SELECT COUNT(*) FROM vouchers v WHERE v.batch_id=b.id AND v.status='used'),0),
		       COALESCE(b.sync_status,'none'), COALESCE(b.sync_error,''), COALESCE(b.synced_count,0), b.synced_at
		FROM voucher_batches b
		LEFT JOIN plans p ON p.id = b.plan_id
		LEFT JOIN routers r ON r.id = b.router_id
		WHERE b.tenant_id=$1
		ORDER BY b.created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []VoucherBatch
	for rows.Next() {
		var (
			id, tid                                           xid.ID
			planID, routerID                                  *xid.ID
			planName, routerName, name, syncStatus, syncError string
			quantity, available, used, syncedCount            int
			price                                             int64
			expiresAt, syncedAt                               *time.Time
			created                                           time.Time
		)
		if err := rows.Scan(
			&id, &tid, &planID, &planName, &routerID, &routerName,
			&name, &quantity, &price, &expiresAt, &created,
			&available, &used, &syncStatus, &syncError, &syncedCount, &syncedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, scanVoucherBatch(
			id, tid, planID, planName, routerID, routerName, name, quantity, price,
			expiresAt, created, available, used, syncStatus, syncError, syncedCount, syncedAt,
		))
	}
	return list, rows.Err()
}

func (s *Store) GetVoucherBatch(ctx context.Context, tenantID, id xid.ID) (*VoucherBatch, error) {
	var (
		planID, routerID                                  *xid.ID
		planName, routerName, name, syncStatus, syncError string
		quantity, available, used, syncedCount            int
		price                                             int64
		expiresAt, syncedAt                               *time.Time
		created                                           time.Time
		tid, bid                                          xid.ID
	)
	err := s.Pool.QueryRow(ctx, `
		SELECT b.id, b.tenant_id, b.plan_id, COALESCE(p.name,''), b.router_id, COALESCE(r.name,''),
		       b.name, b.quantity, b.price, b.expires_at, b.created_at,
		       COALESCE((SELECT COUNT(*) FROM vouchers v WHERE v.batch_id=b.id AND v.status='available'),0),
		       COALESCE((SELECT COUNT(*) FROM vouchers v WHERE v.batch_id=b.id AND v.status='used'),0),
		       COALESCE(b.sync_status,'none'), COALESCE(b.sync_error,''), COALESCE(b.synced_count,0), b.synced_at
		FROM voucher_batches b
		LEFT JOIN plans p ON p.id = b.plan_id
		LEFT JOIN routers r ON r.id = b.router_id
		WHERE b.tenant_id=$1 AND b.id=$2
	`, tenantID, id).Scan(
		&bid, &tid, &planID, &planName, &routerID, &routerName,
		&name, &quantity, &price, &expiresAt, &created,
		&available, &used, &syncStatus, &syncError, &syncedCount, &syncedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	b := scanVoucherBatch(
		bid, tid, planID, planName, routerID, routerName, name, quantity, price,
		expiresAt, created, available, used, syncStatus, syncError, syncedCount, syncedAt,
	)
	return &b, nil
}

func (s *Store) UpdateVoucherBatchSync(ctx context.Context, tenantID, id xid.ID, status string, syncedCount int, syncErr string) error {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "none"
	}
	var errPtr *string
	if strings.TrimSpace(syncErr) != "" {
		e := strings.TrimSpace(syncErr)
		errPtr = &e
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE voucher_batches
		SET sync_status=$3, synced_count=$4, sync_error=$5,
		    synced_at=CASE WHEN $3 IN ('synced','partial','failed') THEN NOW() ELSE synced_at END
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, status, syncedCount, errPtr)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListVouchersByBatch(ctx context.Context, tenantID, batchID xid.ID, status string) ([]Voucher, error) {
	if _, err := s.GetVoucherBatch(ctx, tenantID, batchID); err != nil {
		return nil, err
	}
	where := `WHERE v.tenant_id=$1 AND v.batch_id=$2`
	args := []any{tenantID, batchID}
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "available" || status == "used" {
		args = append(args, status)
		where += fmt.Sprintf(` AND v.status=$%d`, len(args))
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT v.id, v.batch_id, v.code, v.status, v.used_by, COALESCE(c.full_name,''), v.used_at, v.expires_at, v.created_at
		FROM vouchers v
		LEFT JOIN customers c ON c.id = v.used_by
		`+where+` ORDER BY v.id
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Voucher
	for rows.Next() {
		var v Voucher
		if err := rows.Scan(&v.ID, &v.BatchID, &v.Code, &v.Status, &v.UsedBy, &v.UsedName, &v.UsedAt, &v.ExpiresAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func (s *Store) DeleteVoucherBatch(ctx context.Context, tenantID, id xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM voucher_batches WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RedeemVoucher(ctx context.Context, tenantID xid.ID, code string, customerID xid.ID) error {
	result, err := s.Pool.Exec(ctx, `
		UPDATE vouchers SET status='used', used_by=$3, used_at=NOW()
		WHERE tenant_id=$1 AND code=$2 AND status='available'
		  AND (expires_at IS NULL OR expires_at > NOW())
	`, tenantID, code, customerID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type ODP struct {
	ID        xid.ID   `json:"id"`
	TenantID  xid.ID   `json:"tenant_id"`
	ClusterID *xid.ID  `json:"cluster_id,omitempty"`
	ODCID     *xid.ID  `json:"odc_id,omitempty"`
	Name      string   `json:"name"`
	Code      string   `json:"code"`
	Address   *string  `json:"address,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	PortCount int      `json:"port_count"`
	UsedPorts int      `json:"used_ports"`
	FreePorts int      `json:"free_ports"`
}

type ODPPort struct {
	ID               xid.ID  `json:"id"`
	TenantID         xid.ID  `json:"tenant_id"`
	ODPID            xid.ID  `json:"odp_id"`
	PortNumber       int     `json:"port_number"`
	Status           string  `json:"status"`
	CustomerID       *xid.ID `json:"customer_id,omitempty"`
	SubscriptionID   *xid.ID `json:"subscription_id,omitempty"`
	CustomerName     string  `json:"customer_name,omitempty"`
	SubscriptionUser string  `json:"subscription_username,omitempty"`
}

func (s *Store) ListODPs(ctx context.Context, tenantID xid.ID) ([]ODP, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT o.id, o.tenant_id, o.cluster_id, o.odc_id, o.name, o.code, o.address, o.latitude, o.longitude, o.port_count,
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'used'), 0),
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available'), 0)
		FROM odps o WHERE o.tenant_id = $1 ORDER BY o.name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ODP
	for rows.Next() {
		var o ODP
		if err := rows.Scan(&o.ID, &o.TenantID, &o.ClusterID, &o.ODCID, &o.Name, &o.Code, &o.Address, &o.Latitude, &o.Longitude, &o.PortCount, &o.UsedPorts, &o.FreePorts); err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

func (s *Store) GetODP(ctx context.Context, tenantID, id xid.ID) (*ODP, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT o.id, o.tenant_id, o.cluster_id, o.odc_id, o.name, o.code, o.address, o.latitude, o.longitude, o.port_count,
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'used'), 0),
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available'), 0)
		FROM odps o WHERE o.tenant_id = $1 AND o.id = $2
	`, tenantID, id)
	var o ODP
	err := row.Scan(&o.ID, &o.TenantID, &o.ClusterID, &o.ODCID, &o.Name, &o.Code, &o.Address, &o.Latitude, &o.Longitude, &o.PortCount, &o.UsedPorts, &o.FreePorts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Store) CreateODP(ctx context.Context, o *ODP) error {
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO odps (tenant_id, cluster_id, odc_id, name, code, address, latitude, longitude, port_count)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id
	`, o.TenantID, o.ClusterID, o.ODCID, o.Name, o.Code, o.Address, o.Latitude, o.Longitude, o.PortCount).Scan(&o.ID)
	if err != nil {
		return err
	}
	for i := 1; i <= o.PortCount; i++ {
		_, err = s.Pool.Exec(ctx, `
			INSERT INTO odp_ports (tenant_id, odp_id, port_number, status) VALUES ($1,$2,$3,'available')
		`, o.TenantID, o.ID, i)
		if err != nil {
			return err
		}
	}
	o.FreePorts = o.PortCount
	o.UsedPorts = 0
	return nil
}

func (s *Store) UpdateODP(ctx context.Context, o *ODP) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE odps SET name=$3, code=$4, address=$5, latitude=$6, longitude=$7, odc_id=$8, cluster_id=$9
		WHERE tenant_id=$1 AND id=$2
	`, o.TenantID, o.ID, o.Name, o.Code, o.Address, o.Latitude, o.Longitude, o.ODCID, o.ClusterID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetODPByCode(ctx context.Context, tenantID xid.ID, code string) (*ODP, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT o.id, o.tenant_id, o.cluster_id, o.odc_id, o.name, o.code, o.address, o.latitude, o.longitude, o.port_count,
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'used'), 0),
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available'), 0)
		FROM odps o WHERE o.tenant_id = $1 AND o.code = $2
	`, tenantID, code)
	var o ODP
	err := row.Scan(&o.ID, &o.TenantID, &o.ClusterID, &o.ODCID, &o.Name, &o.Code, &o.Address, &o.Latitude, &o.Longitude, &o.PortCount, &o.UsedPorts, &o.FreePorts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Store) DeleteODP(ctx context.Context, tenantID, id xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM odps WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListODPPorts(ctx context.Context, tenantID, odpID xid.ID) ([]ODPPort, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id, p.tenant_id, p.odp_id, p.port_number, p.status, p.customer_id, p.subscription_id,
		       COALESCE(c.full_name, ''), COALESCE(sub.username, '')
		FROM odp_ports p
		LEFT JOIN customers c ON c.id = p.customer_id
		LEFT JOIN subscriptions sub ON sub.id = p.subscription_id
		WHERE p.tenant_id = $1 AND p.odp_id = $2
		ORDER BY p.port_number
	`, tenantID, odpID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ODPPort
	for rows.Next() {
		var p ODPPort
		if err := rows.Scan(&p.ID, &p.TenantID, &p.ODPID, &p.PortNumber, &p.Status, &p.CustomerID, &p.SubscriptionID, &p.CustomerName, &p.SubscriptionUser); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Store) AssignODPPort(ctx context.Context, tenantID xid.ID, odpID xid.ID, portNum int, customerID *xid.ID, subscriptionID *xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE odp_ports SET status='used', customer_id=$4, subscription_id=$5
		WHERE tenant_id=$1 AND odp_id=$2 AND port_number=$3 AND status='available'
	`, tenantID, odpID, portNum, customerID, subscriptionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignNextAvailableODPPort reserves the lowest available port on an ODP.
func (s *Store) AssignNextAvailableODPPort(ctx context.Context, tenantID, odpID, customerID, subscriptionID xid.ID) (*ODPPort, error) {
	row := s.Pool.QueryRow(ctx, `
		WITH next AS (
			SELECT id FROM odp_ports
			WHERE tenant_id = $1 AND odp_id = $2 AND status = 'available'
			ORDER BY port_number
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE odp_ports p
		SET status = 'used', customer_id = $3, subscription_id = $4
		FROM next
		WHERE p.id = next.id
		RETURNING p.id, p.tenant_id, p.odp_id, p.port_number, p.status, p.customer_id, p.subscription_id
	`, tenantID, odpID, customerID, subscriptionID)
	var p ODPPort
	err := row.Scan(&p.ID, &p.TenantID, &p.ODPID, &p.PortNumber, &p.Status, &p.CustomerID, &p.SubscriptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ReleaseODPPortBySubscription(ctx context.Context, tenantID, subscriptionID xid.ID) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE odp_ports
		SET status = 'available', customer_id = NULL, subscription_id = NULL
		WHERE tenant_id = $1 AND subscription_id = $2
	`, tenantID, subscriptionID)
	return err
}

func (s *Store) FindNearestAvailableODP(ctx context.Context, tenantID xid.ID, lat, lng float64) (*ODP, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT o.id, o.tenant_id, o.cluster_id, o.odc_id, o.name, o.code, o.address, o.latitude, o.longitude, o.port_count,
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'used'), 0),
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available'), 0)
		FROM odps o
		WHERE o.tenant_id = $1
		  AND o.latitude IS NOT NULL AND o.longitude IS NOT NULL
		  AND EXISTS (SELECT 1 FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available')
		ORDER BY ((o.latitude - $2)^2 + (o.longitude - $3)^2) ASC
		LIMIT 1
	`, tenantID, lat, lng)
	var o ODP
	err := row.Scan(&o.ID, &o.TenantID, &o.ClusterID, &o.ODCID, &o.Name, &o.Code, &o.Address, &o.Latitude, &o.Longitude, &o.PortCount, &o.UsedPorts, &o.FreePorts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

type JournalEntry struct {
	ID          xid.ID        `json:"id"`
	TenantID    xid.ID        `json:"tenant_id"`
	EntryDate   string        `json:"entry_date"`
	Description string        `json:"description"`
	Lines       []JournalLine `json:"lines"`
}

type JournalLine struct {
	AccountID xid.ID `json:"account_id"`
	Debit     int64  `json:"debit"`
	Credit    int64  `json:"credit"`
}

func (s *Store) PostJournalEntry(ctx context.Context, e *JournalEntry) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (tenant_id, entry_date, description) VALUES ($1,$2,$3) RETURNING id
	`, e.TenantID, e.EntryDate, e.Description).Scan(&e.ID)
	if err != nil {
		return err
	}
	for _, line := range e.Lines {
		_, err = tx.Exec(ctx, `
			INSERT INTO journal_lines (tenant_id, entry_id, account_id, debit, credit) VALUES ($1,$2,$3,$4,$5)
		`, e.TenantID, e.ID, line.AccountID, line.Debit, line.Credit)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordPaymentJournal(ctx context.Context, tenantID xid.ID, amount int64, cashAccountID xid.ID, revenueAccountID xid.ID, ref string) error {
	return s.PostJournalEntry(ctx, &JournalEntry{
		TenantID:    tenantID,
		EntryDate:   time.Now().Format("2006-01-02"),
		Description: "Pembayaran " + ref,
		Lines: []JournalLine{
			{AccountID: cashAccountID, Debit: amount},
			{AccountID: revenueAccountID, Credit: amount},
		},
	})
}

func (s *Store) DashboardStats(ctx context.Context, tenantID xid.ID) (map[string]any, error) {
	stats := make(map[string]any)
	var activeCustomers, activeSubs, unpaidInvoices, monthlyRevenue int64
	_ = s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM customers WHERE tenant_id=$1 AND is_active=true`, tenantID).Scan(&activeCustomers)
	_ = s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM subscriptions WHERE tenant_id=$1 AND status='active'`, tenantID).Scan(&activeSubs)
	_ = s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM invoices WHERE tenant_id=$1 AND status IN ('issued','partial','overdue')`, tenantID).Scan(&unpaidInvoices)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount),0) FROM payments WHERE tenant_id=$1 AND status='paid'
		AND paid_at >= date_trunc('month', NOW())
	`, tenantID).Scan(&monthlyRevenue)
	stats["mode"] = "admin"
	stats["active_customers"] = activeCustomers
	stats["active_subscriptions"] = activeSubs
	stats["unpaid_invoices"] = unpaidInvoices
	stats["monthly_revenue"] = monthlyRevenue
	return stats, nil
}

// FieldOpsDashboardStats returns lead/ticket counts for the assigned technician user.
func (s *Store) FieldOpsDashboardStats(ctx context.Context, tenantID, userID xid.ID) (map[string]any, error) {
	stats := make(map[string]any)
	var leadsAssigned, leadsContacted, leadsSurvey, leadsInstall int64
	var ticketsOpen, ticketsProgress, ticketsResolved, ticketsTotal int64
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM leads
		WHERE tenant_id=$1 AND assigned_to=$2 AND status IN ('contacted','survey','qualified')
	`, tenantID, userID).Scan(&leadsAssigned)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM leads WHERE tenant_id=$1 AND assigned_to=$2 AND status='contacted'
	`, tenantID, userID).Scan(&leadsContacted)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM leads WHERE tenant_id=$1 AND assigned_to=$2 AND status='survey'
	`, tenantID, userID).Scan(&leadsSurvey)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM leads WHERE tenant_id=$1 AND assigned_to=$2 AND status='qualified'
	`, tenantID, userID).Scan(&leadsInstall)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tickets WHERE tenant_id=$1 AND assigned_to=$2 AND status='open'
	`, tenantID, userID).Scan(&ticketsOpen)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tickets WHERE tenant_id=$1 AND assigned_to=$2 AND status='in_progress'
	`, tenantID, userID).Scan(&ticketsProgress)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tickets WHERE tenant_id=$1 AND assigned_to=$2 AND status='resolved'
	`, tenantID, userID).Scan(&ticketsResolved)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tickets WHERE tenant_id=$1 AND assigned_to=$2 AND status IN ('open','in_progress','resolved')
	`, tenantID, userID).Scan(&ticketsTotal)
	stats["mode"] = "field_ops"
	stats["leads_assigned"] = leadsAssigned
	stats["leads_contacted"] = leadsContacted
	stats["leads_survey"] = leadsSurvey
	stats["leads_install"] = leadsInstall
	stats["tickets_open"] = ticketsOpen
	stats["tickets_in_progress"] = ticketsProgress
	stats["tickets_resolved"] = ticketsResolved
	stats["tickets_active"] = ticketsTotal
	return stats, nil
}

// TicketSLASample is one ticket row for SLA evaluation tables.
type TicketSLASample struct {
	ID           xid.ID     `json:"id"`
	Subject      string     `json:"subject"`
	Category     string     `json:"category"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	CustomerName string     `json:"customer_name,omitempty"`
	AssigneeName string     `json:"assignee_name,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	SLADueAt     *time.Time `json:"sla_due_at,omitempty"`
	ResolveHours *float64   `json:"resolve_hours,omitempty"`
	SLAMet       *bool      `json:"sla_met,omitempty"`
	Breached     bool       `json:"breached"`
}

// TicketSLAReport aggregates ticket/outage SLA metrics for management.
type TicketSLAReport struct {
	Days                 int                `json:"days"`
	From                 time.Time          `json:"from"`
	To                   time.Time          `json:"to"`
	Total                int64              `json:"total"`
	OpenCount            int64              `json:"open_count"`
	InProgressCount      int64              `json:"in_progress_count"`
	ResolvedCount        int64              `json:"resolved_count"`
	ClosedCount          int64              `json:"closed_count"`
	OutageCount          int64              `json:"outage_count"`
	OutagePct            float64            `json:"outage_pct"`
	ByCategory           map[string]int64   `json:"by_category"`
	ByPriority           map[string]int64   `json:"by_priority"`
	ByStatus             map[string]int64   `json:"by_status"`
	SLAApplicable        int64              `json:"sla_applicable"`
	SLAMet               int64              `json:"sla_met"`
	SLABreached          int64              `json:"sla_breached"`
	SLAMetPct            float64            `json:"sla_met_pct"`
	OpenBreaching        int64              `json:"open_breaching"`
	AvgResolveHours      float64            `json:"avg_resolve_hours"`
	MedianResolveHours   float64            `json:"median_resolve_hours"`
	P95ResolveHours      float64            `json:"p95_resolve_hours"`
	AvgByCategoryHours   map[string]float64 `json:"avg_by_category_hours"`
	AvgByPriorityHours   map[string]float64 `json:"avg_by_priority_hours"`
	SlowestResolved      []TicketSLASample  `json:"slowest_resolved"`
	OpenBreachingList    []TicketSLASample  `json:"open_breaching_list"`
}

func pct(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) * 100 / float64(total)
}

func percentileSorted(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[n-1]
	}
	idx := int((p / 100) * float64(n-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

func (s *Store) TicketSLAReport(ctx context.Context, tenantID xid.ID, days int) (*TicketSLAReport, error) {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	to := time.Now()
	from := to.AddDate(0, 0, -days)
	rep := &TicketSLAReport{
		Days:               days,
		From:               from,
		To:                 to,
		ByCategory:         map[string]int64{},
		ByPriority:         map[string]int64{},
		ByStatus:           map[string]int64{},
		AvgByCategoryHours: map[string]float64{},
		AvgByPriorityHours: map[string]float64{},
		SlowestResolved:    []TicketSLASample{},
		OpenBreachingList:  []TicketSLASample{},
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.subject, t.category, t.priority, t.status,
		       COALESCE(c.full_name,''), COALESCE(u.full_name,''),
		       t.created_at, t.resolved_at, t.sla_due_at
		FROM tickets t
		LEFT JOIN customers c ON c.id = t.customer_id
		LEFT JOIN users u ON u.id = t.assigned_to
		WHERE t.tenant_id=$1 AND t.created_at >= $2 AND t.created_at <= $3
		ORDER BY t.created_at DESC
	`, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []TicketSLASample
	for rows.Next() {
		var sample TicketSLASample
		if err := rows.Scan(
			&sample.ID, &sample.Subject, &sample.Category, &sample.Priority, &sample.Status,
			&sample.CustomerName, &sample.AssigneeName,
			&sample.CreatedAt, &sample.ResolvedAt, &sample.SLADueAt,
		); err != nil {
			return nil, err
		}
		all = append(all, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var resolveHours []float64
	catHours := map[string][]float64{}
	prioHours := map[string][]float64{}
	var resolvedSamples []TicketSLASample

	for i := range all {
		sample := all[i]
		rep.Total++
		rep.ByCategory[sample.Category]++
		rep.ByPriority[sample.Priority]++
		rep.ByStatus[sample.Status]++
		if sample.Category == "outage" {
			rep.OutageCount++
		}
		switch sample.Status {
		case "open":
			rep.OpenCount++
		case "in_progress":
			rep.InProgressCount++
		case "resolved":
			rep.ResolvedCount++
		case "closed":
			rep.ClosedCount++
		}

		if sample.ResolvedAt != nil {
			h := sample.ResolvedAt.Sub(sample.CreatedAt).Hours()
			if h < 0 {
				h = 0
			}
			sample.ResolveHours = &h
			resolveHours = append(resolveHours, h)
			catHours[sample.Category] = append(catHours[sample.Category], h)
			prioHours[sample.Priority] = append(prioHours[sample.Priority], h)

			if sample.SLADueAt != nil {
				rep.SLAApplicable++
				met := !sample.ResolvedAt.After(*sample.SLADueAt)
				sample.SLAMet = &met
				if met {
					rep.SLAMet++
				} else {
					rep.SLABreached++
					sample.Breached = true
				}
			}
			resolvedSamples = append(resolvedSamples, sample)
		} else if (sample.Status == "open" || sample.Status == "in_progress") && sample.SLADueAt != nil && time.Now().After(*sample.SLADueAt) {
			rep.SLAApplicable++
			rep.SLABreached++
			rep.OpenBreaching++
			sample.Breached = true
			h := time.Since(sample.CreatedAt).Hours()
			sample.ResolveHours = &h
			rep.OpenBreachingList = append(rep.OpenBreachingList, sample)
		}
	}

	rep.OutagePct = pct(rep.OutageCount, rep.Total)
	rep.SLAMetPct = pct(rep.SLAMet, rep.SLAApplicable)

	if len(resolveHours) > 0 {
		sum := 0.0
		for _, h := range resolveHours {
			sum += h
		}
		rep.AvgResolveHours = sum / float64(len(resolveHours))
		sorted := append([]float64(nil), resolveHours...)
		sort.Float64s(sorted)
		rep.MedianResolveHours = percentileSorted(sorted, 50)
		rep.P95ResolveHours = percentileSorted(sorted, 95)
	}

	avgMap := func(src map[string][]float64) map[string]float64 {
		out := map[string]float64{}
		for k, hs := range src {
			if len(hs) == 0 {
				continue
			}
			sum := 0.0
			for _, h := range hs {
				sum += h
			}
			out[k] = sum / float64(len(hs))
		}
		return out
	}
	rep.AvgByCategoryHours = avgMap(catHours)
	rep.AvgByPriorityHours = avgMap(prioHours)

	sort.Slice(resolvedSamples, func(i, j int) bool {
		hi, hj := 0.0, 0.0
		if resolvedSamples[i].ResolveHours != nil {
			hi = *resolvedSamples[i].ResolveHours
		}
		if resolvedSamples[j].ResolveHours != nil {
			hj = *resolvedSamples[j].ResolveHours
		}
		return hj < hi
	})
	limit := 10
	if len(resolvedSamples) < limit {
		limit = len(resolvedSamples)
	}
	rep.SlowestResolved = resolvedSamples[:limit]

	sort.Slice(rep.OpenBreachingList, func(i, j int) bool {
		ai, aj := rep.OpenBreachingList[i].SLADueAt, rep.OpenBreachingList[j].SLADueAt
		if ai == nil {
			return false
		}
		if aj == nil {
			return true
		}
		return ai.Before(*aj)
	})
	if len(rep.OpenBreachingList) > 20 {
		rep.OpenBreachingList = rep.OpenBreachingList[:20]
	}
	return rep, nil
}

func (s *Store) RevenueChart(ctx context.Context, tenantID xid.ID, months int) ([]map[string]any, error) {
	if months <= 0 {
		months = 6
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT to_char(date_trunc('month', paid_at), 'YYYY-MM') as month, COALESCE(SUM(amount),0)
		FROM payments
		WHERE tenant_id=$1 AND status='paid'
		  AND paid_at >= NOW() - ($2 * INTERVAL '1 month')
		GROUP BY 1 ORDER BY 1
	`, tenantID, months)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var month string
		var total int64
		if err := rows.Scan(&month, &total); err != nil {
			return nil, err
		}
		list = append(list, map[string]any{"month": month, "revenue": total})
	}
	return list, rows.Err()
}

func (s *Store) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, slug, name, email, phone, is_active, created_at FROM tenants ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Email, &t.Phone, &t.IsActive, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}
