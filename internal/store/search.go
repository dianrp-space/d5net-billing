package store

import (
	"context"
	"strings"

	"github.com/dianrp/drp-billing/internal/xid"
)

// SearchHit is one global-search result for the admin header.
type SearchHit struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Page     string `json:"page"`
}

func (s *Store) GlobalSearch(ctx context.Context, tenantID xid.ID, q string, perKind int) ([]SearchHit, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if perKind <= 0 {
		perKind = 5
	}
	if perKind > 20 {
		perKind = 20
	}
	pat := "%" + q + "%"
	var hits []SearchHit

	appendRows := func(kind, page string, rows [][3]string) {
		for _, r := range rows {
			hits = append(hits, SearchHit{Kind: kind, ID: r[0], Title: r[1], Subtitle: r[2], Page: page})
		}
	}

	{
		rows, err := s.Pool.Query(ctx, `
			SELECT id::text, full_name, COALESCE(customer_code, '') || CASE WHEN phone IS NOT NULL AND phone <> '' THEN ' · ' || phone ELSE '' END
			FROM customers
			WHERE tenant_id = $1
			  AND (full_name ILIKE $2 OR customer_code ILIKE $2 OR COALESCE(phone,'') ILIKE $2 OR COALESCE(email,'') ILIKE $2)
			ORDER BY full_name
			LIMIT $3
		`, tenantID, pat, perKind)
		if err != nil {
			return nil, err
		}
		var batch [][3]string
		for rows.Next() {
			var id, title, sub string
			if err := rows.Scan(&id, &title, &sub); err != nil {
				rows.Close()
				return nil, err
			}
			batch = append(batch, [3]string{id, title, sub})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		appendRows("customer", "customers", batch)
	}

	{
		rows, err := s.Pool.Query(ctx, `
			SELECT s.id::text, s.username, COALESCE(c.full_name, '') || ' · ' || COALESCE(p.name, '') || ' · ' || s.status
			FROM subscriptions s
			JOIN customers c ON c.id = s.customer_id
			JOIN plans p ON p.id = s.plan_id
			WHERE s.tenant_id = $1
			  AND (s.username ILIKE $2 OR c.full_name ILIKE $2 OR c.customer_code ILIKE $2 OR p.name ILIKE $2 OR p.code ILIKE $2)
			ORDER BY s.id DESC
			LIMIT $3
		`, tenantID, pat, perKind)
		if err != nil {
			return nil, err
		}
		var batch [][3]string
		for rows.Next() {
			var id, title, sub string
			if err := rows.Scan(&id, &title, &sub); err != nil {
				rows.Close()
				return nil, err
			}
			batch = append(batch, [3]string{id, title, sub})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		appendRows("subscription", "subscriptions", batch)
	}

	{
		rows, err := s.Pool.Query(ctx, `
			SELECT i.id::text, i.invoice_number, COALESCE(c.full_name, '') || ' · ' || i.status
			FROM invoices i
			JOIN customers c ON c.id = i.customer_id
			WHERE i.tenant_id = $1
			  AND (i.invoice_number ILIKE $2 OR c.full_name ILIKE $2 OR c.customer_code ILIKE $2)
			ORDER BY i.created_at DESC
			LIMIT $3
		`, tenantID, pat, perKind)
		if err != nil {
			return nil, err
		}
		var batch [][3]string
		for rows.Next() {
			var id, title, sub string
			if err := rows.Scan(&id, &title, &sub); err != nil {
				rows.Close()
				return nil, err
			}
			batch = append(batch, [3]string{id, title, sub})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		appendRows("invoice", "invoices", batch)
	}

	{
		rows, err := s.Pool.Query(ctx, `
			SELECT id::text, name, code || ' · sisa ' ||
			       COALESCE((SELECT COUNT(*)::text FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available'), '0') ||
			       '/' || port_count::text
			FROM odps o
			WHERE tenant_id = $1 AND (name ILIKE $2 OR code ILIKE $2 OR COALESCE(address,'') ILIKE $2)
			ORDER BY name
			LIMIT $3
		`, tenantID, pat, perKind)
		if err != nil {
			return nil, err
		}
		var batch [][3]string
		for rows.Next() {
			var id, title, sub string
			if err := rows.Scan(&id, &title, &sub); err != nil {
				rows.Close()
				return nil, err
			}
			batch = append(batch, [3]string{id, title, sub})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		appendRows("odp", "odp", batch)
	}

	{
		rows, err := s.Pool.Query(ctx, `
			SELECT id::text, name, code || ' · ' || service_type
			FROM plans
			WHERE tenant_id = $1 AND (name ILIKE $2 OR code ILIKE $2 OR COALESCE(profile_name,'') ILIKE $2)
			ORDER BY name
			LIMIT $3
		`, tenantID, pat, perKind)
		if err != nil {
			return nil, err
		}
		var batch [][3]string
		for rows.Next() {
			var id, title, sub string
			if err := rows.Scan(&id, &title, &sub); err != nil {
				rows.Close()
				return nil, err
			}
			batch = append(batch, [3]string{id, title, sub})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		appendRows("plan", "plans", batch)
	}

	{
		rows, err := s.Pool.Query(ctx, `
			SELECT id::text, name, COALESCE(address, '') || CASE WHEN port > 0 THEN ':' || port::text ELSE '' END
			FROM routers
			WHERE tenant_id = $1 AND (name ILIKE $2 OR COALESCE(address,'') ILIKE $2 OR COALESCE(username,'') ILIKE $2)
			ORDER BY name
			LIMIT $3
		`, tenantID, pat, perKind)
		if err != nil {
			return nil, err
		}
		var batch [][3]string
		for rows.Next() {
			var id, title, sub string
			if err := rows.Scan(&id, &title, &sub); err != nil {
				rows.Close()
				return nil, err
			}
			batch = append(batch, [3]string{id, title, sub})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		appendRows("router", "routers", batch)
	}

	{
		rows, err := s.Pool.Query(ctx, `
			SELECT id::text, name, code
			FROM sites
			WHERE tenant_id = $1 AND (name ILIKE $2 OR code ILIKE $2)
			ORDER BY name
			LIMIT $3
		`, tenantID, pat, perKind)
		if err != nil {
			return nil, err
		}
		var batch [][3]string
		for rows.Next() {
			var id, title, sub string
			if err := rows.Scan(&id, &title, &sub); err != nil {
				rows.Close()
				return nil, err
			}
			batch = append(batch, [3]string{id, title, sub})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		appendRows("cluster", "clusters", batch)
	}

	return hits, nil
}
