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
	// Kredensial per-environment agar sandbox & produksi tersimpan bersamaan.
	// Field legacy MerchantCode/APIKey tetap dipertahankan sebagai fallback
	// migrasi dan selalu disinkron ke env yang aktif saat save.
	SandboxMerchantCode string `json:"sandbox_merchant_code,omitempty"`
	SandboxAPIKey       string `json:"sandbox_api_key,omitempty"`
	ProdMerchantCode    string `json:"prod_merchant_code,omitempty"`
	ProdAPIKey          string `json:"prod_api_key,omitempty"`
	// FeeMode: "customer" = biaya admin ditambahkan ke tagihan; selain itu merchant.
	FeeMode    string  `json:"fee_mode"`
	FeeFlat    int64   `json:"fee_flat,omitempty"` // biaya dasar MDR
	FeePercent float64 `json:"fee_percent,omitempty"`
}

type duitkuIntegrationView struct {
	Configured       bool    `json:"configured"`
	Enabled          bool    `json:"enabled"`
	Sandbox          bool    `json:"sandbox"`
	MerchantCode     string  `json:"merchant_code"`
	APIKey           string  `json:"api_key,omitempty"`
	SandboxMerchantCode string `json:"sandbox_merchant_code"`
	SandboxAPIKey       string `json:"sandbox_api_key,omitempty"`
	ProdMerchantCode    string `json:"prod_merchant_code"`
	ProdAPIKey          string `json:"prod_api_key,omitempty"`
	SandboxConfigured bool   `json:"sandbox_configured"`
	ProdConfigured    bool   `json:"prod_configured"`
	ExpiresInMinutes int     `json:"expires_in_minutes"`
	FeeMode          string  `json:"fee_mode"`
	FeeFlat          int64   `json:"fee_flat"`
	FeePercent       float64 `json:"fee_percent"`
	WebhookPath      string  `json:"webhook_path"`
	WebhookURL       string  `json:"webhook_url"`
	WebhookBaseHint  string  `json:"webhook_base_hint"`
}

type duitkuIntegrationPut struct {
	Enabled          bool    `json:"enabled"`
	Sandbox          bool    `json:"sandbox"`
	MerchantCode     string  `json:"merchant_code,omitempty"`
	APIKey           string  `json:"api_key,omitempty"`
	SandboxMerchantCode string `json:"sandbox_merchant_code,omitempty"`
	SandboxAPIKey       string `json:"sandbox_api_key,omitempty"`
	ProdMerchantCode    string `json:"prod_merchant_code,omitempty"`
	ProdAPIKey          string `json:"prod_api_key,omitempty"`
	ExpiresInMinutes int     `json:"expires_in_minutes,omitempty"`
	FeeMode          string  `json:"fee_mode,omitempty"`
	FeeFlat          int64   `json:"fee_flat,omitempty"`
	FeePercent       float64 `json:"fee_percent,omitempty"`
}

