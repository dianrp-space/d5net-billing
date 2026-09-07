package provision

import (
	"context"
	"time"

	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
)

type ServiceSpec struct {
	TenantID       xid.ID
	SubscriptionID xid.ID
	RouterID       xid.ID
	Username       string
	Password       string
	ServiceType    string
	ProfileName    string
	IsolirProfile  string
	IPAddress      string // static remote-address on PPP secret
	LocalAddress   string // optional PPP local-address (gateway)
	AddressPool    string // optional /ip/pool name for profile remote-address
	MACAddress     string
	Comment        string
	DownloadMbps   int // optional; when >0 Apply may set simple queue
	UploadMbps     int // optional; paired with DownloadMbps for max-limit
	// Hotspot limits (RouterOS /ip/hotspot/user)
	LimitBytesTotal int64  // from plan quota_gb
	LimitUptime     string // e.g. "1d", "12h"
	SharedUsers     int    // concurrent logins; 0 = omit
}

type Session struct {
	Username   string    `json:"username"`
	IPAddress  string    `json:"ip_address"`
	MACAddress string    `json:"mac_address,omitempty"`
	Uptime     string    `json:"uptime,omitempty"`
	RxBytes    int64     `json:"rx_bytes"`
	TxBytes    int64     `json:"tx_bytes"`
	StartedAt  time.Time `json:"started_at"`
}

type Caps struct {
	PPPoE   bool `json:"pppoe"`
	Hotspot bool `json:"hotspot"`
	DHCP    bool `json:"dhcp"`
	Queue   bool `json:"queue"`
	CoA     bool `json:"coa"`
	Backup  bool `json:"backup"`
}

type Provisioner interface {
	Apply(ctx context.Context, spec *ServiceSpec) error
	Suspend(ctx context.Context, spec *ServiceSpec) error
	Resume(ctx context.Context, spec *ServiceSpec) error
	Remove(ctx context.Context, spec *ServiceSpec) error
	Disconnect(ctx context.Context, spec *ServiceSpec) error
	ActiveSessions(ctx context.Context, routerID xid.ID) ([]Session, error)
	// TestConnection dials the device (or checks backend for RADIUS) and returns a short status message on success.
	TestConnection(ctx context.Context, routerID xid.ID) (info string, err error)
	BackupConfig(ctx context.Context, routerID xid.ID) (string, error)
	Capabilities() Caps
}

// ProfileEnsurer optionally creates/updates PPP or hotspot bandwidth profiles on a router.
// price (IDR) is embedded in the RouterOS profile comment when > 0.
type ProfileEnsurer interface {
	EnsureBandwidthProfile(ctx context.Context, tenantID, routerID xid.ID, name string, downloadMbps, uploadMbps int, serviceType, addressPool string, price int64) error
}

// IPPoolEnsurer syncs MikroTik /ip/pool from billing IPAM.
type IPPoolEnsurer interface {
	EnsureIPPool(ctx context.Context, tenantID, routerID xid.ID, name, network string, gateway *string) error
	RemoveIPPool(ctx context.Context, tenantID, routerID xid.ID, name string) error
}

// IsolirEnsurer syncs isolir pool, profile, and redirect firewall/NAT rules.
type IsolirEnsurer interface {
	EnsureIsolirInfra(ctx context.Context, tenantID, routerID xid.ID, cfg store.IsolirNetworkSettings, tenantSlug string) error
}

type Factory func(provisionerType string) (Provisioner, error)
