package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/payment"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/wa"
	"github.com/dianrp/drp-billing/internal/xid"
)

const (
	settingDuitku    = "integration.duitku"
	settingMessaging = "integration.messaging"
	settingSMTP      = "integration.smtp"
)

type duitkuIntegrationStored struct {
	MerchantCode     string `json:"merchant_code"`
	APIKey           string `json:"api_key"`
	Sandbox          bool   `json:"sandbox"`
	Enabled          bool   `json:"enabled"`
	ExpiresInMinutes int    `json:"expires_in_minutes"`
}

type duitkuIntegrationView struct {
	Configured       bool   `json:"configured"`
	Enabled          bool   `json:"enabled"`
	Sandbox          bool   `json:"sandbox"`
	MerchantCode     string `json:"merchant_code"`
	APIKey           string `json:"api_key,omitempty"`
	ExpiresInMinutes int    `json:"expires_in_minutes"`
	WebhookPath      string `json:"webhook_path"`
	WebhookURL       string `json:"webhook_url"`
	WebhookBaseHint  string `json:"webhook_base_hint"`
}

type duitkuIntegrationPut struct {
	Enabled          bool   `json:"enabled"`
	Sandbox          bool   `json:"sandbox"`
	MerchantCode     string `json:"merchant_code"`
	APIKey           string `json:"api_key,omitempty"`
	ExpiresInMinutes int    `json:"expires_in_minutes,omitempty"`
}

