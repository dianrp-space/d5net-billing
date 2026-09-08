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

func TestPaymentViewReturnsDecryptedSecretsAndAppWebhookURL(t *testing.T) {
	enc, err := auth.NewEncryptor("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	apiKey, err := enc.EncryptString("drp_live_saved")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := enc.EncryptString("whsec_saved")
	if err != nil {
		t.Fatal(err)
	}
	d := &Deps{Encryptor: enc}
	view := paymentView(context.Background(), d, xid.Nil(), paymentIntegrationStored{
		APIKey:        apiKey,
		WebhookSecret: secret,
		Enabled:       true,
	}, "http://localhost:5173", "", "", "", "127.0.0.1:8080")
	if view.APIKey != "drp_live_saved" {
		t.Fatalf("api_key = %q", view.APIKey)
	}
	if view.WebhookSecret != "whsec_saved" {
		t.Fatalf("webhook_secret = %q", view.WebhookSecret)
	}
	wantURL := "http://localhost:5173/api/webhooks/payment/drp"
	if view.WebhookURL != wantURL {
		t.Fatalf("webhook_url = %q, want %q", view.WebhookURL, wantURL)
	}
	if view.ExpiresInMinutes != 15 {
		t.Fatalf("ttl default = %d", view.ExpiresInMinutes)
	}
	viewTTL := paymentView(context.Background(), d, xid.Nil(), paymentIntegrationStored{
		APIKey:           apiKey,
		WebhookSecret:    secret,
		ExpiresInMinutes: 1440,
		Enabled:          true,
	}, "http://localhost:5173", "", "", "", "127.0.0.1:8080")
	if viewTTL.ExpiresInMinutes != 1440 {
		t.Fatalf("ttl = %d", viewTTL.ExpiresInMinutes)
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
