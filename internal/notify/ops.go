package notify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
)

const opsTelegramJob = "ops_telegram"

// OpsMsg formats a short ops alert for Telegram.
func OpsMsg(kind, title string, lines ...string) string {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind == "" {
		kind = "OPS"
	}
	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(kind)
	b.WriteString("] ")
	b.WriteString(strings.TrimSpace(title))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteByte('\n')
		b.WriteString(line)
	}
	return b.String()
}

func OpsLabel(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "—"
	}
	return v
}

// OpsNetworkLines is cluster + router for ops Telegram (empty values become —).
func OpsNetworkLines(cluster, router string) []string {
	return []string{
		"Cluster: " + OpsLabel(cluster),
		"Router: " + OpsLabel(router),
	}
}

// QueueTicketTelegram sends a ticket ops alert and appends the customer's cluster/router.
func (s *Service) QueueTicketTelegram(ctx context.Context, tenantID xid.ID, customerID *xid.ID, subject string, lines ...string) error {
	if s != nil && s.store != nil && customerID != nil && !xid.IsNil(*customerID) {
		cluster, router, _ := s.store.CustomerNetworkLabels(ctx, tenantID, *customerID)
		copied := make([]string, 0, len(lines)+2)
		copied = append(copied, lines...)
		copied = append(copied, OpsNetworkLines(cluster, router)...)
		lines = copied
	}
	return s.QueueTenantTelegram(ctx, tenantID, OpsMsg("tiket", subject, lines...))
}

// QueueTenantTelegram sends an ops alert to the tenant's configured Telegram chat.
// No-op (nil) when Telegram is disabled or not configured.
func (s *Service) QueueTenantTelegram(ctx context.Context, tenantID xid.ID, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	chatID := s.tenantTelegramChatID(ctx, tenantID)
	if chatID == "" {
		return nil
	}
	return s.Queue(ctx, Message{TenantID: tenantID, Channel: "telegram", Recipient: chatID, Body: body})
}

// QueueTenantTelegramOnce sends at most one ops Telegram for (kind, key) per tenant.
func (s *Service) QueueTenantTelegramOnce(ctx context.Context, tenantID xid.ID, kind, key, body string) error {
	if s.tenantTelegramChatID(ctx, tenantID) == "" {
		return nil
	}
	kind = strings.TrimSpace(kind)
	key = strings.TrimSpace(key)
	if kind == "" || key == "" {
		return s.QueueTenantTelegram(ctx, tenantID, body)
	}
	ok, err := s.store.ClaimJob(ctx, tenantID, opsTelegramJob+":"+kind, key)
	if err != nil || !ok {
		return err
	}
	return s.QueueTenantTelegram(ctx, tenantID, body)
}

func OpsDayKey(id xid.ID) string {
	return fmt.Sprintf("%s-%s", id.String(), time.Now().Format("2006-01-02"))
}

func OpsWeekKey(id xid.ID, now time.Time) string {
	y, w := now.ISOWeek()
	return fmt.Sprintf("%s-%d-W%02d", id.String(), y, w)
}
