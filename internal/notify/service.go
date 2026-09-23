package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/wa"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

type Message struct {
	TenantID    xid.ID
	Channel     string
	Recipient   string
	Subject     string
	Body        string
	ScheduledAt *time.Time
	BatchID     xid.ID
	// Event menandai jenis pesan (invoice_issued, invoice_reminder, payment_confirmation,
	// broadcast, ops_telegram, monthly_report) untuk riwayat di UI.
	Event string
}

type Notifier interface {
	Channel() string
	Send(ctx context.Context, msg Message) error
}

type Service struct {
	store     *store.Store
	notifiers map[string]Notifier
	decrypt   func(encoded string) (string, error)
}

func NewService(st *store.Store) *Service {
	s := &Service{store: st, notifiers: make(map[string]Notifier)}
	s.Register(&TelegramNotifier{})
	s.Register(&EmailNotifier{})
	return s
}

func (s *Service) WithDecryptor(fn func(encoded string) (string, error)) *Service {
	s.decrypt = fn
	return s
}

func (s *Service) Register(n Notifier) {
	s.notifiers[n.Channel()] = n
}

func (s *Service) Queue(ctx context.Context, msg Message) error {
	var batch any
	if !xid.IsNil(msg.BatchID) {
		batch = msg.BatchID
	}
	if msg.ScheduledAt != nil {
		_, err := s.store.Pool.Exec(ctx, `
			INSERT INTO notification_queue (tenant_id, channel, recipient, subject, body, scheduled_at, batch_id, event)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`, msg.TenantID, msg.Channel, msg.Recipient, msg.Subject, msg.Body, *msg.ScheduledAt, batch, msg.Event)
		return err
	}
	_, err := s.store.Pool.Exec(ctx, `
		INSERT INTO notification_queue (tenant_id, channel, recipient, subject, body, batch_id, event)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, msg.TenantID, msg.Channel, msg.Recipient, msg.Subject, msg.Body, batch, msg.Event)
	return err
}

// BroadcastMessage adalah satu pesan broadcast yang sudah dirender final
// untuk penerimanya (variabel seperti {{customer_name}} terisi).
type BroadcastMessage struct {
	Recipient string
	Subject   string
	Body      string
}

// BroadcastVars membangun variabel personalisasi broadcast untuk satu penerima.
// Konteksnya sama seperti template tagihan: paket + satu tagihan acuan
// (tunggakan tertua untuk audience overdue, terbaru untuk active).
func BroadcastVars(customerName, phone, planName, itemName, invoiceNumber, amount, dueDate string) map[string]string {
	item := strings.TrimSpace(itemName)
	if item == "" {
		item = strings.TrimSpace(planName)
	}
	if item == "" {
		item = "layanan"
	}
	return map[string]string{
		"customer_name":  strings.TrimSpace(customerName),
		"phone":          strings.TrimSpace(phone),
		"plan_name":      strings.TrimSpace(planName),
		"item_name":      item,
		"invoice_number": strings.TrimSpace(invoiceNumber),
		"amount":         strings.TrimSpace(amount),
		"due_date":       strings.TrimSpace(dueDate),
	}
}

// RenderBroadcastBody mengisi variabel broadcast ({{customer_name}}, {{phone}})
// untuk satu penerima.
func RenderBroadcastBody(body string, vars map[string]string) string {
	return applyVars(body, vars)
}

// QueueBroadcast enqueues many messages with staggered scheduled_at (rate limit).
// Returns queued count + batch ID for progress tracking.
func (s *Service) QueueBroadcast(ctx context.Context, tenantID xid.ID, channel, subject, body string, recipients []string, delaySeconds int, event string) (int, xid.ID, error) {
	msgs := make([]BroadcastMessage, 0, len(recipients))
	for _, r := range recipients {
		if strings.TrimSpace(r) == "" {
			continue
		}
		msgs = append(msgs, BroadcastMessage{Recipient: r, Subject: subject, Body: body})
	}
	return s.QueueBroadcastMessages(ctx, tenantID, channel, subject, msgs, delaySeconds, event)
}

// QueueBroadcastMessages sama seperti QueueBroadcast tetapi body tiap penerima
// sudah final (hasil personalisasi).
func (s *Service) QueueBroadcastMessages(ctx context.Context, tenantID xid.ID, channel, subject string, msgs []BroadcastMessage, delaySeconds int, event string) (int, xid.ID, error) {
	if delaySeconds < 1 {
		delaySeconds = 2
	}
	if delaySeconds > 60 {
		delaySeconds = 60
	}
	event = strings.TrimSpace(event)
	if event == "" {
		event = "broadcast"
	}
	batch := xid.New()
	n := 0
	base := time.Now()
	for i, m := range msgs {
		r := strings.TrimSpace(m.Recipient)
		if r == "" {
			continue
		}
		subj := m.Subject
		if subj == "" {
			subj = subject
		}
		at := base.Add(time.Duration(i*delaySeconds) * time.Second)
		if err := s.Queue(ctx, Message{
			TenantID: tenantID, Channel: channel, Recipient: r, Subject: subj, Body: m.Body, ScheduledAt: &at, BatchID: batch, Event: event,
		}); err != nil {
			return n, batch, err
		}
		n++
	}
	return n, batch, nil
}

// BroadcastProgress aggregates queue rows of one batch for live progress UI.
func (s *Service) BroadcastProgress(ctx context.Context, tenantID, batchID xid.ID) (total, pending, sent, failed int64, failures []BroadcastFailure, err error) {
	err = s.store.Pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'pending'),
		       COUNT(*) FILTER (WHERE status = 'sent'),
		       COUNT(*) FILTER (WHERE status = 'failed')
		FROM notification_queue WHERE tenant_id = $1 AND batch_id = $2
	`, tenantID, batchID).Scan(&total, &pending, &sent, &failed)
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}
	rows, err := s.store.Pool.Query(ctx, `
		SELECT recipient, COALESCE(error, '') FROM notification_queue
		WHERE tenant_id = $1 AND batch_id = $2 AND status = 'failed'
		ORDER BY created_at LIMIT 20
	`, tenantID, batchID)
	if err != nil {
		return total, pending, sent, failed, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f BroadcastFailure
		if err := rows.Scan(&f.Recipient, &f.Error); err != nil {
			continue
		}
		failures = append(failures, f)
	}
	return total, pending, sent, failed, failures, rows.Err()
}

