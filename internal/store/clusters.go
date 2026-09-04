package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

// Cluster is a POP / area within a tenant (stored in sites table).
type Cluster struct {
	ID                   xid.ID    `json:"id"`
	TenantID             xid.ID    `json:"tenant_id"`
	Name                 string    `json:"name"`
	Code                 string    `json:"code"`
	CustomerCodePrefix   string    `json:"customer_code_prefix"`
	CustomerCodePattern  string    `json:"customer_code_pattern"`
	SeqWidth             int       `json:"seq_width"`
	Address              *string   `json:"address,omitempty"`
	Latitude             *float64  `json:"latitude,omitempty"`
	Longitude            *float64  `json:"longitude,omitempty"`
	Notes                *string   `json:"notes,omitempty"`
	IsActive             bool      `json:"is_active"`
	CreatedAt            time.Time `json:"created_at"`
}

const defaultCustomerCodePattern = "{prefix}-{yyyymm}-{seq}"

func NormalizeClusterCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func FormatCustomerCode(pattern, prefix string, seqWidth int, now time.Time, seq int) string {
	if strings.TrimSpace(pattern) == "" {
		pattern = defaultCustomerCodePattern
	}
	if seqWidth <= 0 {
		seqWidth = 4
	}
	seqStr := fmt.Sprintf("%0*d", seqWidth, seq)
	replacer := strings.NewReplacer(
		"{prefix}", prefix,
		"{yyyymm}", now.Format("200601"),
		"{yyyy}", now.Format("2006"),
		"{yy}", now.Format("06"),
		"{mm}", now.Format("01"),
		"{seq}", seqStr,
	)
	return replacer.Replace(pattern)
}

func sequencePeriod(pattern string, now time.Time) string {
	p := pattern
	if strings.TrimSpace(p) == "" {
		p = defaultCustomerCodePattern
	}
	if strings.Contains(p, "{yyyymm}") ||
		strings.Contains(p, "{yyyy}") ||
		strings.Contains(p, "{yy}") ||
		strings.Contains(p, "{mm}") {
		return now.Format("200601")
	}
	return ""
}

