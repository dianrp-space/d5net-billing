package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/notify"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// broadcastFilterFromInput memvalidasi filter area opsional (cluster/ODP).
// String kosong = tanpa filter.
func broadcastFilterFromInput(clusterID, odpID string) (store.BroadcastFilter, error) {
	var f store.BroadcastFilter
	if clusterID != "" {
		id, err := xid.Parse(clusterID)
		if err != nil {
			return f, httpx.BadRequest("cluster_id tidak valid")
		}
		f.ClusterID = &id
	}
	if odpID != "" {
		id, err := xid.Parse(odpID)
		if err != nil {
			return f, httpx.BadRequest("odp_id tidak valid")
		}
		f.ODPID = &id
	}
	return f, nil
}

func registerNotifications(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-notification-templates", Method: http.MethodGet, Path: "/api/notifications/templates",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.NotificationTemplate }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListNotificationTemplates(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.NotificationTemplate{}
		}
		return &struct{ Body []store.NotificationTemplate }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-notification-template-events", Method: http.MethodGet, Path: "/api/notifications/templates/catalog",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []notify.TemplateEvent }, error) {
		return &struct{ Body []notify.TemplateEvent }{Body: notify.TemplateCatalog()}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "upsert-notification-template", Method: http.MethodPut, Path: "/api/notifications/templates",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Channel string  `json:"channel"`
			Event   string  `json:"event"`
			Subject *string `json:"subject,omitempty"`
			Body    string  `json:"body"`
		}
	}) (*struct{ Body store.NotificationTemplate }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		ch := strings.TrimSpace(input.Body.Channel)
		ev := strings.TrimSpace(input.Body.Event)
		if ch == "" || ev == "" || strings.TrimSpace(input.Body.Body) == "" {
			return nil, httpx.BadRequest("channel, event, dan body wajib")
		}
		t := &store.NotificationTemplate{
			TenantID: tid, Channel: ch, Event: ev, Subject: input.Body.Subject, Body: input.Body.Body,
		}
		if err := d.Store.UpsertNotificationTemplate(ctx, t); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.NotificationTemplate }{Body: *t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-notification-template", Method: http.MethodDelete, Path: "/api/notifications/templates/{id}",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteNotificationTemplate(ctx, tid, input.ID); err != nil {
			return nil, httpx.NotFound("template not found")
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "ok"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "broadcast-preview", Method: http.MethodGet, Path: "/api/notifications/broadcast/preview",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Audience  string `query:"audience"`
		ClusterID string `query:"cluster_id"`
		ODPID     string `query:"odp_id"`
	}) (*struct {
		Body struct {
			Audience string `json:"audience"`
			Count    int64  `json:"count"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		aud := strings.TrimSpace(input.Audience)
		if aud == "" {
			aud = "active"
		}
		if aud != "overdue" && aud != "active" {
			return nil, httpx.BadRequest("audience harus overdue atau active")
		}
		filter, err := broadcastFilterFromInput(strings.TrimSpace(input.ClusterID), strings.TrimSpace(input.ODPID))
		if err != nil {
			return nil, err
		}
		n, err := d.Store.CountBroadcastPhonesFiltered(ctx, tid, aud, filter)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Audience string `json:"audience"`
				Count    int64  `json:"count"`
			}
		}{}
		out.Body.Audience = aud
		out.Body.Count = n
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "broadcast-notifications", Method: http.MethodPost, Path: "/api/notifications/broadcast",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Channel       string   `json:"channel"`
			Audience      string   `json:"audience"` // overdue | active | custom
			ClusterID     string   `json:"cluster_id,omitempty"`
			ODPID         string   `json:"odp_id,omitempty"`
			Recipients    []string `json:"recipients,omitempty"`
			Subject       string   `json:"subject,omitempty"`
			Body          string   `json:"body"`
			TemplateEvent string   `json:"template_event,omitempty"`
			DelaySeconds  int      `json:"delay_seconds"`
		}
	}) (*struct {
		Body struct {
			Queued  int    `json:"queued"`
			BatchID string `json:"batch_id"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		ch := strings.TrimSpace(input.Body.Channel)
		if ch == "" {
			ch = "whatsapp"
		}
		body := strings.TrimSpace(input.Body.Body)
		subject := strings.TrimSpace(input.Body.Subject)
		// Template tersimpan hanya mengisi field yang kosong di composer —
		// hasil edit user di tab Broadcast selalu menang.
		if input.Body.TemplateEvent != "" {
			tpl, err := d.Store.GetNotificationTemplate(ctx, tid, ch, input.Body.TemplateEvent)
			if err == nil && tpl != nil {
				if body == "" {
					body = tpl.Body
				}
				if subject == "" && tpl.Subject != nil {
					subject = strings.TrimSpace(*tpl.Subject)
				}
			}
		}
		if body == "" {
			return nil, httpx.BadRequest("body atau template_event wajib")
		}
		var recipients []string
		var personalized []notify.BroadcastMessage
		switch strings.TrimSpace(input.Body.Audience) {
		case "custom":
			recipients = input.Body.Recipients
		case "overdue", "active", "":
			aud := input.Body.Audience
			if aud == "" {
				aud = "active"
			}
			filter, ferr := broadcastFilterFromInput(strings.TrimSpace(input.Body.ClusterID), strings.TrimSpace(input.Body.ODPID))
			if ferr != nil {
				return nil, ferr
			}
			recips, err := d.Store.ListBroadcastRecipientsFiltered(ctx, tid, aud, filter)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			// Isi variabel per penerima (nama, paket, tagihan acuan) agar
			// pesan tersimpan final per nomor di antrean.
			personalized = make([]notify.BroadcastMessage, 0, len(recips))
			for _, r := range recips {
				vars := notify.BroadcastVars(r.CustomerName, r.Phone, r.PlanName, r.ItemName, r.InvoiceNumber, r.Amount, r.DueDate)
				personalized = append(personalized, notify.BroadcastMessage{
					Recipient: r.Phone,
					Subject:   notify.RenderBroadcastBody(subject, vars),
					Body:      notify.RenderBroadcastBody(body, vars),
				})
			}
		default:
			return nil, httpx.BadRequest("audience harus overdue, active, atau custom")
		}
		if len(recipients) == 0 && len(personalized) == 0 {
			return nil, httpx.BadRequest("tidak ada penerima")
		}
		var n int
		var batch xid.ID
		if len(personalized) > 0 {
			n, batch, err = d.Notify.QueueBroadcastMessages(ctx, tid, ch, subject, personalized, input.Body.DelaySeconds, input.Body.TemplateEvent)
		} else {
			n, batch, err = d.Notify.QueueBroadcast(ctx, tid, ch, subject, body, recipients, input.Body.DelaySeconds, input.Body.TemplateEvent)
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Queued  int    `json:"queued"`
				BatchID string `json:"batch_id"`
			}
		}{}
		out.Body.Queued = n
		out.Body.BatchID = batch.String()
		auditEvent(ctx, d, AuditBroadcast, "notification", nil, map[string]any{
			"channel": ch, "audience": input.Body.Audience, "queued": n, "batch_id": batch.String(),
			"cluster_id": strings.TrimSpace(input.Body.ClusterID), "odp_id": strings.TrimSpace(input.Body.ODPID),
			"template_event": strings.TrimSpace(input.Body.TemplateEvent),
		})
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "broadcast-progress", Method: http.MethodGet, Path: "/api/notifications/broadcast/{batch}",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Batch string `path:"batch"`
	}) (*struct {
		Body struct {
			Total    int64                     `json:"total"`
			Pending  int64                     `json:"pending"`
			Sent     int64                     `json:"sent"`
			Failed   int64                     `json:"failed"`
			Failures []notify.BroadcastFailure `json:"failures"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		batchID, err := xid.Parse(strings.TrimSpace(input.Batch))
		if err != nil {
			return nil, httpx.BadRequest("batch tidak valid")
		}
		total, pending, sent, failed, failures, err := d.Notify.BroadcastProgress(ctx, tid, batchID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if failures == nil {
			failures = []notify.BroadcastFailure{}
		}
		out := &struct {
			Body struct {
				Total    int64                     `json:"total"`
				Pending  int64                     `json:"pending"`
				Sent     int64                     `json:"sent"`
				Failed   int64                     `json:"failed"`
				Failures []notify.BroadcastFailure `json:"failures"`
			}
		}{}
		out.Body.Total = total
		out.Body.Pending = pending
		out.Body.Sent = sent
		out.Body.Failed = failed
		out.Body.Failures = failures
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-notification-history", Method: http.MethodGet, Path: "/api/notifications/history",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status  string `query:"status"`
		Channel string `query:"channel"`
		Search  string `query:"search"`
		Limit   int    `query:"limit"`
		Offset  int    `query:"offset"`
	}) (*struct {
		Body struct {
			Data    []store.NotificationLog `json:"data"`
			Total   int64                   `json:"total"`
			Pending int64                   `json:"pending"`
			Sent    int64                   `json:"sent"`
			Failed  int64                   `json:"failed"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		list, total, err := d.Store.ListNotificationHistory(ctx, tid, input.Status, input.Channel, input.Search, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		pending, sent, failed, err := d.Store.NotificationHistoryStats(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Data    []store.NotificationLog `json:"data"`
				Total   int64                   `json:"total"`
				Pending int64                   `json:"pending"`
				Sent    int64                   `json:"sent"`
				Failed  int64                   `json:"failed"`
			}
		}{}
		out.Body.Data = list
		out.Body.Total = total
		out.Body.Pending = pending
		out.Body.Sent = sent
		out.Body.Failed = failed
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-notification-retention", Method: http.MethodGet, Path: "/api/notifications/retention",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			RetentionDays int `json:"retention_days"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		cfg, err := d.Store.GetJobScheduleSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				RetentionDays int `json:"retention_days"`
			}
		}{}
		out.Body.RetentionDays = cfg.NotifLogRetentionDays
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-notification-retention", Method: http.MethodPut, Path: "/api/notifications/retention",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			RetentionDays int `json:"retention_days"`
		}
	}) (*struct {
		Body struct {
			RetentionDays int `json:"retention_days"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		days := input.Body.RetentionDays
		if days < 0 || days > 365 {
			return nil, httpx.BadRequest("retensi 0 (nonaktif) sampai 365 hari")
		}
		cfg, err := d.Store.GetJobScheduleSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cfg.NotifLogRetentionDays = days
		if err := d.Store.UpsertJobScheduleSettings(ctx, tid, cfg); err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				RetentionDays int `json:"retention_days"`
			}
		}{}
		out.Body.RetentionDays = store.NormalizeJobScheduleSettings(cfg).NotifLogRetentionDays
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "purge-notification-history", Method: http.MethodPost, Path: "/api/notifications/history/purge",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			RetentionDays int `json:"retention_days"`
		}
	}) (*struct {
		Body struct {
			Deleted       int64 `json:"deleted"`
			RetentionDays int   `json:"retention_days"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		days := input.Body.RetentionDays
		if days < 1 || days > 365 {
			return nil, httpx.BadRequest("retensi hapus 1 sampai 365 hari")
		}
		n, err := d.Store.PurgeNotificationHistory(ctx, tid, days)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditNotifPurge, "notification", nil, map[string]any{
			"retention_days": days, "deleted": n,
		})
		out := &struct {
			Body struct {
				Deleted       int64 `json:"deleted"`
				RetentionDays int   `json:"retention_days"`
			}
		}{}
		out.Body.Deleted = n
		out.Body.RetentionDays = days
		return out, nil
	})
}