type dokuIntegrationStored struct {
	ClientID         string `json:"client_id"`
	SecretKey        string `json:"secret_key"`
	PrivateKey       string `json:"private_key"`
	MerchantID       string `json:"merchant_id"`
	TerminalID       string `json:"terminal_id"`
	PostalCode       string `json:"postal_code"`
	PartnerServiceID string `json:"partner_service_id"`
	// QRISDirectEnabled: pakai Direct API (SNAP) QRIS saat true. Saat false
	// (default) DOKU tetap jalan lewat Checkout.
	QRISDirectEnabled bool `json:"qris_direct_enabled"`
	Sandbox           bool `json:"sandbox"`
	Enabled           bool `json:"enabled"`
	ExpiresInMinutes  int  `json:"expires_in_minutes"`
	// Kredensial per-environment agar sandbox & produksi tersimpan bersamaan.
	// Field legacy ClientID/SecretKey/PrivateKey tetap sebagai fallback migrasi
	// dan disinkron ke env yang aktif saat save. Merchant/terminal/postal/BIN
	// tetap global (shared) karena tidak diedit dari UI.
	SandboxClientID  string `json:"sandbox_client_id,omitempty"`
	SandboxSecretKey string `json:"sandbox_secret_key,omitempty"`
	SandboxPrivateKey string `json:"sandbox_private_key,omitempty"`
	ProdClientID     string `json:"prod_client_id,omitempty"`
	ProdSecretKey    string `json:"prod_secret_key,omitempty"`
	ProdPrivateKey   string `json:"prod_private_key,omitempty"`
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
	SandboxClientID  string               `json:"sandbox_client_id"`
	SandboxSecretKey string               `json:"sandbox_secret_key,omitempty"`
	ProdClientID     string               `json:"prod_client_id"`
	ProdSecretKey    string               `json:"prod_secret_key,omitempty"`
	SandboxConfigured bool                `json:"sandbox_configured"`
	ProdConfigured    bool                `json:"prod_configured"`
	HasPrivateKey    bool                 `json:"has_private_key"`
	HasSandboxPrivateKey bool             `json:"has_sandbox_private_key"`
	HasProdPrivateKey    bool             `json:"has_prod_private_key"`
	MerchantID       string               `json:"merchant_id"`
	TerminalID       string               `json:"terminal_id"`
	PostalCode       string               `json:"postal_code"`
	PartnerServiceID string               `json:"partner_service_id"` // legacy fallback BIN
	QRISDirectEnabled bool                `json:"qris_direct_enabled"`
	ExpiresInMinutes int                  `json:"expires_in_minutes"`
	QREnabled        bool                 `json:"qr_enabled"`
	SnapAuthReady    bool                 `json:"snap_auth_ready"`
	FeeMode          string               `json:"fee_mode"`
	FeeFlat          int64                `json:"fee_flat"`
	FeePercent       float64              `json:"fee_percent"`
	Channels         []dokuChannelFeeView `json:"channels,omitempty"`
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
	ClientID         string              `json:"client_id,omitempty"`
	SecretKey        string              `json:"secret_key,omitempty"`
	SandboxClientID  string              `json:"sandbox_client_id,omitempty"`
	SandboxSecretKey string              `json:"sandbox_secret_key,omitempty"`
	ProdClientID     string              `json:"prod_client_id,omitempty"`
	ProdSecretKey    string              `json:"prod_secret_key,omitempty"`
	PrivateKey       string              `json:"private_key,omitempty"`
	SandboxPrivateKey string             `json:"sandbox_private_key,omitempty"`
	ProdPrivateKey    string             `json:"prod_private_key,omitempty"`
	MerchantID       string              `json:"merchant_id,omitempty"`
	TerminalID       string              `json:"terminal_id,omitempty"`
	PostalCode       string              `json:"postal_code,omitempty"`
	PartnerServiceID string              `json:"partner_service_id,omitempty"`
	QRISDirectEnabled bool               `json:"qris_direct_enabled"`
	ExpiresInMinutes int                 `json:"expires_in_minutes,omitempty"`
	FeeMode          string              `json:"fee_mode,omitempty"`
	FeeFlat          int64               `json:"fee_flat,omitempty"`
	FeePercent       float64             `json:"fee_percent,omitempty"`
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

// dokuCustomerFee menghitung biaya admin DOKU — persen dari fee_flat (MDR),
// bukan dari nominal invoice.
func dokuCustomerFee(cfg dokuIntegrationStored, invoiceBase int64) int64 {
	if normalizeDokuFeeMode(cfg.FeeMode) != DokuFeeModeCustomer || invoiceBase <= 0 {
		return 0
	}
	return dokuFeeFromBaseMDR(cfg.FeeFlat, cfg.FeePercent)
}

// duitkuCustomerFee sama rumusnya dengan DOKU (MDR dasar × % ke customer).
func duitkuCustomerFee(cfg duitkuIntegrationStored, invoiceBase int64) int64 {
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
	// Channel = DOKU Direct API channel id (mis. "qris"); kosong = Checkout.
	Channel string `json:"channel,omitempty"`
	// FeeMode/fee_flat/fee_percent: biaya admin DOKU Checkout (MDR dasar × %).
	FeeMode    string  `json:"fee_mode,omitempty"`
	FeeFlat    int64   `json:"fee_flat,omitempty"`
	FeePercent float64 `json:"fee_percent,omitempty"`
	// Channels hanya untuk Direct API (legacy); Checkout tidak mengisinya.
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
		cur.Enabled = input.Body.Enabled
		cur.Sandbox = input.Body.Sandbox
		// Kredensial per-env: field baru diutamakan, field legacy dipetakan ke
		// env yang aktif agar klien lama tetap berfungsi.
		sandboxMerchant := strings.TrimSpace(input.Body.SandboxMerchantCode)
		prodMerchant := strings.TrimSpace(input.Body.ProdMerchantCode)
		legacyMerchant := strings.TrimSpace(input.Body.MerchantCode)
		if legacyMerchant != "" && sandboxMerchant == "" && prodMerchant == "" {
			if input.Body.Sandbox {
				sandboxMerchant = legacyMerchant
			} else {
				prodMerchant = legacyMerchant
			}
		}
		if sandboxMerchant != "" {
			cur.SandboxMerchantCode = sandboxMerchant
		}
		if prodMerchant != "" {
			cur.ProdMerchantCode = prodMerchant
		}
		if v := strings.TrimSpace(input.Body.SandboxAPIKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.SandboxAPIKey = enc
		}
		if v := strings.TrimSpace(input.Body.ProdAPIKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.ProdAPIKey = enc
		}
		if v := strings.TrimSpace(input.Body.APIKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			if input.Body.Sandbox {
				cur.SandboxAPIKey = enc
			} else {
				cur.ProdAPIKey = enc
			}
		}
		activeMerchant, activeKeyEnc := duitkuActiveCredsStored(cur)
		// Fallback: bila slot per-env masih kosong tapi legacy terisi (data lama
		// yang belum termigrasi), pakai legacy untuk validasi.
		if activeMerchant == "" {
			activeMerchant = strings.TrimSpace(cur.MerchantCode)
		}
		if activeKeyEnc == "" {
			activeKeyEnc = cur.APIKey
		}
		if input.Body.Enabled && activeMerchant == "" {
			return nil, httpx.BadRequest("merchant code Duitku wajib diisi (sandbox / produksi sesuai mode aktif)")
		}
		if input.Body.Enabled && activeKeyEnc == "" {
			return nil, httpx.BadRequest("API key Duitku wajib diisi (sandbox / produksi sesuai mode aktif)")
		}
		// Sinkronkan legacy ke env aktif untuk kompatibilitas pembaca lama.
		cur.MerchantCode = activeMerchant
		if activeKeyEnc != "" {
			cur.APIKey = activeKeyEnc
		}
		if input.Body.ExpiresInMinutes > 0 {
			cur.ExpiresInMinutes = payment.ClampDuitkuExpiryMinutes(input.Body.ExpiresInMinutes)
		}
		cur.FeeMode = normalizeDokuFeeMode(input.Body.FeeMode)
		if input.Body.FeeFlat < 0 {
			cur.FeeFlat = 0
		} else {
			cur.FeeFlat = input.Body.FeeFlat
		}
		cur.FeePercent = clampFeePercent(input.Body.FeePercent)
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
		// Kredensial per-env: field baru diutamakan, field legacy dipetakan ke
		// env yang aktif agar klien lama tetap berfungsi.
		sandboxClient := strings.TrimSpace(input.Body.SandboxClientID)
		prodClient := strings.TrimSpace(input.Body.ProdClientID)
		legacyClient := strings.TrimSpace(input.Body.ClientID)
		if legacyClient != "" && sandboxClient == "" && prodClient == "" {
			if input.Body.Sandbox {
				sandboxClient = legacyClient
			} else {
				prodClient = legacyClient
			}
		}
		if sandboxClient != "" {
			cur.SandboxClientID = sandboxClient
		}
		if prodClient != "" {
			cur.ProdClientID = prodClient
		}
		encryptTo := func(plain string, dst *string) error {
			v := strings.TrimSpace(plain)
			if v == "" {
				return nil
			}
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return err
			}
			*dst = enc
			return nil
		}
		if err := encryptTo(input.Body.SandboxSecretKey, &cur.SandboxSecretKey); err != nil {
			return nil, httpx.Internal(err)
		}
		if err := encryptTo(input.Body.ProdSecretKey, &cur.ProdSecretKey); err != nil {
			return nil, httpx.Internal(err)
		}
		if v := strings.TrimSpace(input.Body.SecretKey); v != "" {
			if input.Body.Sandbox {
				if err := encryptTo(v, &cur.SandboxSecretKey); err != nil {
					return nil, httpx.Internal(err)
				}
			} else {
				if err := encryptTo(v, &cur.ProdSecretKey); err != nil {
					return nil, httpx.Internal(err)
				}
			}
		}
		if err := encryptTo(input.Body.SandboxPrivateKey, &cur.SandboxPrivateKey); err != nil {
			return nil, httpx.Internal(err)
		}
		if err := encryptTo(input.Body.ProdPrivateKey, &cur.ProdPrivateKey); err != nil {
			return nil, httpx.Internal(err)
		}
		if v := strings.TrimSpace(input.Body.PrivateKey); v != "" {
			if input.Body.Sandbox {
				if err := encryptTo(v, &cur.SandboxPrivateKey); err != nil {
					return nil, httpx.Internal(err)
				}
			} else {
				if err := encryptTo(v, &cur.ProdPrivateKey); err != nil {
					return nil, httpx.Internal(err)
				}
			}
		}
		activeClient, activeSecretEnc, _ := dokuActiveCredsStored(cur)
		if activeClient == "" {
			activeClient = strings.TrimSpace(cur.ClientID)
		}
		if activeSecretEnc == "" {
			activeSecretEnc = cur.SecretKey
		}
		if input.Body.Enabled && activeClient == "" {
			return nil, httpx.BadRequest("client ID DOKU wajib diisi (sandbox / produksi sesuai mode aktif)")
		}
		if input.Body.Enabled && activeSecretEnc == "" {
			return nil, httpx.BadRequest("secret key DOKU wajib diisi (sandbox / produksi sesuai mode aktif)")
		}
		// Sinkronkan legacy ke env aktif untuk kompatibilitas pembaca lama.
		cur.ClientID = activeClient
		if activeSecretEnc != "" {
			cur.SecretKey = activeSecretEnc
		}
		if _, _, activePriv := dokuActiveCredsStored(cur); activePriv != "" {
			cur.PrivateKey = activePriv
		} else if strings.TrimSpace(cur.PrivateKey) == "" {
			// Pertahankan legacy bila kedua slot per-env masih kosong.
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
		cur.QRISDirectEnabled = input.Body.QRISDirectEnabled
		if input.Body.ExpiresInMinutes > 0 {
			cur.ExpiresInMinutes = payment.ClampDokuExpiryMinutes(input.Body.ExpiresInMinutes)
		}
		cur.FeeMode = normalizeDokuFeeMode(input.Body.FeeMode)
		if input.Body.FeeFlat < 0 {
			cur.FeeFlat = 0
		} else {
			cur.FeeFlat = input.Body.FeeFlat
		}
		cur.FeePercent = clampFeePercent(input.Body.FeePercent)
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
		return duitkuIntegrationStored{Sandbox: true}, nil
	}
	if err != nil {
		return s, err
	}
	migrateDuitkuLegacy(&s)
	return s, nil
}

