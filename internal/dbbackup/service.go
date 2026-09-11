package dbbackup

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	FormatTenantJSON = "tenant_json"
	Version          = 1
)

var safeNameRE = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type Service struct {
	pool  *pgxpool.Pool
	dbURL string
	root  string
}

func New(pool *pgxpool.Pool, databaseURL, root string) (*Service, error) {
	if root == "" {
		root = "./data/db-backups"
	}
	if err := os.MkdirAll(filepath.Join(root, "platform"), 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "tenants"), 0o750); err != nil {
		return nil, err
	}
	return &Service{pool: pool, dbURL: databaseURL, root: root}, nil
}

type FileInfo struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Scope     string    `json:"scope"`
}

type TenantArchive struct {
	Version    int                          `json:"version"`
	Format     string                       `json:"format"`
	TenantID   string                       `json:"tenant_id"`
	Slug       string                       `json:"slug,omitempty"`
	ExportedAt string                       `json:"exported_at"`
	Tables     map[string][]json.RawMessage `json:"tables"`
}

func (s *Service) platformDir() string { return filepath.Join(s.root, "platform") }

func (s *Service) tenantDir(tenantID xid.ID) string {
	dir := filepath.Join(s.root, "tenants", tenantID.String())
	_ = os.MkdirAll(dir, 0o750)
	return dir
}

func (s *Service) ListPlatform(ctx context.Context) ([]FileInfo, error) {
	_ = ctx
	return listDir(s.platformDir(), "platform")
}

func (s *Service) ListTenant(ctx context.Context, tenantID xid.ID) ([]FileInfo, error) {
	_ = ctx
	return listDir(s.tenantDir(tenantID), "tenant")
}

