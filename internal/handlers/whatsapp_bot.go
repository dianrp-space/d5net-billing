package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/payment"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/wa"
	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/skip2/go-qrcode"
)

// WhatsApp customer bot.
//
// Incoming messages arrive from the GOWA gateway webhook (event "message").
// The bot only reacts to /tagihan, /link, and /qris sent by a registered,
// active customer. /tagihan without an unpaid bill replies with the last
// paid invoice instead. Everything else stays silent.
// All replies are pushed straight from the configured bot number; the webhook
// itself always answers with an empty reply.

// BotName identifies this customer payment bot, for reference in settings,
// logs, and UI. If more bots are added later (e.g. a CS bot), each gets its
// own name and handler instead of sharing this one.
const BotName = "wabot"

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

func handleWhatsAppBotMessage(ctx context.Context, d *Deps, tid xid.ID, in waBotIncoming) {
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
	slog.Info("wabot command", "bot", BotName, "tenant_id", tid, "cmd", cmd)

	client, err := botClientForTenant(ctx, d, tid)
	if err != nil {
		slog.Warn("wabot: gateway tidak siap", "bot", BotName, "tenant_id", tid, "err", err)
		return
	}
	phone := in.From

	// Bot hanya melayani nomor yang sudah terdaftar sebagai pelanggan aktif.
	// Nomor tak dikenal tidak dibalas sama sekali.
	custs := matchBotCustomers(ctx, d, tid, phone)
	if len(custs) == 0 {
		return
	}
	// Tampilkan "mengetik…" + jeda singkat agar balasan terasa natural.
	// Best-effort: gateway lama tanpa /send/presence mengabaikannya.
	botShowTyping(ctx, client, phone)
	defer botStopTyping(ctx, client, phone)
	name := strings.TrimSpace(custs[0].FullName)
	if name == "" {
		name = custs[0].CustomerCode
	}
	unpaid := unpaidBotInvoices(ctx, d, tid, custs)
	if len(unpaid) == 0 {
		// /tagihan tanpa tunggakan: kirim invoice lunas terakhir + rinciannya.
		if cmd == "/tagihan" && sendLastPaidInvoice(ctx, d, tid, client, phone, custs) {
			return
		}
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
			slog.Warn("wabot: get invoice", "bot", BotName, "tenant_id", tid, "err", err)
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
			slog.Warn("wabot: kirim PDF", "bot", BotName, "tenant_id", tid, "err", err)
		}
	case "/link", "/qris":
		opts := listEnabledPayOptions(ctx, d, tid)
		if len(opts) == 0 {
			_ = client.SendText(ctx, phone, "Pembayaran online belum aktif. Hubungi admin.")
			return
		}
		origin := appPublicOrigin(ctx, d, tid, "", "", "", "", "")
		if cmd == "/qris" {
			if sendDokuQRIS(ctx, d, tid, phone, &target, remaining, extra, origin) {
				return
			}
			_ = client.SendText(ctx, phone,
				"QRIS belum tersedia. Admin perlu mengisi private key + merchant ID + terminal ID + kode pos di Integrasi → DOKU, lalu aktifkan channel QRIS.")
			return
		}
		providerName := opts[0].Provider
		channel := ""
		returnURL := origin
		if providerName == payment.ProviderDoku {
			if len(opts[0].Channels) == 0 {
				_ = client.SendText(ctx, phone, "Metode DOKU belum dikonfigurasi. Hubungi admin.")
				return
			}
			channel = pickDokuBotChannel(opts[0].Channels)
			if channel == "" {
				_ = client.SendText(ctx, phone, "Tidak ada channel DOKU yang siap dipakai. Hubungi admin.")
				return
			}
		}
		pi, err := checkoutInvoice(ctx, d, tid, &target, providerName, channel, returnURL, origin)
		if err != nil {
			slog.Warn("wabot: checkout", "bot", BotName, "tenant_id", tid, "provider", providerName, "channel", channel, "err", err)
			_ = client.SendText(ctx, phone, waCheckoutFailMsg(err))
			return
		}
		// Direct: tampilkan nomor VA / kode gerai / QR bila ada; e-wallet butuh URL.
		if pi.Metadata != nil {
			if code := strings.TrimSpace(fmt.Sprint(pi.Metadata["va_number"])); code != "" && code != "<nil>" {
				_ = client.SendText(ctx, phone, fmt.Sprintf("Transfer VA %s:\n%s\nNominal %s%s",
					fmt.Sprint(pi.Metadata["va_bank"]), code, formatRupiahID(pi.Amount), extra))
				return
			}
			if code := strings.TrimSpace(fmt.Sprint(pi.Metadata["payment_code"])); code != "" && code != "<nil>" {
				_ = client.SendText(ctx, phone, fmt.Sprintf("Bayar di %s dengan kode:\n%s\nNominal %s%s",
					fmt.Sprint(pi.Metadata["retail_label"]), code, formatRupiahID(pi.Amount), extra))
				return
			}
		}
		if pi.QRString != "" {
			png, err := qrcode.Encode(pi.QRString, qrcode.Medium, 512)
			if err == nil && len(png) > 0 {
				caption := fmt.Sprintf("Scan QRIS untuk membayar %s (%s).%s",
					target.InvoiceNumber, formatRupiahID(pi.Amount), extra)
				if err := client.SendImage(ctx, phone, caption, png); err == nil {
					return
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

// pickDokuBotChannel memilih channel yang paling cocok untuk WhatsApp:
// QRIS → VA → retail → e-wallet (redirect kurang nyaman di chat).
func pickDokuBotChannel(channels []dokuChannelFeeView) string {
	prefer := []string{"qr", "va", "retail", "ewallet"}
	for _, kind := range prefer {
		for _, ch := range channels {
			if strings.EqualFold(ch.Kind, kind) && ch.Enabled {
				return ch.ID
			}
		}
	}
	if len(channels) > 0 {
		return channels[0].ID
	}
	return ""
}

func waCheckoutFailMsg(err error) string {
	if err == nil {
		return "Gagal membuat pembayaran. Coba lagi atau hubungi admin."
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "URL publik"):
		return "Pembayaran online belum tersedia saat ini. Hubungi admin."
	case strings.Contains(s, "return URL"):
		return "E-wallet butuh URL portal publik. Isi Portal base URL (pengaturan Isolir/Jaringan), atau aktifkan QRIS / VA / retail."
	case strings.Contains(s, "BIN") || strings.Contains(s, "partner service"):
		return "VA DOKU belum siap. Admin perlu mengisi BIN (partner service ID) di Integrasi → DOKU."
	case strings.Contains(s, "private key") || strings.Contains(s, "QRIS") || strings.Contains(s, "SNAP"):
		return "QRIS DOKU belum siap. Admin perlu mengisi private key + merchant/terminal/kode pos."
	case strings.Contains(s, "channel"):
		return "Metode pembayaran belum dikonfigurasi. Hubungi admin."
	default:
		return "Gagal membuat pembayaran. Coba lagi atau hubungi admin."
	}
}

// sendLastPaidInvoice mengirim PDF invoice lunas terakhir beserta rinciannya
// (nomor, item, jumlah, jatuh tempo, waktu dibayar, status) untuk balasan
// /tagihan saat pelanggan tidak punya tunggakan. False bila tidak ada yang
// lunas / gagal, agar pemanggil memakai teks fallback biasa.
func sendLastPaidInvoice(ctx context.Context, d *Deps, tid xid.ID, client *wa.Client, phone string, custs []*store.Customer) bool {
	ids := make([]xid.ID, 0, len(custs))
	for _, c := range custs {
		if c != nil {
			ids = append(ids, c.ID)
		}
	}
	inv, items, err := d.Store.LastPaidInvoice(ctx, tid, ids)
	if err != nil {
		return false
	}
	pdf := renderInvoicePDF(ctx, d, tid, inv, items)
	if len(pdf) == 0 {
		return false
	}
	planName := d.Store.PlanNameForSubscription(ctx, tid, inv.SubscriptionID)
	item := store.SummarizeInvoiceItems(items)
	if strings.TrimSpace(item) == "" {
		item = store.NotificationItemName(planName, items)
	}
	paidWhen := "—"
	if inv.PaidAt != nil && !inv.PaidAt.IsZero() {
		paidWhen = inv.PaidAt.Format("02/01/2006 15:04")
	}
	caption := fmt.Sprintf("Tagihan terakhir (LUNAS)\nNo: %s\nItem: %s\nJumlah: %s\nJatuh tempo: %s\nDibayar: %s\nStatus: Lunas",
		inv.InvoiceNumber, item, formatRupiahID(inv.TotalAmount), inv.DueDate.Format("02/01/2006"), paidWhen)
	if err := client.SendFile(ctx, phone, caption, inv.InvoiceNumber+".pdf", "application/pdf", pdf); err != nil {
		slog.Warn("wabot: kirim PDF lunas", "bot", BotName, "tenant_id", tid, "err", err)
		return false
	}
	return true
}

// sendDokuQRIS membuat intent Direct QRIS (dengan fee channel) lalu kirim gambar QR.
// False bila channel QRIS tidak ditawarkan / gagal mint.
func sendDokuQRIS(ctx context.Context, d *Deps, tid xid.ID, phone string, inv *store.Invoice, remaining int64, extra, origin string) bool {
	opts := listEnabledPayOptions(ctx, d, tid)
	ready := false
	for _, o := range opts {
		if o.Provider != payment.ProviderDoku {
			continue
		}
		for _, ch := range o.Channels {
			if ch.ID == "qris" {
				ready = true
				break
			}
		}
	}
	if !ready {
		return false
	}
	pi, err := checkoutInvoice(ctx, d, tid, inv, payment.ProviderDoku, "qris", origin, origin)
	if err != nil {
		slog.Warn("wabot: doku QRIS checkout", "bot", BotName, "tenant_id", tid, "err", err)
		return false
	}
	if strings.TrimSpace(pi.QRString) == "" {
		return false
	}
	png, err := qrcode.Encode(pi.QRString, qrcode.Medium, 512)
	if err != nil || len(png) == 0 {
		return false
	}
	client, err := botClientForTenant(ctx, d, tid)
	if err != nil {
		return false
	}
	payAmt := pi.PayableAmount
	if payAmt <= 0 {
		payAmt = pi.Amount
	}
	if payAmt <= 0 {
		payAmt = remaining
	}
	caption := fmt.Sprintf("Scan QRIS untuk membayar %s (%s).%s",
		strings.TrimSpace(inv.InvoiceNumber), formatRupiahID(payAmt), extra)
	if err := client.SendImage(ctx, phone, caption, png); err != nil {
		slog.Warn("wabot: kirim QRIS", "bot", BotName, "tenant_id", tid, "err", err)
		return false
	}
	return true
}

// botTypingDelay menahan balasan sebentar agar indikator mengetik sempat
// terlihat pelanggan sebelum pesan masuk.
const botTypingDelay = 1200 * time.Millisecond

// botShowTyping mengirim presence "mengetik…" lalu menahan sebentar.
// Best-effort: error (mis. gateway lama tanpa /send/presence) diabaikan.
func botShowTyping(ctx context.Context, client *wa.Client, phone string) {
	if client == nil {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = client.SendPresence(pctx, phone, wa.PresenceComposing)
	time.Sleep(botTypingDelay)
}

// botStopTyping menghentikan indikator mengetik setelah balasan dikirim.
func botStopTyping(ctx context.Context, client *wa.Client, phone string) {
	if client == nil {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = client.SendPresence(pctx, phone, wa.PresencePaused)
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
