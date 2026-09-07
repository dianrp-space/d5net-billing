package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/dbbackup"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/go-chi/chi/v5"
)

func registerDBBackup(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-tenant-db-backups", Method: http.MethodGet, Path: "/api/db-backups",
		Tags: []string{"Backup"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []dbbackup.FileInfo }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		if d.DBBackup == nil {
			return nil, httpx.Internal(fmt.Errorf("db backup not configured"))
		}
		list, err := d.DBBackup.ListTenant(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []dbbackup.FileInfo{}
		}
		return &struct{ Body []dbbackup.FileInfo }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-tenant-db-backup", Method: http.MethodPost, Path: "/api/db-backups",
		Tags: []string{"Backup"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body dbbackup.FileInfo }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		if d.DBBackup == nil {
			return nil, httpx.Internal(fmt.Errorf("db backup not configured"))
		}
		slug := ""
		if t, err := d.Store.GetTenant(ctx, tid); err == nil && t != nil {
			slug = t.Slug
		}
		info, err := d.DBBackup.CreateTenantBackup(ctx, tid, slug)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body dbbackup.FileInfo }{Body: *info}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-tenant-db-backup", Method: http.MethodDelete, Path: "/api/db-backups/{name}",
		Tags: []string{"Backup"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Name string `path:"name"`
	}) (*struct{}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		if d.DBBackup == nil {
			return nil, httpx.Internal(fmt.Errorf("db backup not configured"))
		}
		if err := d.DBBackup.DeleteTenant(tid, input.Name); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "restore-tenant-db-backup", Method: http.MethodPost, Path: "/api/db-backups/restore",
		Tags: []string{"Backup"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name    string `json:"name,omitempty"`
			Confirm bool   `json:"confirm"`
		}
	}) (*struct {
		Body struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		if !input.Body.Confirm {
			return nil, httpx.BadRequest("confirm=true wajib untuk restore")
		}
		if strings.TrimSpace(input.Body.Name) == "" {
			return nil, httpx.BadRequest("name wajib (atau upload file via multipart)")
		}
		if d.DBBackup == nil {
			return nil, httpx.Internal(fmt.Errorf("db backup not configured"))
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if err := d.DBBackup.RestoreTenant(ctx, tid, input.Body.Name, nil, false); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct {
			Body struct {
				OK      bool   `json:"ok"`
				Message string `json:"message"`
			}
		}{Body: struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		}{OK: true, Message: "Restore tenant selesai"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-platform-db-backups", Method: http.MethodGet, Path: "/api/platform/db-backups",
		Tags: []string{"Platform"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []dbbackup.FileInfo }, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		if d.DBBackup == nil {
			return nil, httpx.Internal(fmt.Errorf("db backup not configured"))
		}
		list, err := d.DBBackup.ListPlatform(ctx)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []dbbackup.FileInfo{}
		}
		return &struct{ Body []dbbackup.FileInfo }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-platform-db-backup", Method: http.MethodPost, Path: "/api/platform/db-backups",
		Tags: []string{"Platform"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body dbbackup.FileInfo }, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		if d.DBBackup == nil {
			return nil, httpx.Internal(fmt.Errorf("db backup not configured"))
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		info, err := d.DBBackup.CreatePlatformBackup(ctx)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body dbbackup.FileInfo }{Body: *info}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-platform-db-backup", Method: http.MethodDelete, Path: "/api/platform/db-backups/{name}",
		Tags: []string{"Platform"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Name string `path:"name"`
	}) (*struct{}, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		if d.DBBackup == nil {
			return nil, httpx.Internal(fmt.Errorf("db backup not configured"))
		}
		if err := d.DBBackup.DeletePlatform(input.Name); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "restore-platform-db-backup", Method: http.MethodPost, Path: "/api/platform/db-backups/restore",
		Tags: []string{"Platform"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name    string `json:"name,omitempty"`
			Confirm bool   `json:"confirm"`
		}
	}) (*struct {
		Body struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		}
	}, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		if !input.Body.Confirm {
			return nil, httpx.BadRequest("confirm=true wajib untuk restore")
		}
		if strings.TrimSpace(input.Body.Name) == "" {
			return nil, httpx.BadRequest("name wajib")
		}
		if d.DBBackup == nil {
			return nil, httpx.Internal(fmt.Errorf("db backup not configured"))
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		if err := d.DBBackup.RestorePlatform(ctx, input.Body.Name, nil, false); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct {
			Body struct {
				OK      bool   `json:"ok"`
				Message string `json:"message"`
			}
		}{Body: struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		}{OK: true, Message: "Restore database selesai"}}, nil
	})
}