func (s *Store) ListClusters(ctx context.Context, tenantID xid.ID) ([]Cluster, error) {
	var list []Cluster
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant_id, name, code, customer_code_prefix, customer_code_pattern, seq_width,
			       address, latitude, longitude, notes, is_active, created_at
			FROM sites WHERE tenant_id = $1 ORDER BY name
		`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Cluster
			if err := rows.Scan(&c.ID, &c.TenantID, &c.Name, &c.Code, &c.CustomerCodePrefix, &c.CustomerCodePattern,
				&c.SeqWidth, &c.Address, &c.Latitude, &c.Longitude, &c.Notes, &c.IsActive, &c.CreatedAt); err != nil {
				return err
			}
			list = append(list, c)
		}
		return rows.Err()
	})
	return list, err
}

func (s *Store) GetCluster(ctx context.Context, tenantID, id xid.ID) (*Cluster, error) {
	var c Cluster
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, name, code, customer_code_prefix, customer_code_pattern, seq_width,
			       address, latitude, longitude, notes, is_active, created_at
			FROM sites WHERE tenant_id = $1 AND id = $2
		`, tenantID, id)
		err := row.Scan(&c.ID, &c.TenantID, &c.Name, &c.Code, &c.CustomerCodePrefix, &c.CustomerCodePattern,
			&c.SeqWidth, &c.Address, &c.Latitude, &c.Longitude, &c.Notes, &c.IsActive, &c.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) CreateCluster(ctx context.Context, c *Cluster) error {
	code := NormalizeClusterCode(c.Code)
	if code == "" {
		return fmt.Errorf("cluster code required")
	}
	c.Code = code
	prefix := NormalizeClusterCode(c.CustomerCodePrefix)
	if prefix == "" {
		prefix = code
	}
	c.CustomerCodePrefix = prefix
	if strings.TrimSpace(c.CustomerCodePattern) == "" {
		c.CustomerCodePattern = defaultCustomerCodePattern
	}
	if c.SeqWidth <= 0 {
		c.SeqWidth = 4
	}
	return s.withTenant(ctx, c.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO sites (tenant_id, name, code, customer_code_prefix, customer_code_pattern, seq_width,
			                   address, latitude, longitude, notes, is_active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			RETURNING id, created_at
		`, c.TenantID, c.Name, c.Code, c.CustomerCodePrefix, c.CustomerCodePattern, c.SeqWidth,
			c.Address, c.Latitude, c.Longitude, c.Notes, c.IsActive).Scan(&c.ID, &c.CreatedAt)
	})
}

func (s *Store) UpdateCluster(ctx context.Context, c *Cluster) error {
	code := NormalizeClusterCode(c.Code)
	if code == "" {
		return fmt.Errorf("cluster code required")
	}
	c.Code = code
	prefix := NormalizeClusterCode(c.CustomerCodePrefix)
	if prefix == "" {
		prefix = code
	}
	c.CustomerCodePrefix = prefix
	if strings.TrimSpace(c.CustomerCodePattern) == "" {
		c.CustomerCodePattern = defaultCustomerCodePattern
	}
	if c.SeqWidth <= 0 {
		c.SeqWidth = 4
	}
	return s.withTenant(ctx, c.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE sites SET name=$3, code=$4, customer_code_prefix=$5, customer_code_pattern=$6, seq_width=$7,
			                 address=$8, latitude=$9, longitude=$10, notes=$11, is_active=$12, updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2
		`, c.TenantID, c.ID, c.Name, c.Code, c.CustomerCodePrefix, c.CustomerCodePattern, c.SeqWidth,
			c.Address, c.Latitude, c.Longitude, c.Notes, c.IsActive)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) DeleteCluster(ctx context.Context, tenantID, id xid.ID) error {
	return s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM sites WHERE tenant_id=$1 AND id=$2`, tenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// NextCustomerCodeForCluster allocates the next code for a cluster (atomic per period).
func (s *Store) NextCustomerCodeForCluster(ctx context.Context, tenantID, clusterID xid.ID) (string, error) {
	var code string
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var c Cluster
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, name, code, customer_code_prefix, customer_code_pattern, seq_width,
			       address, latitude, longitude, notes, is_active, created_at
			FROM sites WHERE tenant_id = $1 AND id = $2
		`, tenantID, clusterID)
		if err := row.Scan(&c.ID, &c.TenantID, &c.Name, &c.Code, &c.CustomerCodePrefix, &c.CustomerCodePattern,
			&c.SeqWidth, &c.Address, &c.Latitude, &c.Longitude, &c.Notes, &c.IsActive, &c.CreatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		now := time.Now()
		period := sequencePeriod(c.CustomerCodePattern, now)
		var seq int
		if err := tx.QueryRow(ctx, `
			INSERT INTO customer_code_sequences (cluster_id, period, last_seq)
			VALUES ($1, $2, 1)
			ON CONFLICT (cluster_id, period)
			DO UPDATE SET last_seq = customer_code_sequences.last_seq + 1
			RETURNING last_seq
		`, clusterID, period).Scan(&seq); err != nil {
			return err
		}
		code = FormatCustomerCode(c.CustomerCodePattern, c.CustomerCodePrefix, c.SeqWidth, now, seq)
		return nil
	})
	return code, err
}

// PreviewCustomerCode shows what the next code would look like without allocating.
func (s *Store) PreviewCustomerCode(ctx context.Context, tenantID, clusterID xid.ID) (string, error) {
	c, err := s.GetCluster(ctx, tenantID, clusterID)
	if err != nil {
		return "", err
	}
	now := time.Now()
	period := sequencePeriod(c.CustomerCodePattern, now)
	var last int
	err = s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT last_seq FROM customer_code_sequences WHERE cluster_id = $1 AND period = $2
		`, clusterID, period)
		err := row.Scan(&last)
		if errors.Is(err, pgx.ErrNoRows) {
			last = 0
			return nil
		}
		return err
	})
	if err != nil {
		return "", err
	}
	return FormatCustomerCode(c.CustomerCodePattern, c.CustomerCodePrefix, c.SeqWidth, now, last+1), nil
}
