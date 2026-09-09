package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
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
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode >= 400 {
		if desc := telegramAPIDesc(raw); desc != "" {
			return fmt.Errorf("telegram API error: %d %s", resp.StatusCode, desc)
		}
		return fmt.Errorf("telegram API error: %d", resp.StatusCode)
	}
	return nil
}

func telegramAPIDesc(raw []byte) string {
	var body struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.Description)
}

type EmailNotifier struct {
	Host     string
	Port     string
	User     string
	Pass     string
	From     string
	FromName string
}

func (n *EmailNotifier) Channel() string { return "email" }

func (n *EmailNotifier) Send(ctx context.Context, msg Message) error {
	host, port, user, pass, from, fromName := n.resolved()
	if host == "" {
		slog.Warn("email not configured", "to", msg.Recipient)
		return nil
	}
	if from == "" {
		from = "noreply@localhost"
	}
	subject := strings.TrimSpace(msg.Subject)
	if subject == "" {
		subject = "Notifikasi drp-billing"
	}
	raw := buildEmailMessage(from, fromName, msg.Recipient, subject, msg.Body)
	return sendSMTP(ctx, host, port, user, pass, from, []string{msg.Recipient}, raw)
}

func (n *EmailNotifier) resolved() (host, port, user, pass, from, fromName string) {
	if n == nil {
		n = &EmailNotifier{}
	}
	fromName = strings.TrimSpace(n.FromName)
	if host = strings.TrimSpace(n.Host); host != "" {
		return host, smtpPortOrDefault(n.Port), strings.TrimSpace(n.User), n.Pass, strings.TrimSpace(n.From), fromName
	}
	host = strings.TrimSpace(os.Getenv("SMTP_HOST"))
	port = smtpPortOrDefault(os.Getenv("SMTP_PORT"))
	user = strings.TrimSpace(os.Getenv("SMTP_USER"))
	pass = os.Getenv("SMTP_PASS")
	from = strings.TrimSpace(os.Getenv("SMTP_FROM"))
	return
}

func smtpPortOrDefault(port string) string {
	p := strings.TrimSpace(port)
	if p == "" || p == "0" {
		return "587"
	}
	return p
}

func buildEmailMessage(from, fromName, to, subject, body string) []byte {
	fromHdr := from
	if name := strings.TrimSpace(fromName); name != "" {
		fromHdr = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", name), from)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", fromHdr)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

func sendSMTP(ctx context.Context, host, port, user, pass, from string, to []string, msg []byte) error {
	addr := net.JoinHostPort(host, port)
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if port == "465" {
		return sendSMTPTLS(ctx, addr, host, auth, from, to, msg)
	}
	return smtp.SendMail(addr, auth, from, to, msg)
}

func sendSMTPTLS(ctx context.Context, addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	d := tls.Dialer{Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