func listDir(dir, scope string) ([]FileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []FileInfo{}, nil
		}
		return nil, err
	}
	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !safeNameRE.MatchString(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, FileInfo{
			Name:      name,
			Size:      info.Size(),
			CreatedAt: info.ModTime().UTC(),
			Scope:     scope,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Service) CreatePlatformBackup(ctx context.Context) (*FileInfo, error) {
	name := fmt.Sprintf("full-%s.sql.gz", time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(s.platformDir(), name)

	cmd := exec.CommandContext(ctx, "pg_dump",
		"--dbname="+s.dbURL,
		"--no-owner",
		"--no-acl",
		"--clean",
		"--if-exists",
		"--format=plain",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("pg_dump start: %w (%s)", err, stderr.String())
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	gz := gzip.NewWriter(f)
	_, copyErr := io.Copy(gz, stdout)
	closeErr := gz.Close()
	fileErr := f.Close()
	waitErr := cmd.Wait()
	if copyErr != nil || closeErr != nil || fileErr != nil || waitErr != nil {
		_ = os.Remove(path)
		if waitErr != nil {
			return nil, fmt.Errorf("pg_dump: %w (%s)", waitErr, strings.TrimSpace(stderr.String()))
		}
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return nil, fileErr
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &FileInfo{Name: name, Size: st.Size(), CreatedAt: st.ModTime().UTC(), Scope: "platform"}, nil
}

func (s *Service) RestorePlatform(ctx context.Context, name string, r io.Reader, gzipped bool) error {
	if name != "" {
		path, err := s.ResolvePlatformPath(name)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
		gzipped = strings.HasSuffix(strings.ToLower(name), ".gz")
	}
	if r == nil {
		return fmt.Errorf("no backup input")
	}
	sqlReader := r
	if gzipped {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return fmt.Errorf("gunzip: %w", err)
		}
		defer gz.Close()
		sqlReader = gz
	}

	cmd := exec.CommandContext(ctx, "psql",
		"--dbname="+s.dbURL,
		"-v", "ON_ERROR_STOP=1",
		"-q",
	)
	var stderr bytes.Buffer
	cmd.Stdin = sqlReader
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (s *Service) CreateTenantBackup(ctx context.Context, tenantID xid.ID, slug string) (*FileInfo, error) {
	tables, err := s.tenantTables(ctx)
	if err != nil {
		return nil, err
	}
	arch := TenantArchive{
		Version:    Version,
		Format:     FormatTenantJSON,
		TenantID:   tenantID.String(),
		Slug:       slug,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Tables:     make(map[string][]json.RawMessage, len(tables)+1),
	}

	tenantRows, err := s.selectJSON(ctx, "tenants", "id = $1", tenantID)
	if err != nil {
		return nil, err
	}
	arch.Tables["tenants"] = tenantRows

	for _, table := range tables {
		if table == "tenants" {
			continue
		}
		rows, err := s.selectJSON(ctx, table, "tenant_id = $1", tenantID)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", table, err)
		}
		arch.Tables[table] = rows
	}

	raw, err := json.Marshal(arch)
	if err != nil {
		return nil, err
	}
	prefix := slug
	if prefix == "" {
		prefix = tenantID.String()
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
	}
	prefix = sanitizeFilePart(prefix)
	name := fmt.Sprintf("tenant-%s-%s.json.gz", prefix, time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(s.tenantDir(tenantID), name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(raw); err != nil {
		_ = gz.Close()
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := gz.Close(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &FileInfo{Name: name, Size: st.Size(), CreatedAt: st.ModTime().UTC(), Scope: "tenant"}, nil
}

func (s *Service) RestoreTenant(ctx context.Context, tenantID xid.ID, name string, r io.Reader, gzipped bool) error {
	if name != "" {
		path, err := s.ResolveTenantPath(tenantID, name)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
		gzipped = strings.HasSuffix(strings.ToLower(name), ".gz")
	}
	if r == nil {
		return fmt.Errorf("no backup input")
	}
	var data []byte
	var err error
	if gzipped {
		gz, gerr := gzip.NewReader(r)
		if gerr != nil {
			return gerr
		}
		defer gz.Close()
		data, err = io.ReadAll(gz)
	} else {
		data, err = io.ReadAll(r)
	}
	if err != nil {
		return err
	}
	var arch TenantArchive
	if err := json.Unmarshal(data, &arch); err != nil {
		return fmt.Errorf("invalid tenant backup: %w", err)
	}
	if arch.Format != FormatTenantJSON {
		return fmt.Errorf("format tidak didukung: %s", arch.Format)
	}
	if arch.TenantID != "" && arch.TenantID != tenantID.String() {
		return fmt.Errorf("backup milik tenant lain (%s)", arch.TenantID)
	}

	tables, err := s.tenantTables(ctx)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		return fmt.Errorf("enable replica role: %w (butuh role DB yang boleh set session_replication_role)", err)
	}

	for _, table := range tables {
		if table == "tenants" {
			continue
		}
		q := fmt.Sprintf(`DELETE FROM %s WHERE tenant_id = $1`, quoteIdent(table))
		if _, err := tx.Exec(ctx, q, tenantID); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}

	for _, table := range tables {
		if table == "tenants" {
			continue
		}
		for _, row := range arch.Tables[table] {
			if err := insertJSONRow(ctx, tx, table, row); err != nil {
				return fmt.Errorf("insert %s: %w", table, err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = origin`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) OpenPlatform(name string) (*os.File, *FileInfo, error) {
	path, err := s.ResolvePlatformPath(name)
	if err != nil {
		return nil, nil, err
	}
	return openInfo(path, "platform")
}

func (s *Service) OpenTenant(tenantID xid.ID, name string) (*os.File, *FileInfo, error) {
	path, err := s.ResolveTenantPath(tenantID, name)
	if err != nil {
		return nil, nil, err
	}
	return openInfo(path, "tenant")
}

func (s *Service) DeletePlatform(name string) error {
	path, err := s.ResolvePlatformPath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *Service) DeleteTenant(tenantID xid.ID, name string) error {
	path, err := s.ResolveTenantPath(tenantID, name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *Service) ResolvePlatformPath(name string) (string, error) {
	if !safeNameRE.MatchString(name) {
		return "", fmt.Errorf("nama file tidak valid")
	}
	path := filepath.Join(s.platformDir(), filepath.Base(name))
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("backup tidak ditemukan")
	}
	return path, nil
}

func (s *Service) ResolveTenantPath(tenantID xid.ID, name string) (string, error) {
	if !safeNameRE.MatchString(name) {
		return "", fmt.Errorf("nama file tidak valid")
	}
	path := filepath.Join(s.tenantDir(tenantID), filepath.Base(name))
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("backup tidak ditemukan")
	}
	return path, nil
}

func openInfo(path, scope string) (*os.File, *FileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, &FileInfo{
		Name:      filepath.Base(path),
		Size:      st.Size(),
		CreatedAt: st.ModTime().UTC(),
		Scope:     scope,
	}, nil
}

func (s *Service) tenantTables(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.table_name
		FROM information_schema.columns c
		JOIN information_schema.tables t
		  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = 'public'
		  AND c.column_name = 'tenant_id'
		  AND t.table_type = 'BASE TABLE'
		ORDER BY c.table_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !safeIdent(name) {
			continue
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (s *Service) selectJSON(ctx context.Context, table, where string, arg any) ([]json.RawMessage, error) {
	q := fmt.Sprintf(`SELECT COALESCE(jsonb_agg(to_jsonb(t)), '[]'::jsonb) FROM %s t WHERE %s`, quoteIdent(table), where)
	var raw []byte
	if err := s.pool.QueryRow(ctx, q, arg).Scan(&raw); err != nil {
		return nil, err
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func insertJSONRow(ctx context.Context, tx pgx.Tx, table string, row json.RawMessage) error {
	q := fmt.Sprintf(
		`INSERT INTO %s SELECT * FROM jsonb_populate_record(NULL::%s, $1::jsonb)`,
		quoteIdent(table), quoteIdent(table),
	)
	_, err := tx.Exec(ctx, q, string(row))
	return err
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func safeIdent(name string) bool {
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return name != ""
}

func sanitizeFilePart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "tenant"
	}
	if len(out) > 40 {
		return out[:40]
	}
	return out
}
