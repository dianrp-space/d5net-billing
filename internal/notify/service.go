package notify

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

type Message struct {
	TenantID    xid.ID
	Channel     string
	Recipient   string
	Subject     string
	Body        string
	ScheduledAt *time.Time
}

type Notifier interface {
	Channel() string
	Send(ctx context.Context, msg Message) error
}

type Service struct {
	store     *store.Store
	notifiers map[string]Notifier
	decrypt   func(encoded string) (string, error)
	wa        WhatsAppSender
}

func NewService(st *store.Store) *Service {
	s := &Service{store: st, notifiers: make(map[string]Notifier)}
	s.Register(&WhatsAppNotifier{})
	s.Register(&TelegramNotifier{})
	s.Register(&EmailNotifier{})
	return s
}

func (s *Service) WithDecryptor(fn func(encoded string) (string, error)) *Service {
	s.decrypt = fn
	return s
}

func (s *Service) WithWhatsApp(sender WhatsAppSender) *Service {
	s.wa = sender
	return s
}

type WhatsAppSender interface {
	IsReady(tenantID xid.ID) bool
	SendText(ctx context.Context, tenantID xid.ID, phone, body string) error
}

func (s *Service) Register(n Notifier) {
	s.notifiers[n.Channel()] = n
}

func (s *Service) Queue(ctx context.Context, msg Message) error {
	if msg.ScheduledAt != nil {
		_, err := s.store.Pool.Exec(ctx, `
			INSERT INTO notification_queue (tenant_id, channel, recipient, subject, body, scheduled_at)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, msg.TenantID, msg.Channel, msg.Recipient, msg.Subject, msg.Body, *msg.ScheduledAt)
		return err
	}
	_, err := s.store.Pool.Exec(ctx, `
		INSERT INTO notification_queue (tenant_id, channel, recipient, subject, body)
		VALUES ($1,$2,$3,$4,$5)
	`, msg.TenantID, msg.Channel, msg.Recipient, msg.Subject, msg.Body)
	return err
}

// QueueBroadcast enqueues many messages with staggered scheduled_at (rate limit).
func (s *Service) QueueBroadcast(ctx context.Context, tenantID xid.ID, channel, subject, body string, recipients []string, delaySeconds int) (int, error) {
	if delaySeconds < 1 {
		delaySeconds = 2
	}
	if delaySeconds > 60 {
		delaySeconds = 60
	}
	n := 0
	base := time.Now()
	for i, r := range recipients {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		at := base.Add(time.Duration(i*delaySeconds) * time.Second)
		if err := s.Queue(ctx, Message{
			TenantID: tenantID, Channel: channel, Recipient: r, Subject: subject, Body: body, ScheduledAt: &at,
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
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
		var sendErr error
		if channel == "whatsapp" && s.wa != nil && s.wa.IsReady(tenantID) {
			sendErr = s.wa.SendText(ctx, tenantID, recipient, body)
		} else {
			sender := n
			if channel == "whatsapp" || channel == "telegram" {
				if overlay := s.tenantMessagingNotifier(ctx, tenantID, channel); overlay != nil {
					sender = overlay
				}
			}
			if channel == "email" {
				if overlay := s.tenantEmailNotifier(ctx, tenantID); overlay != nil {
					sender = overlay
				}
			}
			if channel == "telegram" {
				if chatID := s.tenantTelegramChatID(ctx, tenantID); chatID != "" {
					msg.Recipient = chatID
				}
			}
			sendErr = sender.Send(ctx, msg)
		}
		if sendErr != nil {
			_, _ = s.store.Pool.Exec(ctx, `UPDATE notification_queue SET status='failed', error=$2, attempts=attempts+1 WHERE id=$1`, id, sendErr.Error())
			slog.Error("notification failed", "id", id, "err", sendErr)
			continue
		}
		_, _ = s.store.Pool.Exec(ctx, `UPDATE notification_queue SET status='sent', sent_at=NOW() WHERE id=$1`, id)
		sent++
	}
	return sent, nil
}

type tenantMessagingCfg struct {
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramChatID   string `json:"telegram_chat_id"`
	TelegramEnabled  bool   `json:"telegram_enabled"`
}

func (s *Service) loadTenantMessaging(ctx context.Context, tenantID xid.ID) (tenantMessagingCfg, bool) {
	var cfg tenantMessagingCfg
	if err := s.store.GetSettingJSON(ctx, tenantID, "integration.messaging", &cfg); err != nil {
		return cfg, false
	}
	return cfg, true
}

func (s *Service) tenantTelegramChatID(ctx context.Context, tenantID xid.ID) string {
	cfg, ok := s.loadTenantMessaging(ctx, tenantID)
	if !ok || !cfg.TelegramEnabled {
		return ""
	}
	return strings.TrimSpace(cfg.TelegramChatID)
}

func (s *Service) tenantMessagingNotifier(ctx context.Context, tenantID xid.ID, channel string) Notifier {
	cfg, ok := s.loadTenantMessaging(ctx, tenantID)
	if !ok {
		return nil
	}
	dec := func(v string) string {
		if v == "" || s.decrypt == nil {
			return v
		}
		plain, err := s.decrypt(v)
		if err != nil {
			return ""
		}
		return plain
	}
	switch channel {
	case "telegram":
		if !cfg.TelegramEnabled {
			return nil
		}
		token := dec(cfg.TelegramBotToken)
		if token == "" || strings.TrimSpace(cfg.TelegramChatID) == "" {
			return nil
		}
		return &TelegramNotifier{BotToken: token}
	default:
		return nil
	}
}

type tenantSMTPCfg struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	FromName string `json:"from_name"`
	Enabled  bool   `json:"enabled"`
}

func (s *Service) tenantEmailNotifier(ctx context.Context, tenantID xid.ID) Notifier {
	var cfg tenantSMTPCfg
	if err := s.store.GetSettingJSON(ctx, tenantID, "integration.smtp", &cfg); err != nil {
		return nil
	}
	if !cfg.Enabled {
		return nil
	}
	host := strings.TrimSpace(cfg.Host)
	from := strings.TrimSpace(cfg.From)
	if host == "" || from == "" {
		return nil
	}
	port := cfg.Port
	if port < 1 || port > 65535 {
		port = 587
	}
	pass := cfg.Password
	if pass != "" && s.decrypt != nil {
		plain, err := s.decrypt(pass)
		if err != nil {
			slog.Warn("tenant smtp password decrypt failed", "tenant_id", tenantID)
			pass = ""
		} else {
			pass = plain
		}
	}
	return &EmailNotifier{
		Host:     host,
		Port:     fmt.Sprintf("%d", port),
		User:     strings.TrimSpace(cfg.Username),
		Pass:     pass,
		From:     from,
		FromName: strings.TrimSpace(cfg.FromName),
	}
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
		invoices, _, err := s.store.ListInvoices(ctx, tenantID, "issued", "", false, 5, 0)
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
		var custID *xid.ID
		lines := []string{"Dari: " + phone, "Prioritas: high", "Kategori: outage"}
		if cust, lookupErr := s.store.GetCustomerByPhone(ctx, tenantID, phone); lookupErr == nil && cust != nil {
			id := cust.ID
			custID = &id
			who := strings.TrimSpace(cust.FullName)
			if who == "" {
				who = cust.CustomerCode
			}
			if who != "" {
				lines = append(lines, "Pelanggan: "+who)
			}
		}
		_ = s.QueueTicketTelegram(ctx, tenantID, custID, "Laporan gangguan via WhatsApp", lines...)
		return "Tiket gangguan telah dibuat. Tim kami akan segera menghubungi Anda.", nil
	default:
		return "Perintah: tagihan, status, gangguan", nil
	}
}
