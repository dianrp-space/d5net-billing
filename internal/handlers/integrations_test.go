package handlers

import (
	"context"
	"testing"

	"github.com/dianrp/drp-billing/internal/auth"
	"github.com/dianrp/drp-billing/internal/xid"
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
