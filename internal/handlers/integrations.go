package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
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
)

type paymentIntegrationStored struct {
	APIKey        string `json:"api_key"`
	WebhookSecret string `json:"webhook_secret"`
	BaseURL       string `json:"base_url"`
	Enabled       bool   `json:"enabled"`
}

type paymentIntegrationView struct {
	Configured      bool   `json:"configured"`
	Enabled         bool   `json:"enabled"`
	BaseURL         string `json:"base_url"`
	Method          string `json:"method"`
	Provider        string `json:"provider"`
	EnvFallback     bool   `json:"env_fallback"`
	APIKey          string `json:"api_key,omitempty"`
	WebhookSecret   string `json:"webhook_secret,omitempty"`
	WebhookPath     string `json:"webhook_path"`
	WebhookURL      string `json:"webhook_url"`
	WebhookBaseHint string `json:"webhook_base_hint"`
}

type paymentIntegrationPut struct {
	Enabled       bool   `json:"enabled"`
	BaseURL       string `json:"base_url,omitempty"`
	APIKey        string `json:"api_key,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
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

func registerIntegrations(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-payment-integration", Method: http.MethodGet, Path: "/api/integrations/payment",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Host            string `header:"Host"`
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
		return &struct{ Body paymentIntegrationView }{Body: paymentView(ctx, d, tid, stored, input.XForwardedProto, input.XForwardedHost, input.Host)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-payment-integration", Method: http.MethodPut, Path: "/api/integrations/payment",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Host            string `header:"Host"`
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
		if err := d.Store.UpsertSettingJSON(ctx, tid, settingPayment, cur); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body paymentIntegrationView }{Body: paymentView(ctx, d, tid, cur, input.XForwardedProto, input.XForwardedHost, input.Host)}, nil
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
		return &struct{ Body messagingIntegrationView }{Body: messagingView(stored)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-telegram-integration", Method: http.MethodPut, Path: "/api/integrations/telegram",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body messagingIntegrationView
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
		return &struct{ Body messagingIntegrationView }{Body: messagingView(cur)}, nil
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

func paymentWebhookPath() string {
	return "/api/webhooks/payment/" + payment.ProviderDRP
}

func publicOrigin(proto, forwardedHost, host string) string {
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

func paymentWebhookURL(ctx context.Context, d *Deps, tid xid.ID, proto, forwardedHost, host string) string {
	path := paymentWebhookPath()
	if net, err := d.Store.GetIsolirNetworkSettings(ctx, tid); err == nil {
		if base := strings.TrimRight(strings.TrimSpace(net.PortalBaseURL), "/"); base != "" {
			return base + path
		}
	}
	if origin := publicOrigin(proto, forwardedHost, host); origin != "" {
		return origin + path
	}
	return path
}

func paymentView(ctx context.Context, d *Deps, tid xid.ID, s paymentIntegrationStored, proto, forwardedHost, host string) paymentIntegrationView {
	envKey := ""
	if d != nil && d.Config != nil {
		envKey = strings.TrimSpace(d.Config.DRPPaymentAPIKey)
	}
	envFallback := s.APIKey == "" && envKey != ""
	base := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	if base == "" && d != nil && d.Config != nil {
		base = strings.TrimRight(strings.TrimSpace(d.Config.DRPPaymentBaseURL), "/")
	}
	if base == "" {
		base = payment.DefaultDRPBaseURL
	}
	hint := paymentWebhookPath()
	webhookURL := paymentWebhookURL(ctx, d, tid, proto, forwardedHost, host)
	return paymentIntegrationView{
		Configured:      s.APIKey != "" || envKey != "",
		Enabled:         s.Enabled,
		BaseURL:         base,
		Method:          "qris",
		Provider:        payment.ProviderDRP,
		EnvFallback:     envFallback,
		WebhookPath:     hint,
		WebhookURL:      webhookURL,
		WebhookBaseHint: webhookURL,
	}
}

func messagingView(s messagingIntegrationStored) messagingIntegrationView {
	return messagingIntegrationView{
		WhatsAppAPIURL:     s.WhatsAppAPIURL,
		WhatsAppConfigured: s.WhatsAppAPIKey != "",
		TelegramConfigured: s.TelegramBotToken != "" && s.TelegramChatID != "",
		TelegramChatID:     s.TelegramChatID,
		WhatsAppEnabled:    s.WhatsAppEnabled,
		TelegramEnabled:    s.TelegramEnabled,
	}
}

func decryptSecret(d *Deps, enc string) string {
	if enc == "" || d.Encryptor == nil {
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
	return payment.NewDRPProvider(baseURL, apiKey, webhookSecret), nil
}
