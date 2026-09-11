package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// Document kinds shared by lead & customer galleries.
var DocumentKinds = map[string]bool{
	"ktp": true, "rumah": true, "odp": true, "psb": true, "other": true,
}

func NormalizeDocumentKind(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	if DocumentKinds[kind] {
		return kind
	}
	return "other"
}

func DocumentKindLabel(kind string) string {
	switch NormalizeDocumentKind(kind) {
	case "ktp":
		return "KTP / identitas"
	case "rumah":
		return "Rumah / lokasi"
	case "odp":
		return "ODP"
	case "psb":
		return "Proses PSB"
	default:
		return "Lainnya"
	}
}

type LeadDocument struct {
	ID           xid.ID    `json:"id"`
	LeadID       xid.ID    `json:"lead_id"`
	Kind         string    `json:"kind"`
	URL          string    `json:"url"`
	Caption      string    `json:"caption"`
	UploadedBy   *xid.ID   `json:"uploaded_by,omitempty"`
	UploaderName string    `json:"uploader_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type CustomerDocument struct {
	ID           xid.ID    `json:"id"`
	CustomerID   xid.ID    `json:"customer_id"`
	Kind         string    `json:"kind"`
	URL          string    `json:"url"`
	Caption      string    `json:"caption"`
	Source       string    `json:"source"`
	SourceRef    *xid.ID   `json:"source_ref,omitempty"`
	UploadedBy   *xid.ID   `json:"uploaded_by,omitempty"`
	UploaderName string    `json:"uploader_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) ListLeadDocuments(ctx context.Context, tenantID, leadID xid.ID) ([]LeadDocument, error) {
	if _, err := s.GetLead(ctx, tenantID, leadID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT d.id, d.lead_id, d.kind, d.url, d.caption, d.uploaded_by,
		       COALESCE(u.full_name, ''), d.created_at
		FROM lead_documents d
		LEFT JOIN users u ON u.id = d.uploaded_by
		WHERE d.tenant_id=$1 AND d.lead_id=$2
		ORDER BY d.created_at ASC
	`, tenantID, leadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []LeadDocument
	for rows.Next() {
		var d LeadDocument
		if err := rows.Scan(&d.ID, &d.LeadID, &d.Kind, &d.URL, &d.Caption, &d.UploadedBy, &d.UploaderName, &d.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

func (s *Store) AddLeadDocument(ctx context.Context, tenantID, leadID xid.ID, uploadedBy *xid.ID, kind, url, caption string) (*LeadDocument, error) {
	if _, err := s.GetLead(ctx, tenantID, leadID); err != nil {
		return nil, err
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, fmt.Errorf("url dokumen wajib")
	}
	kind = NormalizeDocumentKind(kind)
	caption = strings.TrimSpace(caption)
	var d LeadDocument
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO lead_documents (tenant_id, lead_id, kind, url, caption, uploaded_by)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, lead_id, kind, url, caption, uploaded_by, created_at
	`, tenantID, leadID, kind, url, caption, uploadedBy).Scan(
		&d.ID, &d.LeadID, &d.Kind, &d.URL, &d.Caption, &d.UploadedBy, &d.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if uploadedBy != nil {
		_ = s.Pool.QueryRow(ctx, `SELECT COALESCE(full_name,'') FROM users WHERE id=$1`, *uploadedBy).Scan(&d.UploaderName)
	}
	return &d, nil
}

func (s *Store) DeleteLeadDocument(ctx context.Context, tenantID, leadID, docID xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM lead_documents WHERE tenant_id=$1 AND lead_id=$2 AND id=$3
	`, tenantID, leadID, docID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListCustomerDocuments(ctx context.Context, tenantID, customerID xid.ID) ([]CustomerDocument, error) {
	if _, err := s.GetCustomer(ctx, tenantID, customerID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT d.id, d.customer_id, d.kind, d.url, d.caption, d.source, d.source_ref,
		       d.uploaded_by, COALESCE(u.full_name, ''), d.created_at
		FROM customer_documents d
		LEFT JOIN users u ON u.id = d.uploaded_by
		WHERE d.tenant_id=$1 AND d.customer_id=$2
		ORDER BY d.created_at ASC
	`, tenantID, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CustomerDocument
	for rows.Next() {
		var d CustomerDocument
		if err := rows.Scan(&d.ID, &d.CustomerID, &d.Kind, &d.URL, &d.Caption, &d.Source, &d.SourceRef,
			&d.UploadedBy, &d.UploaderName, &d.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

func (s *Store) AddCustomerDocument(ctx context.Context, tenantID, customerID xid.ID, uploadedBy *xid.ID, kind, url, caption, source string, sourceRef *xid.ID) (*CustomerDocument, error) {
	if _, err := s.GetCustomer(ctx, tenantID, customerID); err != nil {
		return nil, err
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, fmt.Errorf("url dokumen wajib")
	}
	kind = NormalizeDocumentKind(kind)
	caption = strings.TrimSpace(caption)
	source = strings.TrimSpace(strings.ToLower(source))
	if source != "lead_convert" {
		source = "upload"
	}
	var d CustomerDocument
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO customer_documents (tenant_id, customer_id, kind, url, caption, source, source_ref, uploaded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, customer_id, kind, url, caption, source, source_ref, uploaded_by, created_at
	`, tenantID, customerID, kind, url, caption, source, sourceRef, uploadedBy).Scan(
		&d.ID, &d.CustomerID, &d.Kind, &d.URL, &d.Caption, &d.Source, &d.SourceRef, &d.UploadedBy, &d.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if uploadedBy != nil {
		_ = s.Pool.QueryRow(ctx, `SELECT COALESCE(full_name,'') FROM users WHERE id=$1`, *uploadedBy).Scan(&d.UploaderName)
	}
	return &d, nil
}

func (s *Store) DeleteCustomerDocument(ctx context.Context, tenantID, customerID, docID xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM customer_documents WHERE tenant_id=$1 AND customer_id=$2 AND id=$3
	`, tenantID, customerID, docID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CopyLeadDocumentsToCustomer copies PSB/gallery docs from a lead onto a new customer.
func (s *Store) CopyLeadDocumentsToCustomer(ctx context.Context, tenantID, leadID, customerID xid.ID) (int, error) {
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO customer_documents (tenant_id, customer_id, kind, url, caption, source, source_ref, uploaded_by, created_at)
		SELECT tenant_id, $3, kind, url, caption, 'lead_convert', lead_id, uploaded_by, created_at
		FROM lead_documents
		WHERE tenant_id=$1 AND lead_id=$2
	`, tenantID, leadID, customerID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
