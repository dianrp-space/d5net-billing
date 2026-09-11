package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// ticketSeverity maps ticket priority to alert severity.
func ticketSeverity(priority string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "urgent":
		return "critical"
	case "high":
		return "warn"
	default:
		return "info"
	}
}

// queueTicketAlert records a tenant-wide bell alert for a new ticket.
func queueTicketAlert(ctx context.Context, d *Deps, tid xid.ID, t *store.Ticket) {
	if t == nil {
		return
	}
	et := "ticket"
	eid := t.ID
	_ = d.Store.CreateAlert(ctx, &store.Alert{
		TenantID: tid, Severity: ticketSeverity(t.Priority), Kind: "ticket_new",
		Title:   "Tiket baru: " + strings.TrimSpace(t.Subject),
		Message: "Prioritas: " + t.Priority + " · Kategori: " + t.Category,
		EntityType: &et, EntityID: &eid,
	})
}

// queueLeadAlert records a tenant-wide bell alert for lead teamwork events.
func queueLeadAlert(ctx context.Context, d *Deps, tid xid.ID, kind, title, message string, leadID xid.ID) {
	et := "lead"
	eid := leadID
	_ = d.Store.CreateAlert(ctx, &store.Alert{
		TenantID: tid, Severity: "info", Kind: kind,
		Title:   title,
		Message: message,
		EntityType: &et, EntityID: &eid,
	})
}

// Alerts feed the header notification bell.
// (List lives in extra.go alongside the dashboard AlertsPanel consumer.)
func registerAlerts(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "ack-alert", Method: http.MethodPost, Path: "/api/alerts/{id}/ack",
		Summary: "Mark alert as read", Tags: []string{"Alerts"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.AckAlert(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("notifikasi tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "ok"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ack-all-alerts", Method: http.MethodPost, Path: "/api/alerts/ack-all",
		Summary: "Mark all alerts as read", Tags: []string{"Alerts"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Acked int64 `json:"acked"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		n, err := d.Store.AckAllAlerts(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Acked int64 `json:"acked"`
			}
		}{}
		out.Body.Acked = n
		return out, nil
	})
}
