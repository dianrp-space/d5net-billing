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
	MidtransServerKey  string `json:"midtrans_server_key"`
	XenditSecretKey    string `json:"xendit_secret_key"`
	TripayPrivateKey   string `json:"tripay_private_key"`
	TripayMerchantCode string `json:"tripay_merchant_code"`
	MidtransEnabled    bool   `json:"midtrans_enabled"`
	XenditEnabled      bool   `json:"xendit_enabled"`
	TripayEnabled      bool   `json:"tripay_enabled"`
}

type paymentIntegrationView struct {
	MidtransConfigured bool   `json:"midtrans_configured"`
	XenditConfigured   bool   `json:"xendit_configured"`
	TripayConfigured   bool   `json:"tripay_configured"`
	MidtransEnabled    bool   `json:"midtrans_enabled"`
	XenditEnabled      bool   `json:"xendit_enabled"`
	TripayEnabled      bool   `json:"tripay_enabled"`
	TripayMerchantCode string `json:"tripay_merchant_code"`
	// Empty on GET; send new value on PUT (blank = keep existing)
	MidtransServerKey string `json:"midtrans_server_key,omitempty"`
	XenditSecretKey   string `json:"xendit_secret_key,omitempty"`
	TripayPrivateKey  string `json:"tripay_private_key,omitempty"`
	WebhookBaseHint   string `json:"webhook_base_hint"`
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
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body paymentIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		stored, err := loadPaymentIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body paymentIntegrationView }{Body: paymentView(stored)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-payment-integration", Method: http.MethodPut, Path: "/api/integrations/payment",
		Tags: []string{"Integrations"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body paymentIntegrationView
	}) (*struct{ Body paymentIntegrationView }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := loadPaymentIntegration(ctx, d, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cur.MidtransEnabled = input.Body.MidtransEnabled
		cur.XenditEnabled = input.Body.XenditEnabled
		cur.TripayEnabled = input.Body.TripayEnabled
		cur.TripayMerchantCode = strings.TrimSpace(input.Body.TripayMerchantCode)
		if v := strings.TrimSpace(input.Body.MidtransServerKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.MidtransServerKey = enc
		}
		if v := strings.TrimSpace(input.Body.XenditSecretKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.XenditSecretKey = enc
		}
		if v := strings.TrimSpace(input.Body.TripayPrivateKey); v != "" {
			enc, err := d.Encryptor.EncryptString(v)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			cur.TripayPrivateKey = enc
		}
		if err := d.Store.UpsertSettingJSON(ctx, tid, settingPayment, cur); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body paymentIntegrationView }{Body: paymentView(cur)}, nil
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
	Enabled      bool   `json:"enabled"`
	Connected    bool   `json:"connected"`
	LoggedIn     bool   `json:"logged_in"`
	JID          string `json:"jid,omitempty"`
	Phone        string `json:"phone,omitempty"`
	QRCode       string `json:"qr_code,omitempty"`
	QREvent      string `json:"qr_event,omitempty"`
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

func paymentView(s paymentIntegrationStored) paymentIntegrationView {
	return paymentIntegrationView{
		MidtransConfigured: s.MidtransServerKey != "",
		XenditConfigured:   s.XenditSecretKey != "",
		TripayConfigured:   s.TripayPrivateKey != "",
		MidtransEnabled:    s.MidtransEnabled,
		XenditEnabled:      s.XenditEnabled,
		TripayEnabled:      s.TripayEnabled,
		TripayMerchantCode: s.TripayMerchantCode,
		WebhookBaseHint:    "/api/webhooks/payment/{midtrans|xendit|tripay}",
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

// resolvePaymentProvider prefers tenant integration keys, then falls back to env registry.
func resolvePaymentProvider(ctx context.Context, d *Deps, tenantID xid.ID, name string) (payment.Provider, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = "manual"
	}
	if name == "manual" {
		return d.Payments.Get("manual")
	}
	cfg, _ := loadPaymentIntegration(ctx, d, tenantID)
	switch name {
	case "midtrans":
		key := decryptSecret(d, cfg.MidtransServerKey)
		if key != "" {
			if !cfg.MidtransEnabled {
				return nil, httpx.BadRequest("midtrans belum diaktifkan di Integrasi")
			}
			return &payment.MidtransProvider{ServerKey: key}, nil
		}
		return d.Payments.Get("midtrans")
	case "xendit":
		key := decryptSecret(d, cfg.XenditSecretKey)
		if key != "" {
			if !cfg.XenditEnabled {
				return nil, httpx.BadRequest("xendit belum diaktifkan di Integrasi")
			}
			return &payment.XenditProvider{SecretKey: key}, nil
		}
		return d.Payments.Get("xendit")
	case "tripay":
		key := decryptSecret(d, cfg.TripayPrivateKey)
		if key != "" {
			if !cfg.TripayEnabled {
				return nil, httpx.BadRequest("tripay belum diaktifkan di Integrasi")
			}
			return &payment.TripayProvider{PrivateKey: key, MerchantCode: cfg.TripayMerchantCode}, nil
		}
		return d.Payments.Get("tripay")
	default:
		return nil, fmt.Errorf("unknown payment provider: %s", name)
	}
}