// migrateDuitkuLegacy menyalin kredensial lama satu-slot ke slot per-env agar
// tenant lama tidak kehilangan kredensial saat upgrade.
func migrateDuitkuLegacy(s *duitkuIntegrationStored) {
	if s == nil {
		return
	}
	legacyMerchant := strings.TrimSpace(s.MerchantCode)
	if legacyMerchant != "" {
		if s.Sandbox {
			if strings.TrimSpace(s.SandboxMerchantCode) == "" {
				s.SandboxMerchantCode = legacyMerchant
			}
		} else {
			if strings.TrimSpace(s.ProdMerchantCode) == "" {
				s.ProdMerchantCode = legacyMerchant
			}
		}
		if strings.TrimSpace(s.SandboxMerchantCode) == "" && strings.TrimSpace(s.ProdMerchantCode) == "" {
			// Data sangat lama tanpa flag jelas: isi kedua slot agar tidak hilang.
			s.SandboxMerchantCode = legacyMerchant
			s.ProdMerchantCode = legacyMerchant
		}
	}
	if strings.TrimSpace(s.APIKey) != "" {
		if s.Sandbox {
			if strings.TrimSpace(s.SandboxAPIKey) == "" {
				s.SandboxAPIKey = s.APIKey
			}
		} else {
			if strings.TrimSpace(s.ProdAPIKey) == "" {
				s.ProdAPIKey = s.APIKey
			}
		}
		if strings.TrimSpace(s.SandboxAPIKey) == "" && strings.TrimSpace(s.ProdAPIKey) == "" {
			s.SandboxAPIKey = s.APIKey
			s.ProdAPIKey = s.APIKey
		}
	}
}