type payOptionView struct {
	Provider    string `json:"provider"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
}

type messagingIntegrationStored struct {
	TelegramBotToken string           `json:"telegram_bot_token"`
	TelegramChatID   string           `json:"telegram_chat_id"`
	TelegramEnabled  bool             `json:"telegram_enabled"`
	WhatsAppEnabled  bool             `json:"whatsapp_enabled"`
	WhatsAppBaseURL  string           `json:"whatsapp_base_url"`
	WhatsAppUsername string           `json:"whatsapp_username"`
	WhatsAppPassword string           `json:"whatsapp_password"`
	WhatsAppDeviceID string           `json:"whatsapp_device_id"` // legacy single device
	WhatsAppDevices  []waDeviceStored `json:"whatsapp_devices"`
	// WhatsAppBotEnabled mengaktifkan bot perintah pelanggan (/tagihan, /link, /qris).
	// WhatsAppBotDeviceID memilih nomor yang menjadi bot; kosong = nomor pertama/default.
	WhatsAppBotEnabled  bool   `json:"whatsapp_bot_enabled"`
	WhatsAppBotDeviceID string `json:"whatsapp_bot_device_id"`
}

type waDeviceStored struct {
	DeviceID string `json:"device_id"`
	Label    string `json:"label"`
	Priority int    `json:"priority"`
}

type messagingIntegrationView struct {
	TelegramConfigured bool   `json:"telegram_configured"`
	TelegramChatID     string `json:"telegram_chat_id"`
	TelegramEnabled    bool   `json:"telegram_enabled"`
	TelegramBotToken   string `json:"telegram_bot_token,omitempty"`
}

type waDeviceView struct {
	DeviceID string `json:"device_id"`
	Label    string `json:"label"`
	Priority int    `json:"priority"`
}

type whatsappIntegrationView struct {
	Configured bool           `json:"configured"`
	Enabled    bool           `json:"enabled"`
	BaseURL    string         `json:"base_url"`
	Username   string         `json:"username"`
	Password   string         `json:"password,omitempty"`
	Devices    []waDeviceView `json:"devices"`
	BotEnabled bool           `json:"bot_enabled"`
	BotDevice  string         `json:"bot_device_id,omitempty"`
	// BotName is the reference name of the customer payment bot (wabot),
	// to tell it apart from future bots (e.g. a CS bot).
	BotName string `json:"bot_name"`
}

type whatsappIntegrationPut struct {
	Enabled     bool           `json:"enabled"`
	BaseURL     string         `json:"base_url"`
	Username    string         `json:"username"`
	Password    string         `json:"password,omitempty"`
	Devices     []waDeviceView `json:"devices"`
	BotEnabled  bool           `json:"bot_enabled"`
	BotDeviceID string         `json:"bot_device_id,omitempty"`
}

type telegramIntegrationPut struct {
	TelegramEnabled  bool   `json:"telegram_enabled"`
	TelegramChatID   string `json:"telegram_chat_id"`
	TelegramBotToken string `json:"telegram_bot_token,omitempty"`
}

type smtpIntegrationStored struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	FromName string `json:"from_name"`
	Enabled  bool   `json:"enabled"`
}

type smtpIntegrationView struct {
	Configured  bool   `json:"configured"`
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	From        string `json:"from"`
	FromName    string `json:"from_name"`
	EnvFallback bool   `json:"env_fallback"`
}

type smtpIntegrationPut struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	From     string `json:"from"`
	FromName string `json:"from_name"`
}

func registerIntegrations(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-duitku-integration", Method: http.MethodGet, Path: "/api/integrations/duitku",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Host            string `header:"Host"`
		Origin          string `header:"Origin"`
		Referer         string `header:"Referer"`
		XForwardedHost  string `header:"X-Forwarded-Host"`
		XForwardedProto string `header:"X-Forwarded-Proto"`
	}) (*struct{ Body duitkuIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadDuitkuIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body duitkuIntegrationView }{Body: duitkuView(ctx, d, tid, stored, input.Origin, input.Referer, input.XForwardedProto, input.XForwardedHost, input.Host)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-duitku-integration", Method: http.MethodPut, Path: "/api/integrations/duitku",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Host            string `header:"Host"`
		Origin          string `header:"Origin"`
		Referer         string `header:"Referer"`
		XForwardedHost  string `header:"X-Forwarded-Host"`
		XForwardedProto string `header:"X-Forwarded-Proto"`
		Body            duitkuIntegrationPut
	}) (*struct{ Body duitkuIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := loadDuitkuIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		merchant := strings.TrimSpace(input.Body.MerchantCode)
		if merchant == "" {
			merchant = strings.TrimSpace(cur.MerchantCode)
		}
		if input.Body.Enabled && merchant == "" {
			return nil, httpx.BadRequest("merchant code Duitku wajib diisi")
		}
		if input.Body.Enabled && strings.TrimSpace(input.Body.APIKey) == "" && cur.APIKey == "" {
			return nil, httpx.BadRequest("API key Duitku wajib diisi")
		}
		cur.Enabled = input.Body.Enabled
		cur.Sandbox = input.Body.Sandbox
		cur.MerchantCode = merchant
		if v := strings.TrimSpace(input.Body.APIKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.APIKey = enc
		}
		if input.Body.ExpiresInMinutes > 0 {
			cur.ExpiresInMinutes = payment.ClampDuitkuExpiryMinutes(input.Body.ExpiresInMinutes)
		}
		if err := d.Store.UpsertSettingJSON(ctx, tid, settingDuitku, cur); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body duitkuIntegrationView }{Body: duitkuView(ctx, d, tid, cur, input.Origin, input.Referer, input.XForwardedProto, input.XForwardedHost, input.Host)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-pay-options", Method: http.MethodGet, Path: "/api/integrations/pay-options",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []payOptionView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		return &struct{ Body []payOptionView }{Body: listEnabledPayOptions(ctx, d, tid)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-telegram-integration", Method: http.MethodGet, Path: "/api/integrations/telegram",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body messagingIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadMessagingIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body messagingIntegrationView }{Body: messagingView(d, stored)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-telegram-integration", Method: http.MethodPut, Path: "/api/integrations/telegram",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body telegramIntegrationPut
	}) (*struct{ Body messagingIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := loadMessagingIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cur.TelegramEnabled = input.Body.TelegramEnabled
		cur.TelegramChatID = strings.TrimSpace(input.Body.TelegramChatID)
		if v := strings.TrimSpace(input.Body.TelegramBotToken); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.TelegramBotToken = enc
		}
		if err := d.Store.UpsertSettingJSON(ctx, tid, settingMessaging, cur); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body messagingIntegrationView }{Body: messagingView(d, cur)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-smtp-integration", Method: http.MethodGet, Path: "/api/integrations/smtp",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body smtpIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadSMTPIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body smtpIntegrationView }{Body: smtpView(d, stored)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-smtp-integration", Method: http.MethodPut, Path: "/api/integrations/smtp",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body smtpIntegrationPut
	}) (*struct{ Body smtpIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := loadSMTPIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		host := strings.TrimSpace(input.Body.Host)
		from := strings.TrimSpace(input.Body.From)
		if input.Body.Enabled {
			if host == "" {
				return nil, httpx.BadRequest("host SMTP wajib diisi")
			}
			if from == "" || !strings.Contains(from, "@") {
				return nil, httpx.BadRequest("alamat From wajib diisi (contoh: noreply@domain.id)")
			}
		}
		cur.Enabled = input.Body.Enabled
		cur.Host = host
		cur.From = from
		cur.FromName = strings.TrimSpace(input.Body.FromName)
		cur.Username = strings.TrimSpace(input.Body.Username)
		cur.Port = clampSMTPPort(input.Body.Port)
		if v := strings.TrimSpace(input.Body.Password); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.Password = enc
		}
		if err := d.Store.UpsertSettingJSON(ctx, tid, settingSMTP, cur); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body smtpIntegrationView }{Body: smtpView(d, cur)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-whatsapp-integration", Method: http.MethodGet, Path: "/api/integrations/whatsapp",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body whatsappIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadMessagingIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body whatsappIntegrationView }{Body: whatsappView(d, stored)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-whatsapp-integration", Method: http.MethodPut, Path: "/api/integrations/whatsapp",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body whatsappIntegrationPut
	}) (*struct{ Body whatsappIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := loadMessagingIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		base := strings.TrimRight(strings.TrimSpace(input.Body.BaseURL), "/")
		if input.Body.Enabled && base == "" {
			return nil, httpx.BadRequest("base URL gateway WhatsApp wajib diisi")
		}
		cur.WhatsAppEnabled = input.Body.Enabled
		cur.WhatsAppBaseURL = base
		cur.WhatsAppUsername = strings.TrimSpace(input.Body.Username)
		cur.WhatsAppDeviceID = ""
		cur.WhatsAppDevices = normalizeWADevices(input.Body.Devices)
		cur.WhatsAppBotEnabled = input.Body.BotEnabled
		cur.WhatsAppBotDeviceID = strings.TrimSpace(input.Body.BotDeviceID)
		if v := strings.TrimSpace(input.Body.Password); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.WhatsAppPassword = enc
		}
		if err := d.Store.UpsertSettingJSON(ctx, tid, settingMessaging, cur); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body whatsappIntegrationView }{Body: whatsappView(d, cur)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "check-whatsapp-integration", Method: http.MethodPost, Path: "/api/integrations/whatsapp/check",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body whatsappCheckView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadMessagingIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if strings.TrimSpace(stored.WhatsAppBaseURL) == "" {
			return nil, httpx.BadRequest("gateway WhatsApp belum dikonfigurasi")
		}
		pass := decryptSecret(d, stored.WhatsAppPassword)
		out := &struct{ Body whatsappCheckView }{}
		for _, dev := range effectiveWADevices(stored) {
			row := whatsappDeviceStatus{DeviceID: dev.DeviceID, Label: dev.Label}
			client := wa.NewClient(wa.Config{
				BaseURL:  stored.WhatsAppBaseURL,
				Username: stored.WhatsAppUsername,
				Password: pass,
				DeviceID: dev.DeviceID,
			})
			st, cerr := client.CheckStatus(ctx)
			if cerr != nil {
				row.Error = cerr.Error()
			} else {
				row.Connected = st.Connected
				row.LoggedIn = st.LoggedIn
				row.JID = st.JID
			}
			out.Body.Devices = append(out.Body.Devices, row)
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-whatsapp-devices", Method: http.MethodGet, Path: "/api/integrations/whatsapp/devices",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body whatsappDevicesView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadMessagingIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		client := wa.NewClient(wa.Config{
			BaseURL:  stored.WhatsAppBaseURL,
			Username: stored.WhatsAppUsername,
			Password: decryptSecret(d, stored.WhatsAppPassword),
		})
		if !client.Configured() {
			return nil, httpx.BadRequest("gateway WhatsApp belum dikonfigurasi")
		}
		list, err := client.ListDevices(ctx)
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		out := &struct{ Body whatsappDevicesView }{}
		for _, dev := range list {
			out.Body.Devices = append(out.Body.Devices, whatsappGatewayDevice{
				DeviceID: dev.DeviceID,
				Name:     dev.Name,
				JID:      dev.JID,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "test-messaging-integration", Method: http.MethodPost, Path: "/api/integrations/messaging/test",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Channel   string `json:"channel"`
			Recipient string `json:"recipient,omitempty"`
			Subject   string `json:"subject,omitempty"`
			Body      string `json:"body,omitempty"`
		}
	}) (*struct{ Body messagingTestView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if d.Notify == nil {
			return nil, httpx.Internal(fmt.Errorf("notify service not configured"))
		}
		channel := strings.ToLower(strings.TrimSpace(input.Body.Channel))
		if channel == "" {
			return nil, httpx.BadRequest("channel wajib diisi")
		}
		if err := d.Notify.SendTest(ctx, tid, channel, input.Body.Recipient, input.Body.Subject, input.Body.Body); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body messagingTestView }{Body: messagingTestView{Status: "sent", Channel: channel}}, nil
	})
}

type messagingTestView struct {
	Status  string `json:"status"`
	Channel string `json:"channel"`
}

type whatsappDeviceStatus struct {
	DeviceID  string `json:"device_id"`
	Label     string `json:"label,omitempty"`
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"logged_in"`
	JID       string `json:"jid,omitempty"`
	Error     string `json:"error,omitempty"`
}

