package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/payment"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/wa"
	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/skip2/go-qrcode"
)

const (
	settingPayment   = "integration.payment"
	settingMessaging = "integration.messaging"
	settingSMTP      = "integration.smtp"
)

type paymentIntegrationStored struct {
	APIKey           string `json:"api_key"`
	WebhookSecret    string `json:"webhook_secret"`
	BaseURL          string `json:"base_url"`
	ExpiresInMinutes int    `json:"expires_in_minutes"`
	Enabled          bool   `json:"enabled"`
}

type paymentIntegrationView struct {
	Configured       bool   `json:"configured"`
	Enabled          bool   `json:"enabled"`
	BaseURL          string `json:"base_url"`
	Method           string `json:"method"`
	Provider         string `json:"provider"`
	EnvFallback      bool   `json:"env_fallback"`
	APIKey           string `json:"api_key"`
	WebhookSecret    string `json:"webhook_secret"`
	ExpiresInMinutes int    `json:"expires_in_minutes"`
	WebhookPath      string `json:"webhook_path"`
	WebhookURL       string `json:"webhook_url"`
	WebhookBaseHint  string `json:"webhook_base_hint"`
}

type paymentIntegrationPut struct {
	Enabled          bool   `json:"enabled"`
	BaseURL          string `json:"base_url,omitempty"`
	APIKey           string `json:"api_key,omitempty"`
	WebhookSecret    string `json:"webhook_secret,omitempty"`
	ExpiresInMinutes int    `json:"expires_in_minutes,omitempty"`
}

type messagingIntegrationStored struct {
	WhatsAppAPIURL   string `json:"whatsapp_api_url"`
	WhatsAppAPIKey   string `json:"whatsapp_api_key"`
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramChatID   string `json:"telegram_chat_id"`
	WhatsAppEnabled  bool   `json:"whatsapp_enabled"`
	TelegramEnabled  bool   `json:"telegram_enabled"`
}

type messagingIntegrationView struct {
	WhatsAppAPIURL     string `json:"whatsapp_api_url"`
	WhatsAppConfigured bool   `json:"whatsapp_configured"`
	TelegramConfigured bool   `json:"telegram_configured"`
	TelegramChatID     string `json:"telegram_chat_id"`
	WhatsAppEnabled    bool   `json:"whatsapp_enabled"`
	TelegramEnabled    bool   `json:"telegram_enabled"`
	WhatsAppAPIKey     string `json:"whatsapp_api_key,omitempty"`
	TelegramBotToken   string `json:"telegram_bot_token,omitempty"`
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
		OperationID: "get-payment-integration", Method: http.MethodGet, Path: "/api/integrations/payment",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Host            string `header:"Host"`
		Origin          string `header:"Origin"`
		Referer         string `header:"Referer"`
		XForwardedHost  string `header:"X-Forwarded-Host"`
		XForwardedProto string `header:"X-Forwarded-Proto"`
	}) (*struct{ Body paymentIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadPaymentIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body paymentIntegrationView }{Body: paymentView(ctx, d, tid, stored, input.Origin, input.Referer, input.XForwardedProto, input.XForwardedHost, input.Host)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-payment-integration", Method: http.MethodPut, Path: "/api/integrations/payment",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Host            string `header:"Host"`
		Origin          string `header:"Origin"`
		Referer         string `header:"Referer"`
		XForwardedHost  string `header:"X-Forwarded-Host"`
		XForwardedProto string `header:"X-Forwarded-Proto"`
		Body            paymentIntegrationPut
	}) (*struct{ Body paymentIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := loadPaymentIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cur.Enabled = input.Body.Enabled
		if v := strings.TrimSpace(input.Body.BaseURL); v != "" {
			cur.BaseURL = strings.TrimRight(v, "/")
		}
		if v := strings.TrimSpace(input.Body.APIKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.APIKey = enc
		}
		if v := strings.TrimSpace(input.Body.WebhookSecret); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.WebhookSecret = enc
		}
		if input.Body.ExpiresInMinutes > 0 {
			cur.ExpiresInMinutes = payment.ClampQRISExpiresMinutes(input.Body.ExpiresInMinutes)
		}
		if err := d.Store.UpsertSettingJSON(ctx, tid, settingPayment, cur); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body paymentIntegrationView }{Body: paymentView(ctx, d, tid, cur, input.Origin, input.Referer, input.XForwardedProto, input.XForwardedHost, input.Host)}, nil
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
		OperationID: "get-whatsapp-status", Method: http.MethodGet, Path: "/api/integrations/whatsapp/status",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body waStatusView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if d.WA == nil {
			return nil, httpx.Internal(fmt.Errorf("whatsapp manager not configured"))
		}
		st, err := d.WA.Status(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cfg, _ := loadMessagingIntegration(ctx, d, tid)
		return &struct{ Body waStatusView }{Body: toWAStatusView(st, cfg.WhatsAppEnabled)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "connect-whatsapp", Method: http.MethodPost, Path: "/api/integrations/whatsapp/connect",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Enabled *bool `json:"enabled,omitempty"`
		}
	}) (*struct{ Body waStatusView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if d.WA == nil {
			return nil, httpx.Internal(fmt.Errorf("whatsapp manager not configured"))
		}
		cfg, err := loadMessagingIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if input.Body.Enabled != nil {
			cfg.WhatsAppEnabled = *input.Body.Enabled
			_ = d.Store.UpsertSettingJSON(ctx, tid, settingMessaging, cfg)
		} else {
			cfg.WhatsAppEnabled = true
			_ = d.Store.UpsertSettingJSON(ctx, tid, settingMessaging, cfg)
		}
		st, err := d.WA.Connect(ctx, tid)
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body waStatusView }{Body: toWAStatusView(st, cfg.WhatsAppEnabled)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "logout-whatsapp", Method: http.MethodPost, Path: "/api/integrations/whatsapp/logout",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body waStatusView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if d.WA == nil {
			return nil, httpx.Internal(fmt.Errorf("whatsapp manager not configured"))
		}
		if err := d.WA.Logout(ctx, tid); err != nil {
			return nil, httpx.Internal(err)
		}
		cfg, _ := loadMessagingIntegration(ctx, d, tid)
		cfg.WhatsAppEnabled = false
		_ = d.Store.UpsertSettingJSON(ctx, tid, settingMessaging, cfg)
		st, err := d.WA.Status(ctx, tid)
		if err != nil {
			return &struct{ Body waStatusView }{Body: waStatusView{Enabled: false}}, nil
		}
		return &struct{ Body waStatusView }{Body: toWAStatusView(st, false)}, nil
	})
}

