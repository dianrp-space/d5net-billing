package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"os"
	"time"
)

type WhatsAppNotifier struct {
	APIURL string
	APIKey string
}

func (n *WhatsAppNotifier) Channel() string { return "whatsapp" }

func (n *WhatsAppNotifier) Send(ctx context.Context, msg Message) error {
	apiURL := n.APIURL
	if apiURL == "" {
		apiURL = os.Getenv("WHATSAPP_API_URL")
	}
	apiKey := n.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("WHATSAPP_API_KEY")
	}
	if apiURL == "" {
		slog.Warn("whatsapp not configured, logging message", "to", msg.Recipient, "body", msg.Body)
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"phone": msg.Recipient, "message": msg.Body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/send", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("whatsapp API error: %d", resp.StatusCode)
	}
	return nil
}

type TelegramNotifier struct {
	BotToken string
}

func (n *TelegramNotifier) Channel() string { return "telegram" }

func (n *TelegramNotifier) Send(ctx context.Context, msg Message) error {
	token := n.BotToken
	if token == "" {
		token = os.Getenv("TELEGRAM_BOT_TOKEN")
	}
	if token == "" {
		slog.Warn("telegram not configured", "to", msg.Recipient)
		return nil
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload, _ := json.Marshal(map[string]string{"chat_id": msg.Recipient, "text": msg.Body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type EmailNotifier struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

func (n *EmailNotifier) Channel() string { return "email" }

func (n *EmailNotifier) Send(ctx context.Context, msg Message) error {
	host := n.Host
	if host == "" {
		host = os.Getenv("SMTP_HOST")
	}
	if host == "" {
		slog.Warn("email not configured", "to", msg.Recipient)
		return nil
	}
	subject := msg.Subject
	if subject == "" {
		subject = "Notifikasi drp-billing"
	}
	body := fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s", msg.Recipient, subject, msg.Body)
	addr := host + ":" + os.Getenv("SMTP_PORT")
	auth := smtp.PlainAuth("", os.Getenv("SMTP_USER"), os.Getenv("SMTP_PASS"), host)
	return smtp.SendMail(addr, auth, os.Getenv("SMTP_FROM"), []string{msg.Recipient}, []byte(body))
}