// duitkuActiveCredsStored mengembalikan merchant + apiKey terenkripsi untuk env aktif.
func duitkuActiveCredsStored(cfg duitkuIntegrationStored) (merchant, apiKeyEnc string) {
	if cfg.Sandbox {
		return strings.TrimSpace(cfg.SandboxMerchantCode), cfg.SandboxAPIKey
	}
	return strings.TrimSpace(cfg.ProdMerchantCode), cfg.ProdAPIKey
}

func duitkuSandboxCreds(d *Deps, cfg duitkuIntegrationStored) (merchant, apiKey string) {
	merchant = strings.TrimSpace(cfg.SandboxMerchantCode)
	if merchant == "" && cfg.Sandbox {
		merchant = strings.TrimSpace(cfg.MerchantCode)
	}
	apiKey = decryptSecret(d, cfg.SandboxAPIKey)
	if apiKey == "" && cfg.Sandbox {
		apiKey = decryptSecret(d, cfg.APIKey)
	}
	return merchant, apiKey
}

func duitkuProdCreds(d *Deps, cfg duitkuIntegrationStored) (merchant, apiKey string) {
	merchant = strings.TrimSpace(cfg.ProdMerchantCode)
	if merchant == "" && !cfg.Sandbox {
		merchant = strings.TrimSpace(cfg.MerchantCode)
	}
	apiKey = decryptSecret(d, cfg.ProdAPIKey)
	if apiKey == "" && !cfg.Sandbox {
		apiKey = decryptSecret(d, cfg.APIKey)
	}
	return merchant, apiKey
}