type waStatusView struct {
	Enabled       bool   `json:"enabled"`
	Connected     bool   `json:"connected"`
	LoggedIn      bool   `json:"logged_in"`
	JID           string `json:"jid,omitempty"`
	Phone         string `json:"phone,omitempty"`
	QRCode        string `json:"qr_code,omitempty"`
	QREvent       string `json:"qr_event,omitempty"`
	QRImageBase64 string `json:"qr_image_base64,omitempty"`
}

func toWAStatusView(st *wa.Status, enabled bool) waStatusView {
	if st == nil {
		return waStatusView{Enabled: enabled}
	}
	out := waStatusView{
		Enabled:   enabled,
		Connected: st.Connected,
		LoggedIn:  st.LoggedIn,
		JID:       st.JID,
		Phone:     st.Phone,
		QRCode:    st.QRCode,
		QREvent:   st.QREvent,
	}
	if st.QRCode != "" {
		if png, err := qrcode.Encode(st.QRCode, qrcode.Medium, 256); err == nil {
			out.QRImageBase64 = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		}
	}
	return out
}

func loadPaymentIntegration(ctx context.Context, d *Deps, tid xid.ID) (paymentIntegrationStored, error) {
	var s paymentIntegrationStored
	err := d.Store.GetSettingJSON(ctx, tid, settingPayment, &s)
	if errors.Is(err, store.ErrNotFound) {
		return paymentIntegrationStored{}, nil
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

func paymentWebhookPath() string {
	return "/api/webhooks/payment/" + payment.ProviderDRP
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

func paymentWebhookURL(ctx context.Context, d *Deps, tid xid.ID, origin, referer, proto, forwardedHost, host string) string {
	path := paymentWebhookPath()
	if app := publicOrigin(origin, referer, proto, forwardedHost, host); app != "" {
		return app + path
	}
	if d != nil && d.Store != nil {
		if net, err := d.Store.GetIsolirNetworkSettings(ctx, tid); err == nil {
			if base := strings.TrimRight(strings.TrimSpace(net.PortalBaseURL), "/"); base != "" {
				return base + path
			}
		}
	}
	return path
}

func paymentView(ctx context.Context, d *Deps, tid xid.ID, s paymentIntegrationStored, origin, referer, proto, forwardedHost, host string) paymentIntegrationView {
	envKey := ""
	envSecret := ""
	if d != nil && d.Config != nil {
		envKey = strings.TrimSpace(d.Config.DRPPaymentAPIKey)
		envSecret = d.Config.DRPPaymentWebhookSecret
	}
	apiKey := decryptSecret(d, s.APIKey)
	webhookSecret := decryptSecret(d, s.WebhookSecret)
	envFallback := s.APIKey == "" && envKey != ""
	if apiKey == "" {
		apiKey = envKey
	}
	if webhookSecret == "" {
		webhookSecret = envSecret
	}
	base := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	if base == "" && d != nil && d.Config != nil {
		base = strings.TrimRight(strings.TrimSpace(d.Config.DRPPaymentBaseURL), "/")
	}
	if base == "" {
		base = payment.DefaultDRPBaseURL
	}
	hint := paymentWebhookPath()
	webhookURL := paymentWebhookURL(ctx, d, tid, origin, referer, proto, forwardedHost, host)
	return paymentIntegrationView{
		Configured:       s.APIKey != "" || envKey != "",
		Enabled:          s.Enabled,
		BaseURL:          base,
		Method:           "qris",
		Provider:         payment.ProviderDRP,
		EnvFallback:      envFallback,
		APIKey:           apiKey,
		WebhookSecret:    webhookSecret,
		ExpiresInMinutes: qrisExpiresMinutes(s, d),
		WebhookPath:      hint,
		WebhookURL:       webhookURL,
		WebhookBaseHint:  webhookURL,
	}
}

func messagingView(d *Deps, s messagingIntegrationStored) messagingIntegrationView {
	return messagingIntegrationView{
		WhatsAppAPIURL:     s.WhatsAppAPIURL,
		WhatsAppConfigured: s.WhatsAppAPIKey != "",
		TelegramConfigured: s.TelegramBotToken != "" && s.TelegramChatID != "",
		TelegramChatID:     s.TelegramChatID,
		WhatsAppEnabled:    s.WhatsAppEnabled,
		TelegramEnabled:    s.TelegramEnabled,
		TelegramBotToken:   decryptSecret(d, s.TelegramBotToken),
	}
}

func qrisExpiresMinutes(s paymentIntegrationStored, d *Deps) int {
	if s.ExpiresInMinutes > 0 {
		return payment.ClampQRISExpiresMinutes(s.ExpiresInMinutes)
	}
	if d != nil && d.Config != nil && d.Config.DRPPaymentExpiresInMinutes > 0 {
		return payment.ClampQRISExpiresMinutes(d.Config.DRPPaymentExpiresInMinutes)
	}
	return payment.DefaultQRISExpiresMinutes
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
	case "", "qris", "drp_payment", "drp-payment", "drppayment":
		return payment.ProviderDRP
	default:
		return name
	}
}

func drpCredentials(d *Deps, cfg paymentIntegrationStored) (baseURL, apiKey, webhookSecret string) {
	apiKey = decryptSecret(d, cfg.APIKey)
	webhookSecret = decryptSecret(d, cfg.WebhookSecret)
	baseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if d != nil && d.Config != nil {
		if apiKey == "" {
			apiKey = strings.TrimSpace(d.Config.DRPPaymentAPIKey)
		}
		if webhookSecret == "" {
			webhookSecret = d.Config.DRPPaymentWebhookSecret
		}
		if baseURL == "" {
			baseURL = strings.TrimRight(strings.TrimSpace(d.Config.DRPPaymentBaseURL), "/")
		}
	}
	if baseURL == "" {
		baseURL = payment.DefaultDRPBaseURL
	}
	return baseURL, apiKey, webhookSecret
}

// resolvePaymentProvider prefers tenant integration keys, then falls back to env.
func resolvePaymentProvider(ctx context.Context, d *Deps, tenantID xid.ID, name string) (payment.Provider, error) {
	name = normalizePaymentProviderName(name)
	if name == payment.ProviderManual {
		return d.Payments.Get(payment.ProviderManual)
	}
	if name != payment.ProviderDRP {
		return nil, fmt.Errorf("unknown payment provider: %s", name)
	}
	cfg, _ := loadPaymentIntegration(ctx, d, tenantID)
	hasTenantCfg := cfg.APIKey != "" || cfg.WebhookSecret != "" || cfg.BaseURL != "" || cfg.Enabled
	if hasTenantCfg && !cfg.Enabled {
		return nil, httpx.BadRequest("DRP Payment belum diaktifkan di Integrasi")
	}
	baseURL, apiKey, webhookSecret := drpCredentials(d, cfg)
	if apiKey == "" {
		if d.Payments != nil && d.Payments.Has(payment.ProviderDRP) {
			return d.Payments.Get(payment.ProviderDRP)
		}
		return nil, httpx.BadRequest("DRP Payment belum dikonfigurasi")
	}
	p := payment.NewDRPProvider(baseURL, apiKey, webhookSecret)
	p.ExpiresInMinutes = qrisExpiresMinutes(cfg, d)
	return p, nil
}
