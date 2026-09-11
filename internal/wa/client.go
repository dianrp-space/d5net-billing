// Package wa is a thin HTTP client for an external WhatsApp gateway
// (go-whatsapp-web-multidevice / GOWA). No WhatsApp library runs in-process:
// each tenant configures a gateway base URL plus Basic Auth credentials.
package wa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

type Config struct {
	BaseURL  string
	Username string
	Password string
	DeviceID string
}

type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.DeviceID = strings.TrimSpace(cfg.DeviceID)
	return &Client{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Configured() bool {
	return c != nil && c.cfg.BaseURL != ""
}

// Status mirrors the gateway's GET /app/status results.
type Status struct {
	Connected bool   `json:"is_connected"`
	LoggedIn  bool   `json:"is_logged_in"`
	DeviceID  string `json:"device_id"`
	JID       string `json:"jid"`
}

// CheckStatus calls GET /app/status to verify base URL + credentials.
func (c *Client) CheckStatus(ctx context.Context) (*Status, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("whatsapp gateway belum dikonfigurasi")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/app/status", nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("whatsapp gateway %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Results Status `json:"results"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse status gateway: %w", err)
	}
	return &out.Results, nil
}

// Device is one registered WhatsApp session on the gateway.
type Device struct {
	DeviceID string `json:"device"`
	Name     string `json:"name"`
	JID      string `json:"jid"`
}

// ListDevices calls GET /app/devices to enumerate sessions on the gateway.
func (c *Client) ListDevices(ctx context.Context) ([]Device, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("whatsapp gateway belum dikonfigurasi")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/app/devices", nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("whatsapp gateway %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Results []Device `json:"results"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse devices gateway: %w", err)
	}
	return out.Results, nil
}

// SendText calls POST /send/message with phone + message.
func (c *Client) SendText(ctx context.Context, phone, body string) error {
	if !c.Configured() {
		return fmt.Errorf("whatsapp gateway belum dikonfigurasi")
	}
	p := NormalizePhone(phone)
	if p == "" {
		return fmt.Errorf("nomor WhatsApp kosong")
	}
	payload, _ := json.Marshal(map[string]any{"phone": p, "message": body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/send/message", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("whatsapp gateway %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// SendImage calls POST /send/image (multipart) with PNG/JPEG bytes.
func (c *Client) SendImage(ctx context.Context, phone, caption string, png []byte) error {
	return c.sendMedia(ctx, "/send/image", "image", phone, caption, "qris.png", "image/png", png)
}

// SendFile calls POST /send/file (multipart), e.g. for invoice PDFs.
func (c *Client) SendFile(ctx context.Context, phone, caption, filename, mime string, data []byte) error {
	return c.sendMedia(ctx, "/send/file", "file", phone, caption, filename, mime, data)
}

func (c *Client) sendMedia(ctx context.Context, path, field, phone, caption, filename, mime string, data []byte) error {
	if !c.Configured() {
		return fmt.Errorf("whatsapp gateway belum dikonfigurasi")
	}
	p := NormalizePhone(phone)
	if p == "" {
		return fmt.Errorf("nomor WhatsApp kosong")
	}
	if len(data) == 0 {
		return fmt.Errorf("media kosong")
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("phone", p)
	if strings.TrimSpace(caption) != "" {
		_ = w.WriteField("caption", caption)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
	if mime == "" {
		mime = "application/octet-stream"
	}
	h.Set("Content-Type", mime)
	fw, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("whatsapp gateway %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (c *Client) applyHeaders(req *http.Request) {
	if c.cfg.DeviceID != "" {
		req.Header.Set("X-Device-Id", c.cfg.DeviceID)
	}
	if c.cfg.Username != "" {
		req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	}
}

// NormalizePhone strips non-digits and converts a leading 0 to 62 (Indonesia).
func NormalizePhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	p := b.String()
	if strings.HasPrefix(p, "0") {
		p = "62" + p[1:]
	}
	return p
}