type whatsappCheckView struct {
	Devices []whatsappDeviceStatus `json:"devices"`
}

type whatsappGatewayDevice struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name,omitempty"`
	JID      string `json:"jid,omitempty"`
}

type whatsappDevicesView struct {
	Devices []whatsappGatewayDevice `json:"devices"`
}

func loadDuitkuIntegration(ctx context.Context, d *Deps, tid xid.ID) (duitkuIntegrationStored, error) {
	var s duitkuIntegrationStored
	err := d.Store.GetSettingJSON(ctx, tid, settingDuitku, &s)
	if errors.Is(err, store.ErrNotFound) {
		return duitkuIntegrationStored{}, nil
	}
	return s, err
}

func loadMessagingIntegration(ctx context.Context, d *Deps, tid xid.ID) (messagingIntegrationStored, error) {
	var s messagingIntegrationStored
	err := d.Store.GetSettingJSON(ctx, tid, settingMessaging, &s)
	if errors.Is(err, store.ErrNotFound) {
		return messagingIntegrationStored{}, nil
	}
	return s, err
}

func loadSMTPIntegration(ctx context.Context, d *Deps, tid xid.ID) (smtpIntegrationStored, error) {
	var s smtpIntegrationStored
	err := d.Store.GetSettingJSON(ctx, tid, settingSMTP, &s)
	if errors.Is(err, store.ErrNotFound) {
		return smtpIntegrationStored{}, nil
	}
	return s, err
}

