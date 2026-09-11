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
		OperationID: "broadcast-notifications", Method: http.MethodPost, Path: "/api/notifications/broadcast",
		Tags: []string{"Notifications"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Channel       string   `json:"channel"`
			Audience      string   `json:"audience"` // overdue | active | custom
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
		if input.Body.TemplateEvent != "" {
			tpl, err := d.Store.GetNotificationTemplate(ctx, tid, ch, input.Body.TemplateEvent)
			if err == nil && tpl != nil {
				body = tpl.Body
				if tpl.Subject != nil && input.Body.Subject == "" {
					input.Body.Subject = *tpl.Subject
				}
			}
		}
		if body == "" {
			return nil, httpx.BadRequest("body atau template_event wajib")
		}
		var recipients []string
		switch strings.TrimSpace(input.Body.Audience) {
		case "custom":
			recipients = input.Body.Recipients
		case "overdue", "active", "":
			aud := input.Body.Audience
			if aud == "" {
				aud = "active"
			}
			recipients, err = d.Store.ListBroadcastPhones(ctx, tid, aud)
			if err != nil {
				return nil, httpx.Internal(err)
			}
		default:
			return nil, httpx.BadRequest("audience harus overdue, active, atau custom")
		}
		if len(recipients) == 0 {
			return nil, httpx.BadRequest("tidak ada penerima")
		}
		n, batch, err := d.Notify.QueueBroadcast(ctx, tid, ch, input.Body.Subject, body, recipients, input.Body.DelaySeconds)
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
}
