package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/dianrp-space/d5net-billing/internal/auth"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

func TestPublicOriginPrefersBrowserOrigin(t *testing.T) {
	got := publicOrigin("http://localhost:5173", "http://localhost:5173/app", "http", "127.0.0.1:8080", "127.0.0.1:8080")
	if got != "http://localhost:5173" {
		t.Fatalf("origin = %q", got)
	}
}

func TestOriginFromReferer(t *testing.T) {
	got := originFromReferer("http://billing.example.com:5173/admin?page=payment-gw")
	if got != "http://billing.example.com:5173" {
		t.Fatalf("referer origin = %q", got)
	}
}

func TestDuitkuViewReturnsDecryptedKeyAndCallbackURL(t *testing.T) {
	enc, err := auth.NewEncryptor("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	apiKey, err := enc.EncryptString("duitku-secret")
	if err != nil {
		t.Fatal(err)
	}
	view := duitkuView(context.Background(), &Deps{Encryptor: enc}, xid.Nil(), duitkuIntegrationStored{
		MerchantCode: "D1234",
		APIKey:       apiKey,
		Sandbox:      true,
		Enabled:      true,
	}, "http://localhost:5173", "", "", "", "127.0.0.1:8080")
	if view.APIKey != "duitku-secret" {
		t.Fatalf("api_key = %q", view.APIKey)
	}
	if !view.Configured || !view.Enabled || !view.Sandbox {
		t.Fatal("expected configured+enabled+sandbox")
	}
	wantURL := "http://localhost:5173/api/webhooks/payment/duitku"
	if view.WebhookURL != wantURL {
		t.Fatalf("webhook_url = %q, want %q", view.WebhookURL, wantURL)
	}
	if view.MerchantCode != "D1234" {
		t.Fatalf("merchant = %q", view.MerchantCode)
	}
	if normalizePaymentProviderName("pop") != "duitku" || normalizePaymentProviderName("") != "duitku" {
		t.Fatal("normalize aliases")
	}
	if paymentWebhookPathFor("duitku") != "/api/webhooks/payment/duitku" {
		t.Fatalf("duitku path = %q", paymentWebhookPathFor("duitku"))
	}
}

func TestDokuCustomerFee(t *testing.T) {
	// Merchant menanggung → 0.
	if got := dokuCustomerFee(dokuIntegrationStored{FeeMode: "", FeeFlat: 2500, FeePercent: 1}, 150000); got != 0 {
		t.Fatalf("merchant mode fee = %d, want 0", got)
	}
	// Customer: flat + persen dibulatkan.
	if got := dokuCustomerFee(dokuIntegrationStored{FeeMode: "customer", FeeFlat: 2500, FeePercent: 1}, 150000); got != 4000 {
		t.Fatalf("customer fee = %d, want 4000", got)
	}
	// Flat saja.
	if got := dokuCustomerFee(dokuIntegrationStored{FeeMode: "customer", FeeFlat: 3000}, 150000); got != 3000 {
		t.Fatalf("flat fee = %d, want 3000", got)
	}
	// Persen saja, pembulatan.
	if got := dokuCustomerFee(dokuIntegrationStored{FeeMode: "customer", FeePercent: 0.7}, 100000); got != 700 {
		t.Fatalf("percent fee = %d, want 700", got)
	}
	// Base 0 → 0.
	if got := dokuCustomerFee(dokuIntegrationStored{FeeMode: "customer", FeeFlat: 2500}, 0); got != 0 {
		t.Fatalf("zero base fee = %d, want 0", got)
	}
	if normalizeDokuFeeMode("CUSTOMER") != DokuFeeModeCustomer || normalizeDokuFeeMode("merchant") != "" {
		t.Fatal("normalizeDokuFeeMode")
	}
}

func TestSandboxSimExternalID(t *testing.T) {
	got := sandboxSimExternalID(&store.Invoice{InvoiceNumber: "INV-TES-1"})
	if !strings.Contains(got, "INV-TES-1-SIM-") {
		t.Fatalf("external id = %q", got)
	}
	if sandboxSimExternalID(nil) == "" {
		t.Fatal("empty sim id")
	}
}

func TestDokuViewReturnsDecryptedSecretsAndCallbackURL(t *testing.T) {
	enc, err := auth.NewEncryptor("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := enc.EncryptString("SK-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	priv, err := enc.EncryptString("-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----")
	if err != nil {
		t.Fatal(err)
	}
	view := dokuView(context.Background(), &Deps{Encryptor: enc}, xid.Nil(), dokuIntegrationStored{
		ClientID:   "BRN-TEST-1",
		SecretKey:  secret,
		PrivateKey: priv,
		MerchantID: "MALL-1",
		TerminalID: "T001",
		PostalCode: "28111",
		Enabled:    true,
	}, "http://localhost:5173", "", "", "", "127.0.0.1:8080")
	if view.SecretKey != "SK-test-secret" {
		t.Fatalf("secret = %q", view.SecretKey)
	}
	if !view.Configured || !view.Enabled || !view.HasPrivateKey || !view.QREnabled {
		t.Fatal("expected configured+enabled+private+qr")
	}
	wantURL := "http://localhost:5173/api/webhooks/payment/doku"
	if view.WebhookURL != wantURL {
		t.Fatalf("webhook_url = %q, want %q", view.WebhookURL, wantURL)
	}
	if normalizePaymentProviderName("doku") != "doku" {
		t.Fatal("normalize doku")
	}
	if paymentWebhookPathFor("doku") != "/api/webhooks/payment/doku" {
		t.Fatalf("doku path = %q", paymentWebhookPathFor("doku"))
	}
	if len(view.Channels) == 0 {
		t.Fatal("expected channel catalog in view")
	}
}

func TestSMTPViewReturnsDecryptedPasswordAndDefaultPort(t *testing.T) {
	enc, err := auth.NewEncryptor("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	pass, err := enc.EncryptString("smtp-secret")
	if err != nil {
		t.Fatal(err)
	}
	view := smtpView(&Deps{Encryptor: enc}, smtpIntegrationStored{
		Host:     "smtp.tenant.id",
		Username: "noreply",
		Password: pass,
		From:     "noreply@tenant.id",
		FromName: "Tenant ISP",
		Enabled:  true,
	})
	if view.Password != "smtp-secret" {
		t.Fatalf("password = %q", view.Password)
	}
	if view.Port != 587 {
		t.Fatalf("port default = %d", view.Port)
	}
	if !view.Configured || !view.Enabled {
		t.Fatal("expected configured+enabled")
	}
	if view.EnvFallback {
		t.Fatal("enabled tenant SMTP should not report env fallback")
	}
	if clampSMTPPort(0) != 587 || clampSMTPPort(65536) != 587 || clampSMTPPort(465) != 465 {
		t.Fatalf("clampSMTPPort unexpected: 0=%d 65536=%d 465=%d", clampSMTPPort(0), clampSMTPPort(65536), clampSMTPPort(465))
	}
}

func TestMessagingViewReturnsDecryptedTelegramToken(t *testing.T) {
	enc, err := auth.NewEncryptor("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	token, err := enc.EncryptString("123456:AA-saved-bot-token")
	if err != nil {
		t.Fatal(err)
	}
	view := messagingView(&Deps{Encryptor: enc}, messagingIntegrationStored{
		TelegramBotToken: token,
		TelegramChatID:   "-100123",
		TelegramEnabled:  true,
	})
	if view.TelegramBotToken != "123456:AA-saved-bot-token" {
		t.Fatalf("telegram_bot_token = %q", view.TelegramBotToken)
	}
	if !view.TelegramConfigured {
		t.Fatal("expected telegram_configured")
	}
	if view.TelegramChatID != "-100123" {
		t.Fatalf("chat_id = %q", view.TelegramChatID)
	}
}