func clampSMTPPort(port int) int {
	if port < 1 || port > 65535 {
		return 587
	}
	return port
}

func smtpView(d *Deps, s smtpIntegrationStored) smtpIntegrationView {
	port := s.Port
	if port < 1 || port > 65535 {
		port = 587
	}
	host := strings.TrimSpace(s.Host)
	from := strings.TrimSpace(s.From)
	return smtpIntegrationView{
		Configured:  host != "" && from != "",
		Enabled:     s.Enabled,
		Host:        host,
		Port:        port,
		Username:    strings.TrimSpace(s.Username),
		Password:    decryptSecret(d, s.Password),
		From:        from,
		FromName:    strings.TrimSpace(s.FromName),
		EnvFallback: !s.Enabled && strings.TrimSpace(os.Getenv("SMTP_HOST")) != "",
	}
}

func paymentWebhookPathFor(provider string) string {
	return "/api/webhooks/payment/" + normalizePaymentProviderName(provider)
}

// paymentWebhookPathWithTenant appends the tenant hint so the (shared) webhook
// endpoint can resolve per-tenant gateway credentials even for a test payload
// that has no matching payment intent.
func paymentWebhookPathWithTenant(provider, slug string) string {
	path := paymentWebhookPathFor(provider)
	if strings.TrimSpace(slug) == "" {
		return path
	}
	return path + "?tenant=" + url.QueryEscape(strings.TrimSpace(slug))
}

