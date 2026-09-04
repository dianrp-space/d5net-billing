package notify

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

type Message struct {
	TenantID  xid.ID
	Channel   string
	Recipient string
	Subject   string
	Body      string
}

type Notifier interface {
	Channel() string
	Send(ctx context.Context, msg Message) error
}

type Service struct {
	store     *store.Store
	notifiers map[string]Notifier
}

func NewService(st *store.Store) *Service {
	s := &Service{store: st, notifiers: make(map[string]Notifier)}
	s.Register(&WhatsAppNotifier{})
	s.Register(&TelegramNotifier{})
	s.Register(&EmailNotifier{})
	return s
}

func (s *Service) Register(n Notifier) {
	s.notifiers[n.Channel()] = n
}

func (s *Service) Queue(ctx context.Context, msg Message) error {
	_, err := s.store.Pool.Exec(ctx, `
		INSERT INTO notification_queue (tenant_id, channel, recipient, subject, body)
		VALUES ($1,$2,$3,$4,$5)
	`, msg.TenantID, msg.Channel, msg.Recipient, msg.Subject, msg.Body)
	return err
}

func (s *Service) ProcessPending(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.store.Pool.Query(ctx, `
		SELECT id, tenant_id, channel, recipient, subject, body
		FROM notification_queue WHERE status = 'pending' AND scheduled_at <= NOW()
		ORDER BY scheduled_at LIMIT $1 FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	sent := 0
	for rows.Next() {
		var id, tenantID xid.ID
		var channel, recipient, body string
		var subject *string
		if err := rows.Scan(&id, &tenantID, &channel, &recipient, &subject, &body); err != nil {
			continue
		}
		n, ok := s.notifiers[channel]
		if !ok {
			_, _ = s.store.Pool.Exec(ctx, `UPDATE notification_queue SET status='failed', error=$2, attempts=attempts+1 WHERE id=$1`, id, "unknown channel")
			continue
		}
		msg := Message{TenantID: tenantID, Channel: channel, Recipient: recipient, Body: body}
		if subject != nil {
			msg.Subject = *subject
		}
		if err := n.Send(ctx, msg); err != nil {
			_, _ = s.store.Pool.Exec(ctx, `UPDATE notification_queue SET status='failed', error=$2, attempts=attempts+1 WHERE id=$1`, id, err.Error())
			slog.Error("notification failed", "id", id, "err", err)
			continue
		}
		_, _ = s.store.Pool.Exec(ctx, `UPDATE notification_queue SET status='sent', sent_at=NOW() WHERE id=$1`, id)
		sent++
	}
	return sent, nil
}

// RenderTemplate loads a tenant template by channel+name (event column), or falls back to hardcoded defaults.
func (s *Service) RenderTemplate(ctx context.Context, tenantID xid.ID, channel, name string, vars map[string]string) (subject, body string, err error) {
	var subj *string
	var tplBody string
	qerr := s.store.Pool.QueryRow(ctx, `
		SELECT subject, body FROM notification_templates
		WHERE tenant_id = $1 AND channel = $2 AND event = $3 AND is_active = true
		LIMIT 1
	`, tenantID, channel, name).Scan(&subj, &tplBody)
	if qerr != nil {
		subject, body = defaultTemplate(channel, name, vars)
		return subject, body, nil
	}
	if subj != nil {
		subject = applyVars(*subj, vars)
	}
	body = applyVars(tplBody, vars)
	return subject, body, nil
}

func defaultTemplate(channel, name string, vars map[string]string) (subject, body string) {
	_ = channel
	switch name {
	case "invoice_reminder":
		body = applyVars("Halo, tagihan {{invoice_number}} sebesar Rp {{amount}} jatuh tempo {{due_date}}. Bayar via portal pelanggan.", vars)
	case "payment_confirmation":
		body = applyVars("Pembayaran tagihan {{invoice_number}} sebesar Rp {{amount}} berhasil diterima. Terima kasih!", vars)
	default:
		body = applyVars("{{message}}", vars)
	}
	return "", body
}

func applyVars(tpl string, vars map[string]string) string {
	out := tpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func (s *Service) SendInvoiceReminder(ctx context.Context, tenantID xid.ID, customerID xid.ID, phone, invoiceNum string, amount int64, dueDate string) error {
	_ = customerID
	vars := map[string]string{
		"invoice_number": invoiceNum,
		"amount":         fmt.Sprintf("%d", amount),
		"due_date":       dueDate,
	}
	_, body, err := s.RenderTemplate(ctx, tenantID, "whatsapp", "invoice_reminder", vars)
	if err != nil {
		return err
	}
	return s.Queue(ctx, Message{TenantID: tenantID, Channel: "whatsapp", Recipient: phone, Body: body})
}

func (s *Service) SendPaymentConfirmation(ctx context.Context, tenantID xid.ID, phone, invoiceNum string, amount int64) error {
	vars := map[string]string{
		"invoice_number": invoiceNum,
		"amount":         fmt.Sprintf("%d", amount),
	}
	_, body, err := s.RenderTemplate(ctx, tenantID, "whatsapp", "payment_confirmation", vars)
	if err != nil {
		return err
	}
	return s.Queue(ctx, Message{TenantID: tenantID, Channel: "whatsapp", Recipient: phone, Body: body})
}

func (s *Service) HandleWhatsAppBot(ctx context.Context, tenantID xid.ID, phone, text string) (string, error) {
	switch text {
	case "tagihan", "invoice":
		cust, err := s.store.GetCustomerByPhone(ctx, tenantID, phone)
		if err != nil {
			return "Nomor tidak terdaftar.", nil
		}
		invoices, _, err := s.store.ListInvoices(ctx, tenantID, "issued", 5, 0)
		if err != nil || len(invoices) == 0 {
			return "Tidak ada tagihan aktif.", nil
		}
		for _, inv := range invoices {
			if inv.CustomerID == cust.ID {
				return fmt.Sprintf("Tagihan %s: Rp %d, jatuh tempo %s", inv.InvoiceNumber, inv.TotalAmount, inv.DueDate.Format("02/01/2006")), nil
			}
		}
		return "Tidak ada tagihan aktif.", nil
	case "status":
		return "Layanan aktif. Cek detail di portal pelanggan.", nil
	case "gangguan", "tiket":
		_, err := s.store.Pool.Exec(ctx, `
			INSERT INTO tickets (tenant_id, subject, description, category, priority, status)
			SELECT $1, 'Laporan gangguan via WhatsApp', $2, 'outage', 'high', 'open'
		`, tenantID, "Dilaporkan via WA dari "+phone)
		if err != nil {
			return "Gagal membuat tiket.", err
		}
		return "Tiket gangguan telah dibuat. Tim kami akan segera menghubungi Anda.", nil
	default:
		return "Perintah: tagihan, status, gangguan", nil
	}
}