type BroadcastFailure struct {
	Recipient string `json:"recipient"`
	Error     string `json:"error"`
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
		if !ok && channel != "whatsapp" {
			_, _ = s.store.Pool.Exec(ctx, `UPDATE notification_queue SET status='failed', error=$2, attempts=attempts+1 WHERE id=$1`, id, "unknown channel")
			continue
		}
		msg := Message{TenantID: tenantID, Channel: channel, Recipient: recipient, Body: body}
		if subject != nil {
			msg.Subject = *subject
		}
		var sendErr error
		sender := ""
		switch channel {
		case "whatsapp":
			sender, sendErr = s.sendWhatsApp(ctx, tenantID, recipient, body)
		case "telegram":
			sender = "bot"
			senderSvc := s.tenantMessagingNotifier(ctx, tenantID, "telegram")
			if senderSvc == nil {
				senderSvc = n
			}
			if chatID := s.tenantTelegramChatID(ctx, tenantID); chatID != "" {
				msg.Recipient = chatID
			}
			sendErr = senderSvc.Send(ctx, msg)
		case "email":
			sender = s.tenantEmailFrom(ctx, tenantID)
			senderSvc := s.tenantEmailNotifier(ctx, tenantID)
			if senderSvc == nil {
				senderSvc = n
			}
			sendErr = senderSvc.Send(ctx, msg)
		default:
			sendErr = fmt.Errorf("unknown channel")
		}
		if sendErr != nil {
			_, _ = s.store.Pool.Exec(ctx, `UPDATE notification_queue SET status='failed', error=$2, attempts=attempts+1 WHERE id=$1`, id, sendErr.Error())
			slog.Error("notification failed", "id", id, "err", sendErr)
			continue
		}
		_, _ = s.store.Pool.Exec(ctx, `UPDATE notification_queue SET status='sent', sent_at=NOW(), sender=NULLIF($2,'') WHERE id=$1`, id, sender)
		sent++
	}
	return sent, nil
}