func tenantSlug(ctx context.Context, d *Deps, tid xid.ID) string {
	if d == nil || d.Store == nil || xid.IsNil(tid) {
		return ""
	}
	if t, err := d.Store.GetTenant(ctx, tid); err == nil && t != nil {
		return t.Slug
	}
	return ""
}

func originFromReferer(referer string) string {
	u, err := url.Parse(strings.TrimSpace(referer))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
}

func publicOrigin(origin, referer, proto, forwardedHost, host string) string {
	o := strings.TrimSpace(origin)
	if strings.HasPrefix(o, "http://") || strings.HasPrefix(o, "https://") {
		return strings.TrimRight(o, "/")
	}
	if r := originFromReferer(referer); r != "" {
		return r
	}
	h := strings.TrimSpace(forwardedHost)
	if h == "" {
		h = strings.TrimSpace(host)
	}
	if i := strings.IndexByte(h, ','); i >= 0 {
		h = strings.TrimSpace(h[:i])
	}
	if h == "" {
		return ""
	}
	p := strings.ToLower(strings.TrimSpace(proto))
	if i := strings.IndexByte(p, ','); i >= 0 {
		p = strings.TrimSpace(p[:i])
	}
	if p != "http" && p != "https" {
		if strings.Contains(h, "localhost") || strings.HasPrefix(h, "127.") || strings.HasPrefix(h, "192.168.") {
			p = "http"
		} else {
			p = "https"
		}
	}
	return p + "://" + h
}

func appPublicOrigin(ctx context.Context, d *Deps, tid xid.ID, origin, referer, proto, forwardedHost, host string) string {
	if app := publicOrigin(origin, referer, proto, forwardedHost, host); app != "" {
		return app
	}
	if d != nil && d.Store != nil {
		if net, err := d.Store.GetIsolirNetworkSettings(ctx, tid); err == nil {
			if base := strings.TrimRight(strings.TrimSpace(net.PortalBaseURL), "/"); base != "" {
				return base
			}
		}
	}
	return ""
}

func paymentWebhookURLFor(ctx context.Context, d *Deps, tid xid.ID, origin, referer, proto, forwardedHost, host, provider string) string {
	path := paymentWebhookPathWithTenant(provider, tenantSlug(ctx, d, tid))
	if app := appPublicOrigin(ctx, d, tid, origin, referer, proto, forwardedHost, host); app != "" {
		return app + path
	}
	return path
}

func duitkuView(ctx context.Context, d *Deps, tid xid.ID, s duitkuIntegrationStored, origin, referer, proto, forwardedHost, host string) duitkuIntegrationView {
	apiKey := decryptSecret(d, s.APIKey)
	merchant := strings.TrimSpace(s.MerchantCode)
	webhookURL := paymentWebhookURLFor(ctx, d, tid, origin, referer, proto, forwardedHost, host, payment.ProviderDuitku)
	return duitkuIntegrationView{
		Configured:       merchant != "" && s.APIKey != "",
		Enabled:          s.Enabled,
		Sandbox:          s.Sandbox,
		MerchantCode:     merchant,
		APIKey:           apiKey,
		ExpiresInMinutes: payment.ClampDuitkuExpiryMinutes(s.ExpiresInMinutes),
		WebhookPath:      paymentWebhookPathFor(payment.ProviderDuitku),
		WebhookURL:       webhookURL,
		WebhookBaseHint:  webhookURL,
	}
}

func messagingView(d *Deps, s messagingIntegrationStored) messagingIntegrationView {
	return messagingIntegrationView{
		TelegramConfigured: s.TelegramBotToken != "" && s.TelegramChatID != "",
		TelegramChatID:     s.TelegramChatID,
		TelegramEnabled:    s.TelegramEnabled,
		TelegramBotToken:   decryptSecret(d, s.TelegramBotToken),
	}
}

