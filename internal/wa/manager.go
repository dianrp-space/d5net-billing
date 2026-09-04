package wa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
	_ "modernc.org/sqlite"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// Status is the public session state for a tenant.
type Status struct {
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"logged_in"`
	JID       string `json:"jid,omitempty"`
	Phone     string `json:"phone,omitempty"`
	QRCode    string `json:"qr_code,omitempty"`
	QREvent   string `json:"qr_event,omitempty"`
}

type session struct {
	client    *whatsmeow.Client
	container *sqlstore.Container
	mu        sync.Mutex
	qrCode    string
	qrEvent   string
}

// Manager holds per-tenant whatsmeow clients (sqlite session files, no CGO).
type Manager struct {
	dir string
	log waLog.Logger
	mu  sync.Mutex
	m   map[string]*session
}

func NewManager(sessionDir string) (*Manager, error) {
	if sessionDir == "" {
		sessionDir = "./data/whatsapp"
	}
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, err
	}
	return &Manager{
		dir: sessionDir,
		log: waLog.Stdout("WA", "INFO", true),
		m:   make(map[string]*session),
	}, nil
}

func (m *Manager) dbPath(tenantID xid.ID) string {
	return filepath.Join(m.dir, tenantID.String()+".db")
}

func (m *Manager) getOrCreate(ctx context.Context, tenantID xid.ID) (*session, error) {
	key := tenantID.String()
	m.mu.Lock()
	if s, ok := m.m[key]; ok {
		m.mu.Unlock()
		return s, nil
	}
	m.mu.Unlock()

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", m.dbPath(tenantID))
	container, err := sqlstore.New(ctx, "sqlite", dsn, m.log)
	if err != nil {
		return nil, fmt.Errorf("whatsmeow store: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("whatsmeow device: %w", err)
	}
	client := whatsmeow.NewClient(device, m.log)
	s := &session{client: client, container: container}
	client.AddEventHandler(func(evt any) {
		switch evt.(type) {
		case *events.Connected:
			s.mu.Lock()
			s.qrCode = ""
			s.qrEvent = "success"
			s.mu.Unlock()
		case *events.LoggedOut:
			s.mu.Lock()
			s.qrCode = ""
			s.qrEvent = "logged_out"
			s.mu.Unlock()
		}
	})

	m.mu.Lock()
	if existing, ok := m.m[key]; ok {
		m.mu.Unlock()
		client.Disconnect()
		return existing, nil
	}
	m.m[key] = s
	m.mu.Unlock()
	return s, nil
}

// Connect starts/reuses the client. If not logged in, begins QR pairing.
func (m *Manager) Connect(ctx context.Context, tenantID xid.ID) (*Status, error) {
	s, err := m.getOrCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if s.client.IsConnected() && s.client.IsLoggedIn() {
		return m.Status(ctx, tenantID)
	}

	needQR := s.client.Store.ID == nil
	var qrChan <-chan whatsmeow.QRChannelItem
	if needQR {
		ch, err := s.client.GetQRChannel(ctx)
		if err != nil {
			return nil, err
		}
		qrChan = ch
		go func() {
			for evt := range qrChan {
				s.mu.Lock()
				s.qrEvent = evt.Event
				if evt.Event == "code" {
					s.qrCode = evt.Code
				}
				if evt.Event == "success" || evt.Event == "timeout" || evt.Event == "err-client" {
					if evt.Event != "success" {
						s.qrCode = ""
					}
				}
				s.mu.Unlock()
			}
		}()
	}

	if !s.client.IsConnected() {
		if err := s.client.Connect(); err != nil {
			return nil, err
		}
	}
	// brief wait so first QR may arrive
	if needQR {
		time.Sleep(400 * time.Millisecond)
	}
	return m.Status(ctx, tenantID)
}

// Status returns current connection/login/QR state.
func (m *Manager) Status(ctx context.Context, tenantID xid.ID) (*Status, error) {
	s, err := m.getOrCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	st := &Status{
		Connected: s.client.IsConnected(),
		LoggedIn:  s.client.IsLoggedIn(),
	}
	if s.client.Store.ID != nil {
		st.JID = s.client.Store.ID.String()
		st.Phone = s.client.Store.ID.User
	}
	s.mu.Lock()
	st.QRCode = s.qrCode
	st.QREvent = s.qrEvent
	s.mu.Unlock()
	return st, nil
}

// Logout disconnects and removes the device pairing.
func (m *Manager) Logout(ctx context.Context, tenantID xid.ID) error {
	s, err := m.getOrCreate(ctx, tenantID)
	if err != nil {
		return err
	}
	if s.client.IsLoggedIn() {
		_ = s.client.Logout(ctx)
	}
	s.client.Disconnect()
	s.mu.Lock()
	s.qrCode = ""
	s.qrEvent = "logged_out"
	s.mu.Unlock()

	m.mu.Lock()
	delete(m.m, tenantID.String())
	m.mu.Unlock()

	_ = os.Remove(m.dbPath(tenantID))
	_ = os.Remove(m.dbPath(tenantID) + "-wal")
	_ = os.Remove(m.dbPath(tenantID) + "-shm")
	return nil
}

// IsReady reports whether the tenant session can send messages.
func (m *Manager) IsReady(tenantID xid.ID) bool {
	m.mu.Lock()
	s, ok := m.m[tenantID.String()]
	m.mu.Unlock()
	if !ok || s == nil || s.client == nil {
		// try lazy open existing db without connect
		path := m.dbPath(tenantID)
		if _, err := os.Stat(path); err != nil {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		st, err := m.Status(ctx, tenantID)
		if err != nil {
			return false
		}
		if !st.LoggedIn {
			return false
		}
		_, _ = m.Connect(ctx, tenantID)
		m.mu.Lock()
		s = m.m[tenantID.String()]
		m.mu.Unlock()
	}
	return s != nil && s.client != nil && s.client.IsLoggedIn()
}

// SendText sends a plain WhatsApp text message to phone (E.164-ish digits).
func (m *Manager) SendText(ctx context.Context, tenantID xid.ID, phone, body string) error {
	s, err := m.getOrCreate(ctx, tenantID)
	if err != nil {
		return err
	}
	if !s.client.IsLoggedIn() {
		return fmt.Errorf("whatsapp belum login untuk tenant ini")
	}
	if !s.client.IsConnected() {
		if err := s.client.Connect(); err != nil {
			return err
		}
	}
	jid, err := parsePhoneJID(phone)
	if err != nil {
		return err
	}
	msg := &waE2E.Message{Conversation: proto.String(body)}
	_, err = s.client.SendMessage(ctx, jid, msg)
	return err
}

func parsePhoneJID(phone string) (types.JID, error) {
	p := strings.TrimSpace(phone)
	p = strings.ReplaceAll(p, " ", "")
	p = strings.ReplaceAll(p, "-", "")
	p = strings.TrimPrefix(p, "+")
	if strings.HasPrefix(p, "0") {
		p = "62" + strings.TrimPrefix(p, "0")
	}
	if p == "" {
		return types.JID{}, fmt.Errorf("nomor kosong")
	}
	return types.NewJID(p, types.DefaultUserServer), nil
}

// RestoreConnectedTenants reconnects sessions that already have a device store on disk.
func (m *Manager) RestoreConnectedTenants(ctx context.Context) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".db") {
			continue
		}
		idStr := strings.TrimSuffix(name, ".db")
		tid, err := xid.Parse(idStr)
		if err != nil {
			continue
		}
		st, err := m.Status(ctx, tid)
		if err != nil || !st.LoggedIn {
			continue
		}
		_, _ = m.Connect(ctx, tid)
	}
}