// SendTest delivers a one-off message synchronously so admins can verify a
// tenant's gateway configuration. It mirrors ProcessPending channel resolution
// and returns an explicit error when the channel is not ready.
func (s *Service) SendTest(ctx context.Context, tenantID xid.ID, channel, recipient, subject, body string) error {
	channel = strings.ToLower(strings.TrimSpace(channel))
	recipient = strings.TrimSpace(recipient)
	if strings.TrimSpace(body) == "" {
		body = "Tes notifikasi D5Net — gateway berfungsi."
	}
	switch channel {
	case "whatsapp":
		if recipient == "" {
			return fmt.Errorf("nomor WhatsApp tujuan wajib diisi")
		}
		_, err := s.sendWhatsApp(ctx, tenantID, recipient, body)
		return err
	case "telegram":
		n := s.tenantMessagingNotifier(ctx, tenantID, "telegram")
		if n == nil {
			return fmt.Errorf("Telegram belum diaktifkan/dikonfigurasi (bot token + chat ID)")
		}
		if recipient == "" {
			recipient = s.tenantTelegramChatID(ctx, tenantID)
		}
		if recipient == "" {
			return fmt.Errorf("chat ID Telegram wajib diisi")
		}
		return n.Send(ctx, Message{TenantID: tenantID, Channel: channel, Recipient: recipient, Subject: subject, Body: body})
	case "email":
		n := s.tenantEmailNotifier(ctx, tenantID)
		if n == nil {
			return fmt.Errorf("SMTP tenant belum diaktifkan/dikonfigurasi")
		}
		if recipient == "" {
			return fmt.Errorf("alamat email tujuan wajib diisi")
		}
		return n.Send(ctx, Message{TenantID: tenantID, Channel: channel, Recipient: recipient, Subject: subject, Body: body})
	default:
		return fmt.Errorf("channel tidak dikenal: %s", channel)
	}
}

type waDeviceCfg struct {
	DeviceID string `json:"device_id"`
	Label    string `json:"label"`
	Priority int    `json:"priority"`
}

type tenantMessagingCfg struct {
	TelegramBotToken string        `json:"telegram_bot_token"`
	TelegramChatID   string        `json:"telegram_chat_id"`
	TelegramEnabled  bool          `json:"telegram_enabled"`
	WhatsAppEnabled  bool          `json:"whatsapp_enabled"`
	WhatsAppBaseURL  string        `json:"whatsapp_base_url"`
	WhatsAppUsername string        `json:"whatsapp_username"`
	WhatsAppPassword string        `json:"whatsapp_password"`
	WhatsAppDeviceID string        `json:"whatsapp_device_id"` // legacy single device
	WhatsAppDevices  []waDeviceCfg `json:"whatsapp_devices"`
}

// tenantWhatsAppClients builds one gateway client per configured device
// (same base URL + Basic Auth), for failover across multiple numbers. Returns
// nil when WhatsApp is disabled/unconfigured.
func (s *Service) tenantWhatsAppClients(ctx context.Context, tenantID xid.ID) []*wa.Client {
	cfg, ok := s.loadTenantMessaging(ctx, tenantID)
	if !ok || !cfg.WhatsAppEnabled {
		return nil
	}
	base := strings.TrimSpace(cfg.WhatsAppBaseURL)
	if base == "" {
		return nil
	}
	pass := cfg.WhatsAppPassword
	if pass != "" && s.decrypt != nil {
		if plain, err := s.decrypt(pass); err == nil {
			pass = plain
		} else {
			pass = ""
		}
	}
	devices := cfg.WhatsAppDevices
	if len(devices) == 0 {
		if id := strings.TrimSpace(cfg.WhatsAppDeviceID); id != "" {
			devices = []waDeviceCfg{{DeviceID: id}}
		} else {
			devices = []waDeviceCfg{{}}
		}
	}
	sort.SliceStable(devices, func(i, j int) bool { return devices[i].Priority < devices[j].Priority })
	clients := make([]*wa.Client, 0, len(devices))
	for _, dev := range devices {
		clients = append(clients, wa.NewClient(wa.Config{
			BaseURL:  base,
			Username: strings.TrimSpace(cfg.WhatsAppUsername),
			Password: pass,
			DeviceID: strings.TrimSpace(dev.DeviceID),
		}))
	}
	return clients
}

// sendWhatsApp tries each configured number until one succeeds (redundancy).
// Mengembalikan device ID pengirim yang berhasil ("" = default gateway).
func (s *Service) sendWhatsApp(ctx context.Context, tenantID xid.ID, phone, body string) (string, error) {
	clients := s.tenantWhatsAppClients(ctx, tenantID)
	if len(clients) == 0 {
		return "", fmt.Errorf("WhatsApp gateway belum dikonfigurasi/aktif untuk tenant")
	}
	var errs []string
	for _, c := range clients {
		if err := c.SendText(ctx, phone, body); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		return c.DeviceID(), nil
	}
	return "", fmt.Errorf("semua nomor WhatsApp gagal: %s", strings.Join(errs, "; "))
}