// MountDBBackupRoutes registers download + multipart restore endpoints.
func MountDBBackupRoutes(r chi.Router, d *Deps) {
	r.Get("/api/db-backups/{name}/download", func(w http.ResponseWriter, req *http.Request) {
		tid, err := requireSettings(req.Context(), d)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if d.DBBackup == nil {
			http.Error(w, `{"error":"not configured"}`, http.StatusInternalServerError)
			return
		}
		name := chi.URLParam(req, "name")
		f, info, err := d.DBBackup.OpenTenant(tid, name)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
			return
		}
		defer f.Close()
		serveDownload(w, info.Name, info.Size, f)
	})

	r.Post("/api/db-backups/restore-upload", func(w http.ResponseWriter, req *http.Request) {
		tid, err := requireSettings(req.Context(), d)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if d.DBBackup == nil {
			http.Error(w, `{"error":"not configured"}`, http.StatusInternalServerError)
			return
		}
		if req.FormValue("confirm") != "true" && req.URL.Query().Get("confirm") != "true" {
			_ = req.ParseMultipartForm(64 << 20)
		}
		_ = req.ParseMultipartForm(64 << 20)
		if req.FormValue("confirm") != "true" {
			http.Error(w, `{"error":"confirm=true wajib"}`, http.StatusBadRequest)
			return
		}
		file, header, err := req.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file wajib"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		gzipped := strings.HasSuffix(strings.ToLower(header.Filename), ".gz")
		ctx, cancel := context.WithTimeout(req.Context(), 10*time.Minute)
		defer cancel()
		if err := d.DBBackup.RestoreTenant(ctx, tid, "", file, gzipped); err != nil {
			http.Error(w, `{"error":`+jsonQuote(err.Error())+`}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "Restore tenant selesai"})
	})

	r.Get("/api/platform/db-backups/{name}/download", func(w http.ResponseWriter, req *http.Request) {
		if err := requirePlatform(req.Context()); err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if d.DBBackup == nil {
			http.Error(w, `{"error":"not configured"}`, http.StatusInternalServerError)
			return
		}
		name := chi.URLParam(req, "name")
		f, info, err := d.DBBackup.OpenPlatform(name)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusNotFound)
			return
		}
		defer f.Close()
		serveDownload(w, info.Name, info.Size, f)
	})

	r.Post("/api/platform/db-backups/restore-upload", func(w http.ResponseWriter, req *http.Request) {
		if err := requirePlatform(req.Context()); err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if d.DBBackup == nil {
			http.Error(w, `{"error":"not configured"}`, http.StatusInternalServerError)
			return
		}
		_ = req.ParseMultipartForm(512 << 20)
		if req.FormValue("confirm") != "true" {
			http.Error(w, `{"error":"confirm=true wajib"}`, http.StatusBadRequest)
			return
		}
		file, header, err := req.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file wajib"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		gzipped := strings.HasSuffix(strings.ToLower(header.Filename), ".gz")
		ctx, cancel := context.WithTimeout(req.Context(), 30*time.Minute)
		defer cancel()
		if err := d.DBBackup.RestorePlatform(ctx, "", file, gzipped); err != nil {
			http.Error(w, `{"error":`+jsonQuote(err.Error())+`}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "Restore database selesai"})
	})
}

func serveDownload(w http.ResponseWriter, name string, size int64, r io.Reader) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	if size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	_, _ = io.Copy(w, r)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