func duitkuActiveCreds(d *Deps, cfg duitkuIntegrationStored) (merchant, apiKey string) {
	if cfg.Sandbox {
		return duitkuSandboxCreds(d, cfg)
	}
	return duitkuProdCreds(d, cfg)
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
	activeMerchant, activeKey := duitkuActiveCreds(d, s)
	sandboxMerchant, sandboxKey := duitkuSandboxCreds(d, s)
	prodMerchant, prodKey := duitkuProdCreds(d, s)
	webhookURL := paymentWebhookURLFor(ctx, d, tid, origin, referer, proto, forwardedHost, host, payment.ProviderDuitku)
	return duitkuIntegrationView{
		Configured:       activeMerchant != "" && activeKey != "",
		Enabled:          s.Enabled,
		Sandbox:          s.Sandbox,
		MerchantCode:     activeMerchant,
		APIKey:           activeKey,
		SandboxMerchantCode: sandboxMerchant,
		SandboxAPIKey:       sandboxKey,
		ProdMerchantCode:    prodMerchant,
		ProdAPIKey:          prodKey,
		SandboxConfigured: sandboxMerchant != "" && sandboxKey != "",
		ProdConfigured:    prodMerchant != "" && prodKey != "",
		ExpiresInMinutes: payment.ClampDuitkuExpiryMinutes(s.ExpiresInMinutes),
		FeeMode:          normalizeDokuFeeMode(s.FeeMode),
		FeeFlat:          s.FeeFlat,
		FeePercent:       s.FeePercent,
		WebhookPath:      paymentWebhookPathFor(payment.ProviderDuitku),
		WebhookURL:       webhookURL,
		WebhookBaseHint:  webhookURL,
	}
}

func loadDokuIntegration(ctx context.Context, d *Deps, tid xid.ID) (dokuIntegrationStored, error) {
	var s dokuIntegrationStored
	err := d.Store.GetSettingJSON(ctx, tid, settingDoku, &s)
	if errors.Is(err, store.ErrNotFound) {
		return dokuIntegrationStored{Sandbox: true}, nil
	}
	if err != nil {
		return s, err
	}
	migrateDokuLegacy(&s)
	return s, nil
}

// migrateDokuLegacy menyalin kredensial lama satu-slot ke slot per-env.
func migrateDokuLegacy(s *dokuIntegrationStored) {
	if s == nil {
		return
	}
	legacyClient := strings.TrimSpace(s.ClientID)
	if legacyClient != "" {
		if s.Sandbox {
			if strings.TrimSpace(s.SandboxClientID) == "" {
				s.SandboxClientID = legacyClient
			}
		} else {
			if strings.TrimSpace(s.ProdClientID) == "" {
				s.ProdClientID = legacyClient
			}
		}
		if strings.TrimSpace(s.SandboxClientID) == "" && strings.TrimSpace(s.ProdClientID) == "" {
			s.SandboxClientID = legacyClient
			s.ProdClientID = legacyClient
		}
	}
	if strings.TrimSpace(s.SecretKey) != "" {
		if s.Sandbox {
			if strings.TrimSpace(s.SandboxSecretKey) == "" {
				s.SandboxSecretKey = s.SecretKey
			}
		} else {
			if strings.TrimSpace(s.ProdSecretKey) == "" {
				s.ProdSecretKey = s.SecretKey
			}
		}
		if strings.TrimSpace(s.SandboxSecretKey) == "" && strings.TrimSpace(s.ProdSecretKey) == "" {
			s.SandboxSecretKey = s.SecretKey
			s.ProdSecretKey = s.SecretKey
		}
	}
	if strings.TrimSpace(s.PrivateKey) != "" {
		if s.Sandbox {
			if strings.TrimSpace(s.SandboxPrivateKey) == "" {
				s.SandboxPrivateKey = s.PrivateKey
			}
		} else {
			if strings.TrimSpace(s.ProdPrivateKey) == "" {
				s.ProdPrivateKey = s.PrivateKey
			}
		}
	}
}

// dokuActiveCredsStored mengembalikan clientID + secret terenkripsi + private
// terenkripsi untuk env aktif.
func dokuActiveCredsStored(cfg dokuIntegrationStored) (clientID, secretEnc, privateEnc string) {
	if cfg.Sandbox {
		return strings.TrimSpace(cfg.SandboxClientID), cfg.SandboxSecretKey, cfg.SandboxPrivateKey
	}
	return strings.TrimSpace(cfg.ProdClientID), cfg.ProdSecretKey, cfg.ProdPrivateKey
}