// tenantEmailFrom mengembalikan alamat From SMTP tenant ("" bila tak dikonfigurasi).
func (s *Service) tenantEmailFrom(ctx context.Context, tenantID xid.ID) string {
	var cfg tenantSMTPCfg
	if err := s.store.GetSettingJSON(ctx, tenantID, "integration.smtp", &cfg); err != nil {
		return ""
	}
	if !cfg.Enabled {
		return ""
	}
	return strings.TrimSpace(cfg.From)
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

// TemplateVariable describes a placeholder usable in a template body.
type TemplateVariable struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// TemplateEvent describes an automatic notification event and its default copy.
type TemplateEvent struct {
	Event          string             `json:"event"`
	Label          string             `json:"label"`
	Description    string             `json:"description"`
	Channels       []string           `json:"channels"`
	Variables      []TemplateVariable `json:"variables"`
	DefaultSubject string             `json:"default_subject"`
	DefaultBody    string             `json:"default_body"`
}

// TemplateCatalog lists the notification events a tenant can customise.
func TemplateCatalog() []TemplateEvent {
	return []TemplateEvent{
		{
			Event:       "invoice_issued",
			Label:       "Tagihan baru (manual)",
			Description: "Dikirim otomatis saat admin menerbitkan tagihan manual.",
			Channels:    []string{"whatsapp"},
			Variables: []TemplateVariable{
				{Name: "customer_name", Desc: "Nama pelanggan"},
				{Name: "plan_name", Desc: "Nama paket/langganan"},
				{Name: "item_name", Desc: "Nama item tagihan (baris invoice, tanpa denda)"},
				{Name: "invoice_number", Desc: "Nomor tagihan"},
				{Name: "amount", Desc: "Nominal tagihan (angka)"},
				{Name: "due_date", Desc: "Tanggal jatuh tempo"},
			},
			DefaultBody: "Halo {{customer_name}}, tagihan baru {{item_name}} ({{invoice_number}}) sebesar Rp {{amount}} telah diterbitkan. Jatuh tempo {{due_date}}. Bayar via portal pelanggan.",
		},
		{
			Event:       "invoice_generated",
			Label:       "Tagihan langganan baru (otomatis)",
			Description: "Dikirim otomatis saat tagihan langganan terbit (aktivasi pertama & tagihan rutin).",
			Channels:    []string{"whatsapp"},
			Variables: []TemplateVariable{
				{Name: "customer_name", Desc: "Nama pelanggan"},
				{Name: "plan_name", Desc: "Nama paket/langganan"},
				{Name: "item_name", Desc: "Nama item tagihan (baris invoice, tanpa denda)"},
				{Name: "invoice_number", Desc: "Nomor tagihan"},
				{Name: "amount", Desc: "Nominal tagihan (angka)"},
				{Name: "due_date", Desc: "Tanggal jatuh tempo"},
			},
			DefaultBody: "Halo {{customer_name}}, tagihan {{item_name}} ({{invoice_number}}) sebesar Rp {{amount}} telah diterbitkan. Jatuh tempo {{due_date}}. Bayar via portal pelanggan.",
		},
		{
			Event:       "invoice_reminder",
			Label:       "Pengingat tagihan (dunning)",
			Description: "Dikirim otomatis sesuai offset hari di menu Cronjob.",
			Channels:    []string{"whatsapp"},
			Variables: []TemplateVariable{
				{Name: "customer_name", Desc: "Nama pelanggan"},
				{Name: "plan_name", Desc: "Nama paket/langganan"},
				{Name: "item_name", Desc: "Nama item tagihan (baris invoice, tanpa denda)"},
				{Name: "invoice_number", Desc: "Nomor tagihan"},
				{Name: "amount", Desc: "Nominal tagihan (angka)"},
				{Name: "due_date", Desc: "Tanggal jatuh tempo"},
			},
			DefaultBody: "Halo {{customer_name}}, tagihan {{item_name}} ({{invoice_number}}) sebesar Rp {{amount}} jatuh tempo {{due_date}}. Bayar via portal pelanggan.",
		},
		{
			Event:       "payment_confirmation",
			Label:       "Konfirmasi pembayaran",
			Description: "Dikirim otomatis setelah pembayaran diterima.",
			Channels:    []string{"whatsapp"},
			Variables: []TemplateVariable{
				{Name: "customer_name", Desc: "Nama pelanggan"},
				{Name: "plan_name", Desc: "Nama paket/langganan"},
				{Name: "item_name", Desc: "Nama item tagihan (baris invoice, tanpa denda)"},
				{Name: "invoice_number", Desc: "Nomor tagihan"},
				{Name: "amount", Desc: "Nominal dibayar (angka)"},
			},
			DefaultBody: "Terima kasih {{customer_name}}! Pembayaran {{item_name}} ({{invoice_number}}) sebesar Rp {{amount}} berhasil diterima.",
		},
		{
			Event:       "wallet_topup",
			Label:       "Topup saldo berhasil",
			Description: "Dikirim setelah saldo pelanggan berhasil ditambah.",
			Channels:    []string{"whatsapp"},
			Variables: []TemplateVariable{
				{Name: "customer_name", Desc: "Nama pelanggan"},
				{Name: "amount", Desc: "Nominal topup (angka)"},
				{Name: "balance", Desc: "Saldo terbaru (angka)"},
			},
			DefaultBody: "Terima kasih {{customer_name}}! Topup saldo Rp {{amount}} berhasil. Saldo Anda sekarang Rp {{balance}}.",
		},
		{
			Event:       "wallet_insufficient",
			Label:       "Saldo kurang saat tagihan terbit",
			Description: "Dikirim saat tagihan terbit tapi saldo tidak cukup untuk dibayar otomatis.",
			Channels:    []string{"whatsapp"},
			Variables: []TemplateVariable{
				{Name: "customer_name", Desc: "Nama pelanggan"},
				{Name: "invoice_number", Desc: "Nomor tagihan"},
				{Name: "amount", Desc: "Nominal tagihan (angka)"},
				{Name: "balance", Desc: "Saldo saat ini (angka)"},
			},
			DefaultBody: "Halo {{customer_name}}, tagihan {{invoice_number}} sebesar Rp {{amount}} belum bisa dibayar otomatis karena saldo Anda Rp {{balance}} kurang. Topup saldo atau bayar via portal pelanggan.",
		},
		{
			Event:       "broadcast",
			Label:       "Broadcast manual",
			Description: "Dipakai untuk pesan massal dari tab Broadcast.",
			Channels:    []string{"whatsapp", "telegram", "email"},
			Variables: []TemplateVariable{
				{Name: "customer_name", Desc: "Nama pelanggan"},
				{Name: "phone", Desc: "Nomor penerima"},
				{Name: "plan_name", Desc: "Paket langganan (terbaru)"},
				{Name: "item_name", Desc: "Item tagihan acuan"},
				{Name: "invoice_number", Desc: "Nomor tagihan acuan"},
				{Name: "amount", Desc: "Sisa tagihan acuan (angka)"},
				{Name: "due_date", Desc: "Jatuh tempo tagihan acuan"},
			},
			DefaultBody: "Halo {{customer_name}}! ",
		},
	}
}

func defaultTemplate(channel, name string, vars map[string]string) (subject, body string) {
	for _, ev := range TemplateCatalog() {
		if ev.Event == name {
			return applyVars(ev.DefaultSubject, vars), applyVars(ev.DefaultBody, vars)
		}
	}
	return "", applyVars("{{message}}", vars)
}

func applyVars(tpl string, vars map[string]string) string {
	out := tpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func notificationItemName(planName, itemName string) string {
	if n := strings.TrimSpace(itemName); n != "" {
		return n
	}
	if p := strings.TrimSpace(planName); p != "" {
		return p
	}
	return "layanan"
}

func (s *Service) SendInvoiceIssued(ctx context.Context, tenantID xid.ID, phone, customerName, planName, itemName, invoiceNum string, amount int64, dueDate string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil
	}
	vars := map[string]string{
		"customer_name":  customerName,
		"plan_name":      planName,
		"item_name":      notificationItemName(planName, itemName),
		"invoice_number": invoiceNum,
		"amount":         fmt.Sprintf("%d", amount),
		"due_date":       dueDate,
	}
	_, body, err := s.RenderTemplate(ctx, tenantID, "whatsapp", "invoice_issued", vars)
	if err != nil {
		return err
	}
	return s.Queue(ctx, Message{TenantID: tenantID, Channel: "whatsapp", Recipient: phone, Body: body, Event: "invoice_issued", ScheduledAt: s.invoiceIssuedScheduledAt(ctx, tenantID)})
}

// SendInvoiceGenerated notifies the customer when a subscription invoice is
// generated automatically (first invoice on activation and routine billing).
// The message is delayed until the tenant's configured tagihan-terbit time
// when created before that time today.
func (s *Service) SendInvoiceGenerated(ctx context.Context, tenantID xid.ID, phone, customerName, planName, itemName, invoiceNum string, amount int64, dueDate string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil
	}
	vars := map[string]string{
		"customer_name":  customerName,
		"plan_name":      planName,
		"item_name":      notificationItemName(planName, itemName),
		"invoice_number": invoiceNum,
		"amount":         fmt.Sprintf("%d", amount),
		"due_date":       dueDate,
	}
	_, body, err := s.RenderTemplate(ctx, tenantID, "whatsapp", "invoice_generated", vars)
	if err != nil {
		return err
	}
	return s.Queue(ctx, Message{TenantID: tenantID, Channel: "whatsapp", Recipient: phone, Body: body, Event: "invoice_generated", ScheduledAt: s.invoiceIssuedScheduledAt(ctx, tenantID)})
}

