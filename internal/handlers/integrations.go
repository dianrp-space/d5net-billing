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
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/payment"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/wa"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

const (
	settingDuitku    = "integration.duitku"
	settingDoku      = "integration.doku"
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

type dokuIntegrationStored struct {
	ClientID         string `json:"client_id"`
	SecretKey        string `json:"secret_key"`
	PrivateKey       string `json:"private_key"`
	MerchantID       string `json:"merchant_id"`
	TerminalID       string `json:"terminal_id"`
	PostalCode       string `json:"postal_code"`
	PartnerServiceID string `json:"partner_service_id"`
	Sandbox          bool   `json:"sandbox"`
	Enabled          bool   `json:"enabled"`
	ExpiresInMinutes int    `json:"expires_in_minutes"`
	// FeeMode: "customer" = biaya admin ditambahkan ke tagihan (customer bayar
	// lebih); selain itu (kosong/"merchant") = merchant menanggung (default).
	FeeMode    string                          `json:"fee_mode"`
	FeeFlat    int64                           `json:"fee_flat,omitempty"` // legacy global
	FeePercent float64                         `json:"fee_percent,omitempty"`
	Channels   map[string]dokuChannelFeeStored `json:"channels,omitempty"`
}

type dokuChannelFeeView struct {
	ID               string  `json:"id"`
	Label            string  `json:"label"`
	Kind             string  `json:"kind"`
	Enabled          bool    `json:"enabled"`
	FeeFlat          int64   `json:"fee_flat"`
	FeePercent       float64 `json:"fee_percent"`
	PartnerServiceID string  `json:"partner_service_id,omitempty"`
}

type dokuIntegrationView struct {
	Configured       bool                 `json:"configured"`
	Enabled          bool                 `json:"enabled"`
	Sandbox          bool                 `json:"sandbox"`
	ClientID         string               `json:"client_id"`
	SecretKey        string               `json:"secret_key,omitempty"`
	HasPrivateKey    bool                 `json:"has_private_key"`
	MerchantID       string               `json:"merchant_id"`
	TerminalID       string               `json:"terminal_id"`
	PostalCode       string               `json:"postal_code"`
	PartnerServiceID string               `json:"partner_service_id"` // legacy fallback BIN
	ExpiresInMinutes int                  `json:"expires_in_minutes"`
	QREnabled        bool                 `json:"qr_enabled"`
	SnapAuthReady    bool                 `json:"snap_auth_ready"`
	FeeMode          string               `json:"fee_mode"`
	Channels         []dokuChannelFeeView `json:"channels"`
	WebhookPath      string               `json:"webhook_path"`
	WebhookURL       string               `json:"webhook_url"`
	WebhookBaseHint  string               `json:"webhook_base_hint"`
}

type dokuChannelFeePut struct {
	ID               string  `json:"id"`
	Enabled          bool    `json:"enabled"`
	FeeFlat          int64   `json:"fee_flat"`
	FeePercent       float64 `json:"fee_percent"`
	PartnerServiceID string  `json:"partner_service_id,omitempty"`
}

type dokuIntegrationPut struct {
	Enabled          bool                `json:"enabled"`
	Sandbox          bool                `json:"sandbox"`
	ClientID         string              `json:"client_id"`
	SecretKey        string              `json:"secret_key,omitempty"`
	PrivateKey       string              `json:"private_key,omitempty"`
	MerchantID       string              `json:"merchant_id,omitempty"`
	TerminalID       string              `json:"terminal_id,omitempty"`
	PostalCode       string              `json:"postal_code,omitempty"`
	PartnerServiceID string              `json:"partner_service_id,omitempty"`
	ExpiresInMinutes int                 `json:"expires_in_minutes,omitempty"`
	FeeMode          string              `json:"fee_mode,omitempty"`
	Channels         []dokuChannelFeePut `json:"channels,omitempty"`
}

// DokuFeeModeCustomer menandai biaya admin dibebankan ke customer.
const DokuFeeModeCustomer = "customer"

// normalizeDokuFeeMode memvalidasi mode fee; selain "customer" dianggap merchant.
func normalizeDokuFeeMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), DokuFeeModeCustomer) {
		return DokuFeeModeCustomer
	}
	return ""
}

// dokuCustomerFee menghitung biaya admin legacy (fee global) — rumus sama:
// persen dari fee_flat (MDR), bukan dari nominal invoice.
func dokuCustomerFee(cfg dokuIntegrationStored, invoiceBase int64) int64 {
	if normalizeDokuFeeMode(cfg.FeeMode) != DokuFeeModeCustomer || invoiceBase <= 0 {
		return 0
	}
	return dokuFeeFromBaseMDR(cfg.FeeFlat, cfg.FeePercent)
}