func dokuSandboxCreds(d *Deps, cfg dokuIntegrationStored) (clientID, secret, priv string) {
	clientID = strings.TrimSpace(cfg.SandboxClientID)
	if clientID == "" && cfg.Sandbox {
		clientID = strings.TrimSpace(cfg.ClientID)
	}
	secret = decryptSecret(d, cfg.SandboxSecretKey)
	if secret == "" && cfg.Sandbox {
		secret = decryptSecret(d, cfg.SecretKey)
	}
	priv = decryptSecret(d, cfg.SandboxPrivateKey)
	if priv == "" && cfg.Sandbox {
		priv = decryptSecret(d, cfg.PrivateKey)
	}
	return clientID, secret, priv
}

func dokuProdCreds(d *Deps, cfg dokuIntegrationStored) (clientID, secret, priv string) {
	clientID = strings.TrimSpace(cfg.ProdClientID)
	if clientID == "" && !cfg.Sandbox {
		clientID = strings.TrimSpace(cfg.ClientID)
	}
	secret = decryptSecret(d, cfg.ProdSecretKey)
	if secret == "" && !cfg.Sandbox {
		secret = decryptSecret(d, cfg.SecretKey)
	}
	priv = decryptSecret(d, cfg.ProdPrivateKey)
	if priv == "" && !cfg.Sandbox {
		priv = decryptSecret(d, cfg.PrivateKey)
	}
	return clientID, secret, priv
}

func dokuActiveCreds(d *Deps, cfg dokuIntegrationStored) (clientID, secret, priv string) {
	if cfg.Sandbox {
		return dokuSandboxCreds(d, cfg)
	}
	return dokuProdCreds(d, cfg)
}

func dokuQRReady(s dokuIntegrationStored, d *Deps) bool {
	if !dokuSNAPAuthReady(s, d) {
		return false
	}
	return strings.TrimSpace(s.MerchantID) != "" && strings.TrimSpace(s.TerminalID) != "" && strings.TrimSpace(s.PostalCode) != ""
}

// dokuQRDirectEnabled: toggle Direct API QRIS aktif DAN kredensialnya siap.
// Saat false, DOKU tetap dipakai lewat Checkout dan QRIS tidak muncul di portal.
func dokuQRDirectEnabled(s dokuIntegrationStored, d *Deps) bool {
	return s.QRISDirectEnabled && dokuQRReady(s, d)
}

// dokuSNAPAuthReady: client ID (env aktif) + RSA private key (env aktif, fallback
// legacy) wajib untuk token B2B VA/e-wallet/QRIS SNAP.
func dokuSNAPAuthReady(s dokuIntegrationStored, d *Deps) bool {
	clientID, _, priv := dokuActiveCreds(d, s)
	if clientID == "" {
		clientID = strings.TrimSpace(s.ClientID)
	}
	if priv == "" {
		priv = decryptSecret(d, s.PrivateKey)
	}
	if priv == "" {
		// Fallback: private key di env mana pun (UI lama hanya simpan satu).
		if p := decryptSecret(d, s.SandboxPrivateKey); p != "" {
			priv = p
		} else if p := decryptSecret(d, s.ProdPrivateKey); p != "" {
			priv = p
		}
	}
	if clientID == "" || priv == "" {
		return false
	}
	return payment.ValidateDokuPrivateKeyPEM(priv) == nil
}