// invoiceIssuedScheduledAt returns today at the tenant's configured
// tagihan-terbit time when still in the future, else nil (send immediately).
// Jam memakai Timezone tenant (Pengaturan → Umum), bukan jam server.
func (s *Service) invoiceIssuedScheduledAt(ctx context.Context, tenantID xid.ID) *time.Time {
	hhmm := "08:00"
	if cfg, err := s.store.GetJobScheduleSettings(ctx, tenantID); err == nil {
		hhmm = store.NormalizeNotifyTime(cfg.InvoiceIssuedTime, hhmm)
	}
	return store.ScheduledAtForNotifyTime(s.store.TenantNow(ctx, tenantID), hhmm)
}

func (s *Service) SendInvoiceReminder(ctx context.Context, tenantID xid.ID, phone, customerName, planName, itemName, invoiceNum string, amount int64, dueDate string) error {
	vars := map[string]string{
		"customer_name":  customerName,
		"plan_name":      planName,
		"item_name":      notificationItemName(planName, itemName),
		"invoice_number": invoiceNum,
		"amount":         fmt.Sprintf("%d", amount),
		"due_date":       dueDate,
	}
	_, body, err := s.RenderTemplate(ctx, tenantID, "whatsapp", "invoice_reminder", vars)
	if err != nil {
		return err
	}
	return s.Queue(ctx, Message{TenantID: tenantID, Channel: "whatsapp", Recipient: phone, Body: body, Event: "invoice_reminder"})
}

