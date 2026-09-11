package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/skip2/go-qrcode"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/wa"
	"github.com/dianrp/drp-billing/internal/xid"
)

// WhatsApp customer bot.
//
// Incoming messages arrive from the GOWA gateway webhook (event "message").
// The bot only reacts to /tagihan, /link, and /qris sent by a registered,
// active customer that has an unpaid bill. Everything else stays silent.
// All replies are pushed straight from the configured bot number; the webhook
// itself always answers with an empty reply.

type waBotIncoming struct {
	// DeviceJID is the GOWA top-level device_id (receiving number JID).
	DeviceJID string
	// SessionID is the GOWA top-level session_id (device slot id).
	SessionID string
	// From is the sender JID (payload.from).
	From string
	// ChatID is the conversation JID (payload.chat_id).
	ChatID string
	Body   string
	FromMe bool
}

// whatsappWebhookInput accepts the native GOWA event payload as well as the
// legacy {tenant_id, phone, message} shape.
type whatsappWebhookInput struct {
	TenantID  xid.ID         `json:"tenant_id,omitempty"`
	Phone     string         `json:"phone,omitempty"`
	Message   string         `json:"message,omitempty"`
	Event     string         `json:"event,omitempty"`
	DeviceID  string         `json:"device_id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func parseGOWAMessage(input *whatsappWebhookInput) (waBotIncoming, bool) {
	var in waBotIncoming
	if !strings.EqualFold(strings.TrimSpace(input.Event), "message") {
		return in, false
	}
	in.DeviceJID = strings.TrimSpace(input.DeviceID)
	in.SessionID = strings.TrimSpace(input.SessionID)
	if input.Payload == nil {
		return in, false
	}
	str := func(v any) string {
		s, _ := v.(string)
		return strings.TrimSpace(s)
	}
	in.From = str(input.Payload["from"])
	in.ChatID = str(input.Payload["chat_id"])
	if in.ChatID == "" {
		in.ChatID = in.From
	}
	in.Body = str(input.Payload["body"])
	if b, ok := input.Payload["is_from_me"].(bool); ok {
		in.FromMe = b
	}
	return in, true
}

// botDeviceAllows drops events received on a different number than the
// configured bot number. Without device info (single-number setups) everything passes.
func botDeviceAllows(botDevice, deviceJID, sessionID string) bool {
	want := strings.TrimSpace(botDevice)
	if want == "" {
		return true
	}
	for _, cand := range []string{deviceJID, sessionID} {
		c := strings.TrimSpace(cand)
		if c == "" {
			continue
		}
		if strings.EqualFold(c, want) {
			return true
		}
		if digitsEqualJID(c, want) {
			return true
		}
	}
	// No device info at all: assume the (single) bot number received it.
	return deviceJID == "" && sessionID == ""
}

func digitsEqualJID(a, b string) bool {
	da, db := onlyDigits(a), onlyDigits(b)
	return da != "" && da == db
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// botPhoneCandidates covers both stored formats (08… and 62…).
func botPhoneCandidates(jid string) []string {
	digits := onlyDigits(jid)
	if digits == "" {
		return nil
	}
	out := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		for _, e := range out {
			if e == v {
				return
			}
		}
		out = append(out, v)
	}
	add(wa.NormalizePhone(digits))
	if strings.HasPrefix(digits, "62") && len(digits) > 2 {
		add("0" + digits[2:])
	}
	add(digits)
	return out
}

func matchBotCustomers(ctx context.Context, d *Deps, tid xid.ID, senderJID string) []*store.Customer {
	seen := map[xid.ID]struct{}{}
	var matched []*store.Customer
	for _, cand := range botPhoneCandidates(senderJID) {
		list, err := d.Store.ListCustomersByPhone(ctx, tid, cand)
		if err != nil {
			continue
		}
		for i := range list {
			c := &list[i]
			if !c.IsActive {
				continue
			}
			if _, ok := seen[c.ID]; ok {
				continue
			}
			seen[c.ID] = struct{}{}
			matched = append(matched, c)
		}
	}
	return matched
}

func unpaidBotInvoices(ctx context.Context, d *Deps, tid xid.ID, custs []*store.Customer) []store.Invoice {
	byID := make(map[xid.ID]*store.Customer, len(custs))
	for _, c := range custs {
		if c != nil {
			byID[c.ID] = c
		}
	}
	all := collectPortalInvoices(ctx, d, tid, byID)
	out := make([]store.Invoice, 0, len(all))
	for _, inv := range all {
		st := strings.ToLower(strings.TrimSpace(inv.Status))
		if st == "paid" || st == "void" || st == "cancelled" {
			continue
		}
		if invoiceRemaining(&inv) <= 0 {
			continue
		}
		out = append(out, inv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DueDate.Before(out[j].DueDate) })
	return out
}

func formatRupiahID(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if neg {
		return "Rp -" + b.String()
	}
	return "Rp " + b.String()
}

// botClientForTenant builds a gateway client pinned to the configured bot number.
func botClientForTenant(ctx context.Context, d *Deps, tid xid.ID) (*wa.Client, error) {
	cfg, err := loadMessagingIntegration(ctx, d, tid)
	if err != nil {
		return nil, err
	}
	if !cfg.WhatsAppEnabled {
		return nil, fmt.Errorf("WhatsApp gateway belum diaktifkan")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.WhatsAppBaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("WhatsApp gateway belum dikonfigurasi")
	}
	device := strings.TrimSpace(cfg.WhatsAppBotDeviceID)
	if device == "" {
		devs := effectiveWADevices(cfg)
		if len(devs) > 0 {
			device = strings.TrimSpace(devs[0].DeviceID)
		}
	}
	return wa.NewClient(wa.Config{
		BaseURL:  base,
		Username: strings.TrimSpace(cfg.WhatsAppUsername),
		Password: decryptSecret(d, cfg.WhatsAppPassword),
		DeviceID: device,
	}), nil
}

func botSettings(ctx context.Context, d *Deps, tid xid.ID) (enabled bool, botDevice string) {
	cfg, err := loadMessagingIntegration(ctx, d, tid)
	if err != nil {
		return false, ""
	}
	if !cfg.WhatsAppEnabled {
		return false, ""
	}
	return cfg.WhatsAppBotEnabled, strings.TrimSpace(cfg.WhatsAppBotDeviceID)
}

func handleWhatsAppBotMessage(ctx context.Context, d *Deps, tid xid.ID, ten *store.Tenant, in waBotIncoming) {
	if in.FromMe || strings.TrimSpace(in.From) == "" || strings.TrimSpace(in.Body) == "" {
		return
	}
	if strings.HasSuffix(strings.ToLower(in.ChatID), "@g.us") {
		return
	}
	enabled, botDevice := botSettings(ctx, d, tid)
	if !enabled {
		return
	}
	if !botDeviceAllows(botDevice, in.DeviceJID, in.SessionID) {
		return
	}
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(in.Body)))
	if len(fields) == 0 {
		return
	}
	var cmd string
	switch fields[0] {
	case "/tagihan", "/link", "/qris":
		cmd = fields[0]
	default:
		return // selain command itu bot tidak membalas
	}

	client, err := botClientForTenant(ctx, d, tid)
	if err != nil {
		slog.Warn("wa bot: gateway tidak siap", "tenant_id", tid, "err", err)
		return
	}
	phone := in.From
	appName := ten.Name

	custs := matchBotCustomers(ctx, d, tid, phone)
	if len(custs) == 0 {
		_ = client.SendText(ctx, phone,
			"Halo, nomor WhatsApp ini belum terdaftar sebagai pelanggan "+appName+".\nHubungi admin untuk pendaftaran.")
		return
	}
	name := strings.TrimSpace(custs[0].FullName)
	if name == "" {
		name = custs[0].CustomerCode
	}
	unpaid := unpaidBotInvoices(ctx, d, tid, custs)
	if len(unpaid) == 0 {
		all := collectPortalInvoices(ctx, d, tid, customerByID(custs))
		if len(all) > 0 {
			_ = client.SendText(ctx, phone,
				"Halo "+name+", tidak ada tagihan berjalan saat ini.\nSemua tagihan sudah lunas. Terima kasih.")
		} else {
			_ = client.SendText(ctx, phone,
				"Halo "+name+", tagihan periode berjalan belum terbit.\nKetik /tagihan lagi nanti.")
		}
		return
	}
	target := unpaid[0]
	extra := ""
	if len(unpaid) > 1 {
		extra = fmt.Sprintf("\n(+%d tagihan terbuka lainnya — menampilkan yang terlama)", len(unpaid)-1)
	}
	remaining := invoiceRemaining(&target)
	due := target.DueDate.Format("02/01/2006")

	switch cmd {
	case "/tagihan":
		inv, items, err := d.Store.GetInvoice(ctx, tid, target.ID)
		if err != nil {
			slog.Warn("wa bot: get invoice", "tenant_id", tid, "err", err)
			return
		}
		pdf := renderInvoicePDF(ctx, d, tid, inv, items)
		if len(pdf) == 0 {
			_ = client.SendText(ctx, phone, "Gagal membuat PDF tagihan. Hubungi admin.")
			return
		}
		caption := fmt.Sprintf("Tagihan %s\n%s • jatuh tempo %s%s",
			inv.InvoiceNumber, formatRupiahID(remaining), due, extra)
		if err := client.SendFile(ctx, phone, caption, inv.InvoiceNumber+".pdf", "application/pdf", pdf); err != nil {
			slog.Warn("wa bot: kirim PDF", "tenant_id", tid, "err", err)
		}
	case "/link", "/qris":
		opts := listEnabledPayOptions(ctx, d, tid)
		if len(opts) == 0 {
			_ = client.SendText(ctx, phone, "Pembayaran online belum aktif. Hubungi admin.")
			return
		}
		origin := appPublicOrigin(ctx, d, tid, "", "", "", "", "")
		pi, err := checkoutInvoice(ctx, d, tid, &target, opts[0].Provider, "", origin)
		if err != nil {
			msg := "Gagal membuat pembayaran. Coba lagi atau hubungi admin."
			if strings.Contains(err.Error(), "URL publik") {
				msg = "Pembayaran online belum tersedia saat ini. Hubungi admin."
			}
			_ = client.SendText(ctx, phone, msg)
			return
		}
		if cmd == "/qris" && strings.TrimSpace(pi.QRString) != "" {
			png, err := qrcode.Encode(strings.TrimSpace(pi.QRString), qrcode.Medium, 512)
			if err == nil && len(png) > 0 {
				caption := fmt.Sprintf("Scan QRIS untuk membayar %s (%s).%s",
					target.InvoiceNumber, formatRupiahID(remaining), extra)
				if serr := client.SendImage(ctx, phone, caption, png); serr == nil {
					return
				} else {
					slog.Warn("wa bot: kirim QRIS", "tenant_id", tid, "err", serr)
				}
			}
		}
		if strings.TrimSpace(pi.CheckoutURL) == "" {
			_ = client.SendText(ctx, phone, "Link bayar belum tersedia. Hubungi admin.")
			return
		}
		text := fmt.Sprintf("Tagihan %s sebesar %s.\nBayar online di sini:\n%s%s",
			target.InvoiceNumber, formatRupiahID(remaining), strings.TrimSpace(pi.CheckoutURL), extra)
		if pi.ExpiresAt != nil && !pi.ExpiresAt.IsZero() {
			text += "\nBerlaku s.d. " + pi.ExpiresAt.Format("02/01/2006 15:04")
		}
		if cmd == "/qris" {
			text += "\n(QRIS tersedia di halaman bayar.)"
		}
		_ = client.SendText(ctx, phone, text)
	}
}

func customerByID(custs []*store.Customer) map[xid.ID]*store.Customer {
	m := make(map[xid.ID]*store.Customer, len(custs))
	for _, c := range custs {
		if c != nil {
			m[c.ID] = c
		}
	}
	return m
}
