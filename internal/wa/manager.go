package wa

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

// WhatsApp validates the companion platform/OS when linking. Without these
// device props the QR/code pairing is rejected with "try again later", so we
// advertise a plain Chrome-on-Windows companion (same as the reference bot).
func init() {
	version := store.GetWAVersion()
	store.SetOSInfo("Windows", version)
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
}

// Status is the public session state for a tenant.
type Status struct {
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"logged_in"`
	JID       string `json:"jid,omitempty"`
	Phone     string `json:"phone,omitempty"`
	QRCode    string `json:"qr_code,omitempty"`
	QREvent   string `json:"qr_event,omitempty"`
	QRError   string `json:"qr_error,omitempty"`
	PairCode  string `json:"pair_code,omitempty"`
	PairPhone string `json:"pair_phone,omitempty"`
}

type session struct {
	client    *whatsmeow.Client
	container *sqlstore.Container
	mu        sync.Mutex
	qrCode    string
	qrEvent   string
	qrErr     string
	qrWatch   bool
	pairCode  string
	pairPhone string
	loginMode string
}

// markConnected clears pairing state once the device is authenticated.
func (s *session) markConnected() {
	s.mu.Lock()
	s.qrCode = ""
	s.qrEvent = "success"
	s.qrErr = ""
	s.qrWatch = false
	s.pairCode = ""
	s.pairPhone = ""
	s.mu.Unlock()
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
			if s.client.Store.ID != nil {
				s.markConnected()
			}
		case *events.LoggedOut:
			s.mu.Lock()
			s.qrCode = ""
			s.qrEvent = "logged_out"
			s.qrErr = ""
			s.qrWatch = false
			s.pairCode = ""
			s.pairPhone = ""
			s.loginMode = ""
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
	// Already paired: just make sure the socket is up.
	if s.client.Store.ID != nil {
		if !s.client.IsConnected() {
			if err := s.client.Connect(); err != nil {
				return nil, err
			}
			s.client.WaitForConnection(5 * time.Second)
		}
		return m.Status(ctx, tenantID)
	}

	// Switching back from pair-code mode needs a fresh socket.
	s.mu.Lock()
	switching := s.loginMode == "pair"
	s.mu.Unlock()
	if switching {
		if s, err = m.reset(ctx, tenantID); err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	already := s.qrWatch
	s.loginMode = "qr"
	s.mu.Unlock()
	if !already {
		ch, qerr := s.client.GetQRChannel(ctx)
		if qerr != nil {
			// Already connected/paired: fall through to normal connect.
			if !errors.Is(qerr, whatsmeow.ErrQRAlreadyConnected) && !errors.Is(qerr, whatsmeow.ErrQRStoreContainsID) {
				return nil, qerr
			}
		} else {
			s.mu.Lock()
			s.qrWatch = true
			s.qrCode = ""
			s.qrErr = ""
			s.mu.Unlock()
			go s.watchQR(ctx, ch)
		}
	}

	if !s.client.IsConnected() {
		if err := s.client.Connect(); err != nil {
			return nil, err
		}
	}
	// Wait briefly so the socket is ready and the first QR may arrive.
	s.client.WaitForConnection(5 * time.Second)
	return m.Status(ctx, tenantID)
}

// reset disconnects a session and drops it from memory (the on-disk device
// store is kept) so the next connect starts with a fresh socket.
func (m *Manager) reset(ctx context.Context, tenantID xid.ID) (*session, error) {
	m.mu.Lock()
	s, ok := m.m[tenantID.String()]
	if ok {
		delete(m.m, tenantID.String())
	}
	m.mu.Unlock()
	if ok && s != nil && s.client != nil {
		s.client.Disconnect()
	}
	return m.getOrCreate(ctx, tenantID)
}

// watchQR keeps the latest QR code/state in the session. whatsmeow emits a new
// "code" event before each code expires (auto-refresh); final events close the
// channel.
func (s *session) watchQR(ctx context.Context, ch <-chan whatsmeow.QRChannelItem) {
	defer func() {
		s.mu.Lock()
		s.qrWatch = false
		s.mu.Unlock()
	}()
	for evt := range ch {
		s.mu.Lock()
		switch evt.Event {
		case whatsmeow.QRChannelEventCode:
			s.qrCode = evt.Code
			s.qrEvent = "code"
			s.qrErr = ""
		case whatsmeow.QRChannelEventError:
			s.qrCode = ""
			s.qrEvent = "error"
			if evt.Error != nil {
				s.qrErr = evt.Error.Error()
			}
		case whatsmeow.QRChannelEventPasskeyRequest:
			s.qrEvent = "passkey-request"
			s.qrErr = "Perangkat meminta passkey. Coba login via kode."
		case whatsmeow.QRChannelEventPasskeyResponse:
			s.qrEvent = "passkey-confirmation"
		default:
			s.qrEvent = evt.Event
			if evt.Error != nil {
				s.qrErr = evt.Error.Error()
			}
			s.qrCode = ""
			if evt.Event == "success" {
				s.qrErr = ""
				s.pairCode = ""
				s.pairPhone = ""
			}
		}
		s.mu.Unlock()

		if evt.Event == whatsmeow.QRChannelEventPasskeyResponse {
			if err := s.client.SendPasskeyConfirmation(ctx); err != nil {
				slog.Warn("whatsapp passkey confirmation failed", "err", err)
			}
		}
	}
}

// PairCode links the tenant by phone number and returns an 8-character pairing
// code (no QR scan needed). The client must be connected first.
func (m *Manager) PairCode(ctx context.Context, tenantID xid.ID, phone string) (*Status, error) {
	s, err := m.getOrCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if s.client.Store.ID != nil {
		return nil, fmt.Errorf("perangkat sudah tertaut")
	}

	// Switching from QR mode needs a fresh socket.
	s.mu.Lock()
	switching := s.loginMode == "qr"
	s.mu.Unlock()
	if switching {
		if s, err = m.reset(ctx, tenantID); err != nil {
			return nil, err
		}
	}

	if !s.client.IsConnected() {
		if err := s.client.Connect(); err != nil {
			return nil, fmt.Errorf("gagal connect: %w", err)
		}
	}
	if !s.client.WaitForConnection(10 * time.Second) {
		return nil, fmt.Errorf("gagal terhubung ke WhatsApp, coba lagi")
	}
	p := normalizePairPhone(phone)
	if p == "" {
		return nil, fmt.Errorf("nomor WhatsApp wajib diisi")
	}
	code, err := s.client.PairPhone(ctx, p, true, whatsmeow.PairClientChrome, "Chrome (Windows)")
	if err != nil {
		return nil, fmt.Errorf("gagal buat kode: %w", err)
	}
	s.mu.Lock()
	s.loginMode = "pair"
	s.pairCode = code
	s.pairPhone = p
	s.qrErr = ""
	s.qrCode = ""
	s.mu.Unlock()
	return m.Status(ctx, tenantID)
}

func normalizePairPhone(phone string) string {
	p := strings.TrimSpace(phone)
	p = strings.ReplaceAll(p, " ", "")
	p = strings.ReplaceAll(p, "-", "")
	p = strings.TrimPrefix(p, "+")
	if strings.HasPrefix(p, "0") {
		p = "62" + strings.TrimPrefix(p, "0")
	}
	return p
}

// Status returns current connection/login/QR state.
func (m *Manager) Status(ctx context.Context, tenantID xid.ID) (*Status, error) {
	s, err := m.getOrCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	// Reconcile: device paired + connected but pairing state not cleared yet
	// (e.g. the QR "success" event was missed).
	if s.client.Store.ID != nil && s.client.IsConnected() {
		s.mu.Lock()
		stale := s.qrCode != "" || s.pairCode != "" || s.qrWatch
		s.mu.Unlock()
		if stale {
			s.markConnected()
		}
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
	st.QRError = s.qrErr
	st.PairCode = s.pairCode
	st.PairPhone = s.pairPhone
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
	s.qrErr = ""
	s.qrWatch = false
	s.pairCode = ""
	s.pairPhone = ""
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