func (s *Service) SendPaymentConfirmation(ctx context.Context, tenantID xid.ID, phone, customerName, planName, itemName, invoiceNum string, amount int64) error {
	vars := map[string]string{
		"customer_name":  customerName,
		"plan_name":      planName,
		"item_name":      notificationItemName(planName, itemName),
		"invoice_number": invoiceNum,
		"amount":         fmt.Sprintf("%d", amount),
	}
	_, body, err := s.RenderTemplate(ctx, tenantID, "whatsapp", "payment_confirmation", vars)
	if err != nil {
		return err
	}
	return s.Queue(ctx, Message{TenantID: tenantID, Channel: "whatsapp", Recipient: phone, Body: body, Event: "payment_confirmation"})
}

func (s *Service) SendWalletTopup(ctx context.Context, tenantID xid.ID, phone, customerName string, amount, balance int64) error {
	if strings.TrimSpace(phone) == "" {
		return nil
	}
	vars := map[string]string{
		"customer_name": customerName,
		"amount":        fmt.Sprintf("%d", amount),
		"balance":       fmt.Sprintf("%d", balance),
	}
	_, body, err := s.RenderTemplate(ctx, tenantID, "whatsapp", "wallet_topup", vars)
	if err != nil {
		return err
	}
	return s.Queue(ctx, Message{TenantID: tenantID, Channel: "whatsapp", Recipient: phone, Body: body, Event: "wallet_topup"})
}

func (s *Service) SendWalletInsufficient(ctx context.Context, tenantID xid.ID, phone, customerName, invoiceNum string, amount, balance int64) error {
	if strings.TrimSpace(phone) == "" {
		return nil
	}
	vars := map[string]string{
		"customer_name":  customerName,
		"invoice_number": invoiceNum,
		"amount":         fmt.Sprintf("%d", amount),
		"balance":        fmt.Sprintf("%d", balance),
	}
	_, body, err := s.RenderTemplate(ctx, tenantID, "whatsapp", "wallet_insufficient", vars)
	if err != nil {
		return err
	}
	return s.Queue(ctx, Message{TenantID: tenantID, Channel: "whatsapp", Recipient: phone, Body: body, Event: "wallet_insufficient"})
}