type payOptionView struct {
	Provider    string `json:"provider"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Sandbox     bool   `json:"sandbox"`
	// FeeMode: merchant/customer. Channels berisi metode Direct + fee masing-masing.
	FeeMode  string               `json:"fee_mode,omitempty"`
	Channels []dokuChannelFeeView `json:"channels,omitempty"`
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
	// Chatwoot: live-chat widget di landing + portal pelanggan.
	ChatwootEnabled      bool   `json:"chatwoot_enabled"`
	ChatwootBaseURL      string `json:"chatwoot_base_url"`
	ChatwootWebsiteToken string `json:"chatwoot_website_token"`
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

type chatwootIntegrationView struct {
	Configured   bool   `json:"configured"`
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"base_url"`
	WebsiteToken string `json:"website_token,omitempty"`
}

type chatwootIntegrationPut struct {
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"base_url"`
	WebsiteToken string `json:"website_token,omitempty"`
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
		OperationID: "get-doku-integration", Method: http.MethodGet, Path: "/api/integrations/doku",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Host            string `header:"Host"`
		Origin          string `header:"Origin"`
		Referer         string `header:"Referer"`
		XForwardedHost  string `header:"X-Forwarded-Host"`
		XForwardedProto string `header:"X-Forwarded-Proto"`
	}) (*struct{ Body dokuIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadDokuIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body dokuIntegrationView }{Body: dokuView(ctx, d, tid, stored, input.Origin, input.Referer, input.XForwardedProto, input.XForwardedHost, input.Host)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-doku-integration", Method: http.MethodPut, Path: "/api/integrations/doku",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Host            string `header:"Host"`
		Origin          string `header:"Origin"`
		Referer         string `header:"Referer"`
		XForwardedHost  string `header:"X-Forwarded-Host"`
		XForwardedProto string `header:"X-Forwarded-Proto"`
		Body            dokuIntegrationPut
	}) (*struct{ Body dokuIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := loadDokuIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		clientID := strings.TrimSpace(input.Body.ClientID)
		if clientID == "" {
			clientID = strings.TrimSpace(cur.ClientID)
		}
		if input.Body.Enabled && clientID == "" {
			return nil, httpx.BadRequest("client ID DOKU wajib diisi")
		}
		if input.Body.Enabled && strings.TrimSpace(input.Body.SecretKey) == "" && cur.SecretKey == "" {
			return nil, httpx.BadRequest("secret key DOKU wajib diisi")
		}
		cur.Enabled = input.Body.Enabled
		cur.Sandbox = input.Body.Sandbox
		cur.ClientID = clientID
		if v := strings.TrimSpace(input.Body.SecretKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.SecretKey = enc
		}
		if v := strings.TrimSpace(input.Body.PrivateKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.PrivateKey = enc
		}
		if v := strings.TrimSpace(input.Body.MerchantID); v != "" {
			cur.MerchantID = v
		}
		if v := strings.TrimSpace(input.Body.TerminalID); v != "" {
			cur.TerminalID = v
		}
		if v := strings.TrimSpace(input.Body.PostalCode); v != "" {
			cur.PostalCode = v
		}
		cur.PartnerServiceID = strings.TrimSpace(input.Body.PartnerServiceID)
		if input.Body.ExpiresInMinutes > 0 {
			cur.ExpiresInMinutes = payment.ClampDokuExpiryMinutes(input.Body.ExpiresInMinutes)
		}
		cur.FeeMode = normalizeDokuFeeMode(input.Body.FeeMode)
		if len(input.Body.Channels) > 0 {
			cur.Channels = make(map[string]dokuChannelFeeStored, len(input.Body.Channels))
			for _, ch := range input.Body.Channels {
				id := strings.ToLower(strings.TrimSpace(ch.ID))
				if _, ok := payment.LookupDokuChannel(id); !ok {
					continue
				}
				flat := ch.FeeFlat
				if flat < 0 {
					flat = 0
				}
				cur.Channels[id] = dokuChannelFeeStored{
					Enabled:          ch.Enabled,
					FeeFlat:          flat,
					FeePercent:       clampFeePercent(ch.FeePercent),
					PartnerServiceID: strings.TrimSpace(ch.PartnerServiceID),
				}
			}
			// Hapus fee global legacy setelah channel tersimpan.
			cur.FeeFlat = 0
			cur.FeePercent = 0
		}
		if err := d.Store.UpsertSettingJSON(ctx, tid, settingDoku, cur); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body dokuIntegrationView }{Body: dokuView(ctx, d, tid, cur, input.Origin, input.Referer, input.XForwardedProto, input.XForwardedHost, input.Host)}, nil
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
		OperationID: "get-chatwoot-integration", Method: http.MethodGet, Path: "/api/integrations/chatwoot",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body chatwootIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadMessagingIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body chatwootIntegrationView }{Body: chatwootView(stored)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-chatwoot-integration", Method: http.MethodPut, Path: "/api/integrations/chatwoot",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body chatwootIntegrationPut
	}) (*struct{ Body chatwootIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := loadMessagingIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cur.ChatwootEnabled = input.Body.Enabled
		if v := strings.TrimRight(strings.TrimSpace(input.Body.BaseURL), "/"); v != "" {
			cur.ChatwootBaseURL = v
		}
		if v := strings.TrimSpace(input.Body.WebsiteToken); v != "" {
			cur.ChatwootWebsiteToken = v
		}
		if err := d.Store.UpsertSettingJSON(ctx, tid, settingMessaging, cur); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body chatwootIntegrationView }{Body: chatwootView(cur)}, nil
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
	path := paymentWebhookPathFor(provider)
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

func loadDokuIntegration(ctx context.Context, d *Deps, tid xid.ID) (dokuIntegrationStored, error) {
	var s dokuIntegrationStored
	err := d.Store.GetSettingJSON(ctx, tid, settingDoku, &s)
	if errors.Is(err, store.ErrNotFound) {
		return dokuIntegrationStored{}, nil
	}
	return s, err
}

func dokuQRReady(s dokuIntegrationStored, d *Deps) bool {
	if !dokuSNAPAuthReady(s, d) {
		return false
	}
	return strings.TrimSpace(s.MerchantID) != "" && strings.TrimSpace(s.TerminalID) != "" && strings.TrimSpace(s.PostalCode) != ""
}

// dokuSNAPAuthReady: client ID + RSA private key (wajib token B2B untuk VA/e-wallet/QRIS SNAP).
func dokuSNAPAuthReady(s dokuIntegrationStored, d *Deps) bool {
	if strings.TrimSpace(s.ClientID) == "" {
		return false
	}
	priv := decryptSecret(d, s.PrivateKey)
	if priv == "" {
		return false
	}
	return payment.ValidateDokuPrivateKeyPEM(priv) == nil
}

func dokuView(ctx context.Context, d *Deps, tid xid.ID, s dokuIntegrationStored, origin, referer, proto, forwardedHost, host string) dokuIntegrationView {
	webhookURL := paymentWebhookURLFor(ctx, d, tid, origin, referer, proto, forwardedHost, host, payment.ProviderDoku)
	return dokuIntegrationView{
		Configured:       strings.TrimSpace(s.ClientID) != "" && s.SecretKey != "",
		Enabled:          s.Enabled,
		Sandbox:          s.Sandbox,
		ClientID:         strings.TrimSpace(s.ClientID),
		SecretKey:        decryptSecret(d, s.SecretKey),
		HasPrivateKey:    decryptSecret(d, s.PrivateKey) != "",
		MerchantID:       strings.TrimSpace(s.MerchantID),
		TerminalID:       strings.TrimSpace(s.TerminalID),
		PostalCode:       strings.TrimSpace(s.PostalCode),
		PartnerServiceID: strings.TrimSpace(s.PartnerServiceID),
		ExpiresInMinutes: payment.ClampDokuExpiryMinutes(s.ExpiresInMinutes),
		QREnabled:        dokuQRReady(s, d),
		SnapAuthReady:    dokuSNAPAuthReady(s, d),
		FeeMode:          normalizeDokuFeeMode(s.FeeMode),
		Channels:         dokuChannelViews(s),
		WebhookPath:      paymentWebhookPathFor(payment.ProviderDoku),
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

func chatwootView(s messagingIntegrationStored) chatwootIntegrationView {
	base := strings.TrimRight(strings.TrimSpace(s.ChatwootBaseURL), "/")
	return chatwootIntegrationView{
		Configured:   base != "" && strings.TrimSpace(s.ChatwootWebsiteToken) != "",
		Enabled:      s.ChatwootEnabled,
		BaseURL:      base,
		WebsiteToken: strings.TrimSpace(s.ChatwootWebsiteToken),
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
	case "doku":
		return payment.ProviderDoku
	default:
		return name
	}
}

func listEnabledPayOptions(ctx context.Context, d *Deps, tenantID xid.ID) []payOptionView {
	out := make([]payOptionView, 0, 2)
	if cfg, _ := loadDuitkuIntegration(ctx, d, tenantID); duitkuCredentialsReady(d, cfg) {
		out = append(out, payOptionView{
			Provider:    payment.ProviderDuitku,
			Label:       "Duitku Payment Gateway",
			Description: "Halaman pembayaran Duitku (VA, e-wallet, retail, QRIS)",
			Kind:        "redirect",
			Sandbox:     cfg.Sandbox,
		})
	}
	if cfg, _ := loadDokuIntegration(ctx, d, tenantID); dokuCredentialsReady(d, cfg) {
		channels := make([]dokuChannelFeeView, 0)
		for _, ch := range dokuChannelViews(cfg) {
			if !ch.Enabled {
				continue
			}
			cat, _ := payment.LookupDokuChannel(ch.ID)
			if cat.NeedsSNAP {
				if cat.Kind == payment.DokuKindQR {
					if !dokuQRReady(cfg, d) {
						continue
					}
				} else if !dokuSNAPAuthReady(cfg, d) {
					continue
				}
			}
			if cat.NeedsVABin && dokuVABinForChannel(cfg, ch.ID) == "" {
				continue
			}
			channels = append(channels, ch)
		}
		opt := payOptionView{
			Provider:    payment.ProviderDoku,
			Label:       "DOKU",
			Description: "Bayar langsung: QRIS, VA, e-wallet, Alfamart/Indomaret",
			Kind:        "direct",
			Sandbox:     cfg.Sandbox,
			Channels:    channels,
		}
		if normalizeDokuFeeMode(cfg.FeeMode) == DokuFeeModeCustomer {
			opt.FeeMode = DokuFeeModeCustomer
		}
		if len(channels) > 0 {
			out = append(out, opt)
		}
	}
	return out
}

func duitkuCredentialsReady(d *Deps, cfg duitkuIntegrationStored) bool {
	if !cfg.Enabled {
		return false
	}
	return strings.TrimSpace(cfg.MerchantCode) != "" && decryptSecret(d, cfg.APIKey) != ""
}

func dokuCredentialsReady(d *Deps, cfg dokuIntegrationStored) bool {
	if !cfg.Enabled {
		return false
	}
	return strings.TrimSpace(cfg.ClientID) != "" && decryptSecret(d, cfg.SecretKey) != ""
}

func duitkuPaymentReady(ctx context.Context, d *Deps, tenantID xid.ID) bool {
	cfg, _ := loadDuitkuIntegration(ctx, d, tenantID)
	return duitkuCredentialsReady(d, cfg)
}

func duitkuSandboxReady(ctx context.Context, d *Deps, tenantID xid.ID) bool {
	cfg, _ := loadDuitkuIntegration(ctx, d, tenantID)
	return cfg.Sandbox && duitkuCredentialsReady(d, cfg)
}

func dokuPaymentReady(ctx context.Context, d *Deps, tenantID xid.ID) bool {
	cfg, _ := loadDokuIntegration(ctx, d, tenantID)
	return dokuCredentialsReady(d, cfg)
}

// resolvePaymentProvider resolves the gateway for the provider from the
// tenant's own integration credentials.
func resolvePaymentProvider(ctx context.Context, d *Deps, tenantID xid.ID, name string) (payment.Provider, error) {
	name = normalizePaymentProviderName(name)
	if name == payment.ProviderManual {
		return d.Payments.Get(payment.ProviderManual)
	}
	if name == payment.ProviderDuitku {
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
	if name != payment.ProviderDoku {
		return nil, httpx.BadRequest("payment gateway tidak dikenali")
	}
	cfg, _ := loadDokuIntegration(ctx, d, tenantID)
	if !cfg.Enabled {
		return nil, httpx.BadRequest("DOKU belum diaktifkan di Integrasi")
	}
	if strings.TrimSpace(cfg.ClientID) == "" || decryptSecret(d, cfg.SecretKey) == "" {
		return nil, httpx.BadRequest("DOKU belum dikonfigurasi (client ID & secret key)")
	}
	return payment.NewDokuProvider(
		cfg.ClientID,
		decryptSecret(d, cfg.SecretKey),
		decryptSecret(d, cfg.PrivateKey),
		cfg.MerchantID, cfg.TerminalID, cfg.PostalCode,
		cfg.Sandbox, cfg.ExpiresInMinutes,
	).WithPartnerServiceID(cfg.PartnerServiceID), nil
}