func dokuView(ctx context.Context, d *Deps, tid xid.ID, s dokuIntegrationStored, origin, referer, proto, forwardedHost, host string) dokuIntegrationView {
	webhookURL := paymentWebhookURLFor(ctx, d, tid, origin, referer, proto, forwardedHost, host, payment.ProviderDoku)
	activeClient, activeSecret, activePriv := dokuActiveCreds(d, s)
	sandboxClient, sandboxSecret, sandboxPriv := dokuSandboxCreds(d, s)
	prodClient, prodSecret, prodPriv := dokuProdCreds(d, s)
	hasPriv := activePriv != ""
	if !hasPriv {
		hasPriv = decryptSecret(d, s.PrivateKey) != ""
	}
	return dokuIntegrationView{
		Configured:       activeClient != "" && activeSecret != "",
		Enabled:          s.Enabled,
		Sandbox:          s.Sandbox,
		ClientID:         activeClient,
		SecretKey:        activeSecret,
		SandboxClientID:  sandboxClient,
		SandboxSecretKey: sandboxSecret,
		ProdClientID:     prodClient,
		ProdSecretKey:    prodSecret,
		SandboxConfigured: sandboxClient != "" && sandboxSecret != "",
		ProdConfigured:    prodClient != "" && prodSecret != "",
		HasPrivateKey:    hasPriv,
		HasSandboxPrivateKey: sandboxPriv != "",
		HasProdPrivateKey:    prodPriv != "",
		MerchantID:       strings.TrimSpace(s.MerchantID),
		TerminalID:       strings.TrimSpace(s.TerminalID),
		PostalCode:       strings.TrimSpace(s.PostalCode),
		PartnerServiceID: strings.TrimSpace(s.PartnerServiceID),
		QRISDirectEnabled: s.QRISDirectEnabled,
		ExpiresInMinutes: payment.ClampDokuExpiryMinutes(s.ExpiresInMinutes),
		QREnabled:        dokuQRReady(s, d),
		SnapAuthReady:    dokuSNAPAuthReady(s, d),
		FeeMode:          normalizeDokuFeeMode(s.FeeMode),
		FeeFlat:          s.FeeFlat,
		FeePercent:       s.FeePercent,
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
		opt := payOptionView{
			Provider:    payment.ProviderDuitku,
			Label:       "DUITKU",
			Description: "Halaman pembayaran Duitku (VA, e-wallet, retail, QRIS)",
			Kind:        "redirect",
			Sandbox:     cfg.Sandbox,
		}
		if normalizeDokuFeeMode(cfg.FeeMode) == DokuFeeModeCustomer {
			opt.FeeMode = DokuFeeModeCustomer
			opt.FeeFlat = cfg.FeeFlat
			opt.FeePercent = cfg.FeePercent
		}
		out = append(out, opt)
	}
	if cfg, _ := loadDokuIntegration(ctx, d, tenantID); dokuCredentialsReady(d, cfg) {
		opt := payOptionView{
			Provider:    payment.ProviderDoku,
			Label:       "DOKU",
			Description: "Halaman bayar DOKU (VA, e-wallet, QRIS, retail)",
			Kind:        "redirect",
			Sandbox:     cfg.Sandbox,
		}
		if normalizeDokuFeeMode(cfg.FeeMode) == DokuFeeModeCustomer {
			opt.FeeMode = DokuFeeModeCustomer
			opt.FeeFlat = cfg.FeeFlat
			opt.FeePercent = cfg.FeePercent
		}
		out = append(out, opt)
		// Direct API QRIS: tampil sebagai metode terpisah yang langsung
		// menampilkan QR (tanpa redirect), hanya bila toggle-nya aktif & siap.
		if dokuQRDirectEnabled(cfg, d) {
			qrOpt := payOptionView{
				Provider:    payment.ProviderDoku,
				Label:       "QRIS",
				Description: "Scan QRIS langsung dari aplikasi (tanpa pindah halaman)",
				Kind:        "qr",
				Channel:     "qris",
				Sandbox:     cfg.Sandbox,
			}
			if normalizeDokuFeeMode(cfg.FeeMode) == DokuFeeModeCustomer {
				flat, pct := dokuChannelFeeConfig(cfg, "qris")
				qrOpt.FeeMode = DokuFeeModeCustomer
				qrOpt.FeeFlat = flat
				qrOpt.FeePercent = pct
			}
			out = append(out, qrOpt)
		}
	}
	return out
}

func duitkuCredentialsReady(d *Deps, cfg duitkuIntegrationStored) bool {
	if !cfg.Enabled {
		return false
	}
	merchant, apiKey := duitkuActiveCreds(d, cfg)
	return merchant != "" && apiKey != ""
}

func dokuCredentialsReady(d *Deps, cfg dokuIntegrationStored) bool {
	if !cfg.Enabled {
		return false
	}
	clientID, secret, _ := dokuActiveCreds(d, cfg)
	return clientID != "" && secret != ""
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
// tenant's own integration credentials (env aktif sesuai toggle sandbox).
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
		merchant, apiKey := duitkuActiveCreds(d, cfg)
		if merchant == "" || apiKey == "" {
			return nil, httpx.BadRequest("Duitku POP belum dikonfigurasi (isi kredensial sandbox / produksi sesuai mode aktif)")
		}
		return payment.NewDuitkuProvider(merchant, apiKey, cfg.Sandbox, cfg.ExpiresInMinutes), nil
	}
	if name != payment.ProviderDoku {
		return nil, httpx.BadRequest("payment gateway tidak dikenali")
	}
	cfg, _ := loadDokuIntegration(ctx, d, tenantID)
	if !cfg.Enabled {
		return nil, httpx.BadRequest("DOKU belum diaktifkan di Integrasi")
	}
	clientID, secret, priv := dokuActiveCreds(d, cfg)
	if clientID == "" || secret == "" {
		return nil, httpx.BadRequest("DOKU belum dikonfigurasi (client ID & secret key sandbox / produksi sesuai mode aktif)")
	}
	if priv == "" {
		priv = decryptSecret(d, cfg.PrivateKey)
	}
	return payment.NewDokuProvider(
		clientID,
		secret,
		priv,
		cfg.MerchantID, cfg.TerminalID, cfg.PostalCode,
		cfg.Sandbox, cfg.ExpiresInMinutes,
	).WithPartnerServiceID(cfg.PartnerServiceID), nil
}