func whatsappView(d *Deps, s messagingIntegrationStored) whatsappIntegrationView {
	devices := make([]waDeviceView, 0, len(s.WhatsAppDevices))
	for i, dev := range effectiveWADevices(s) {
		devices = append(devices, waDeviceView{DeviceID: dev.DeviceID, Label: dev.Label, Priority: i})
	}
	return whatsappIntegrationView{
		Configured: strings.TrimSpace(s.WhatsAppBaseURL) != "",
		Enabled:    s.WhatsAppEnabled,
		BaseURL:    s.WhatsAppBaseURL,
		Username:   s.WhatsAppUsername,
		Password:   decryptSecret(d, s.WhatsAppPassword),
		Devices:    devices,
		BotEnabled: s.WhatsAppBotEnabled,
		BotDevice:  strings.TrimSpace(s.WhatsAppBotDeviceID),
		BotName:    BotName,
	}
}

// normalizeWADevices trims/dedupes device entries and assigns priority from the
// order (explicit priority wins, else request order). First item is tried first.
func normalizeWADevices(in []waDeviceView) []waDeviceStored {
	sorted := make([]waDeviceView, len(in))
	copy(sorted, in)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })
	seen := map[string]struct{}{}
	out := make([]waDeviceStored, 0, len(sorted))
	for _, dev := range sorted {
		id := strings.TrimSpace(dev.DeviceID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, waDeviceStored{
			DeviceID: id,
			Label:    strings.TrimSpace(dev.Label),
			Priority: len(out),
		})
	}
	return out
}

// effectiveWADevices returns the configured devices ordered by priority,
// falling back to the legacy single device id, then to a single default
// (empty id = gateway default).
func effectiveWADevices(s messagingIntegrationStored) []waDeviceStored {
	var out []waDeviceStored
	switch {
	case len(s.WhatsAppDevices) > 0:
		out = append(out, s.WhatsAppDevices...)
	case strings.TrimSpace(s.WhatsAppDeviceID) != "":
		out = []waDeviceStored{{DeviceID: strings.TrimSpace(s.WhatsAppDeviceID)}}
	default:
		out = []waDeviceStored{{}}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}

func decryptSecret(d *Deps, enc string) string {
	if enc == "" || d == nil || d.Encryptor == nil {
		return ""
	}
	plain, err := d.Encryptor.DecryptString(enc)
	if err != nil {
		return ""
	}
	return plain
}

func normalizePaymentProviderName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "", "duitku", "duitku_pop", "duitkupop", "pop":
		return payment.ProviderDuitku
	default:
		return name
	}
}

func listEnabledPayOptions(ctx context.Context, d *Deps, tenantID xid.ID) []payOptionView {
	out := make([]payOptionView, 0, 1)
	if duitkuPaymentReady(ctx, d, tenantID) {
		out = append(out, payOptionView{
			Provider:    payment.ProviderDuitku,
			Label:       "Duitku Payment Gateway",
			Description: "Popup pembayaran Duitku (VA, e-wallet, retail, QRIS)",
			Kind:        "popup",
		})
	}
	return out
}

func duitkuPaymentReady(ctx context.Context, d *Deps, tenantID xid.ID) bool {
	cfg, _ := loadDuitkuIntegration(ctx, d, tenantID)
	if !cfg.Enabled {
		return false
	}
	return strings.TrimSpace(cfg.MerchantCode) != "" && decryptSecret(d, cfg.APIKey) != ""
}

// resolvePaymentProvider resolves the gateway for the provider from the
// tenant's own integration credentials.
func resolvePaymentProvider(ctx context.Context, d *Deps, tenantID xid.ID, name string) (payment.Provider, error) {
	name = normalizePaymentProviderName(name)
	if name == payment.ProviderManual {
		return d.Payments.Get(payment.ProviderManual)
	}
	if name != payment.ProviderDuitku {
		return nil, httpx.BadRequest("payment gateway tidak dikenali")
	}
	cfg, _ := loadDuitkuIntegration(ctx, d, tenantID)
	if !cfg.Enabled {
		return nil, httpx.BadRequest("Duitku POP belum diaktifkan di Integrasi")
	}
	apiKey := decryptSecret(d, cfg.APIKey)
	if strings.TrimSpace(cfg.MerchantCode) == "" || apiKey == "" {
		return nil, httpx.BadRequest("Duitku POP belum dikonfigurasi")
	}
	return payment.NewDuitkuProvider(cfg.MerchantCode, apiKey, cfg.Sandbox, cfg.ExpiresInMinutes), nil
}