// resolvePaymentProviderForEnv membangun provider untuk env tertentu (bukan
// env aktif). Dipakai untuk verifikasi webhook dua-env: callback yang dibuat
// saat sandbox tetap valid walau mode sudah dipindah ke produksi, dan sebaliknya.
func resolvePaymentProviderForEnv(ctx context.Context, d *Deps, tenantID xid.ID, name string, sandbox bool) (payment.Provider, error) {
	name = normalizePaymentProviderName(name)
	if name == payment.ProviderManual {
		return d.Payments.Get(payment.ProviderManual)
	}
	if name == payment.ProviderDuitku {
		cfg, _ := loadDuitkuIntegration(ctx, d, tenantID)
		if !cfg.Enabled {
			return nil, httpx.BadRequest("Duitku POP belum diaktifkan di Integrasi")
		}
		var merchant, apiKey string
		if sandbox {
			merchant, apiKey = duitkuSandboxCreds(d, cfg)
		} else {
			merchant, apiKey = duitkuProdCreds(d, cfg)
		}
		if merchant == "" || apiKey == "" {
			return nil, httpx.BadRequest("Duitku POP belum dikonfigurasi untuk env tersebut")
		}
		return payment.NewDuitkuProvider(merchant, apiKey, sandbox, cfg.ExpiresInMinutes), nil
	}
	if name != payment.ProviderDoku {
		return nil, httpx.BadRequest("payment gateway tidak dikenali")
	}
	cfg, _ := loadDokuIntegration(ctx, d, tenantID)
	if !cfg.Enabled {
		return nil, httpx.BadRequest("DOKU belum diaktifkan di Integrasi")
	}
	var clientID, secret, priv string
	if sandbox {
		clientID, secret, priv = dokuSandboxCreds(d, cfg)
	} else {
		clientID, secret, priv = dokuProdCreds(d, cfg)
	}
	if clientID == "" || secret == "" {
		return nil, httpx.BadRequest("DOKU belum dikonfigurasi untuk env tersebut")
	}
	if priv == "" {
		priv = decryptSecret(d, cfg.PrivateKey)
	}
	return payment.NewDokuProvider(
		clientID, secret, priv,
		cfg.MerchantID, cfg.TerminalID, cfg.PostalCode,
		sandbox, cfg.ExpiresInMinutes,
	).WithPartnerServiceID(cfg.PartnerServiceID), nil
}

// verifyPaymentWebhookBothEnvs mencoba verifikasi dengan env aktif dulu, lalu
// env satunya. Mengembalikan provider yang berhasil verifikasi.
func verifyPaymentWebhookBothEnvs(ctx context.Context, d *Deps, tenantID xid.ID, providerName string, headers map[string]string, raw []byte) (payment.Provider, *payment.WebhookEvent, error) {
	prov, err := resolvePaymentProvider(ctx, d, tenantID, providerName)
	if err != nil {
		return nil, nil, err
	}
	if ev, verr := prov.VerifyWebhook(ctx, headers, raw); verr == nil {
		return prov, ev, nil
	} else {
		// Simpan error env aktif; coba env satunya sebelum menyerah.
		activeIsSandbox := true
		switch normalizePaymentProviderName(providerName) {
		case payment.ProviderDuitku:
			if cfg, _ := loadDuitkuIntegration(ctx, d, tenantID); true {
				activeIsSandbox = cfg.Sandbox
			}
		case payment.ProviderDoku:
			if cfg, _ := loadDokuIntegration(ctx, d, tenantID); true {
				activeIsSandbox = cfg.Sandbox
			}
		}
		if alt, aerr := resolvePaymentProviderForEnv(ctx, d, tenantID, providerName, !activeIsSandbox); aerr == nil {
			if ev, verr2 := alt.VerifyWebhook(ctx, headers, raw); verr2 == nil {
				return alt, ev, nil
			}
		}
		return nil, nil, verr
	}
}
