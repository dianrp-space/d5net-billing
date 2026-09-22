package routeros

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// PPPLive adalah status trafik live satu sesi PPPoE.
// RxBps/TxBps dalam perspektif PELANGGAN: Rx = download (masuk ke pelanggan),
// Tx = upload (keluar dari pelanggan).
type PPPLive struct {
	Online    bool   `json:"online"`
	Username  string `json:"username"`
	IPAddress string `json:"ip_address,omitempty"`
	Uptime    string `json:"uptime,omitempty"`
	Iface     string `json:"iface,omitempty"`
	RxBps     int64  `json:"rx_bps"`
	TxBps     int64  `json:"tx_bps"`
}

// PPPCounter adalah counter kumulatif interface PPPoE (untuk akumulasi bulanan).
// Sama seperti PPPLive: Rx = download pelanggan, Tx = upload pelanggan.
type PPPCounter struct {
	Username string `json:"username"`
	Iface    string `json:"iface"`
	RxBytes  int64  `json:"rx_bytes"`
	TxBytes  int64  `json:"tx_bytes"`
}

func pppIfaceName(username string) string {
	return "<pppoe-" + strings.TrimSpace(username) + ">"
}

func pppUsernameFromIface(iface string) (string, bool) {
	iface = strings.TrimSpace(iface)
	if !strings.HasPrefix(iface, "<pppoe-") || !strings.HasSuffix(iface, ">") {
		return "", false
	}
	u := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(iface, "<pppoe-"), ">"))
	if u == "" {
		return "", false
	}
	return u, true
}

// PPPLiveTraffic membaca status + rate live satu user PPPoE.
// Online=false (tanpa error) bila sesi tidak ada di /ppp/active.
func (c *Client) PPPLiveTraffic(ctx context.Context, tenantID, routerID xid.ID, username string) (*PPPLive, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username kosong")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, _, err := c.dial(ctx, tenantID, routerID)
	if err != nil {
		return nil, err
	}
	out := &PPPLive{Username: username}
	active, err := client.Run("/ppp/active/print", "?name="+username)
	if err != nil {
		return nil, err
	}
	if len(active.Re) == 0 {
		return out, nil
	}
	m := active.Re[0].Map
	out.Online = true
	out.IPAddress = strings.TrimSpace(m["address"])
	out.Uptime = strings.TrimSpace(m["uptime"])
	iface := pppIfaceName(username)
	out.Iface = iface
	mon, err := client.Run("/interface/monitor-traffic", "=interface="+iface, "=once=")
	if err != nil {
		// Sesi ada tapi interface belum terbaca (reconnect); anggap rate 0.
		return out, nil
	}
	if len(mon.Re) > 0 {
		mm := mon.Re[0].Map
		routerRx := parseIntLoose(mm["rx-bits-per-second"])
		routerTx := parseIntLoose(mm["tx-bits-per-second"])
		// /interface/monitor-traffic melapor dari sudut pandang ROUTER pada
		// interface <pppoe-USER>: rx = yang diterima router = upload user,
		// tx = yang dikirim router = download user. Balik agar API selalu
		// berbicara dalam perspektif pelanggan (Rx = download).
		out.RxBps = routerTx
		out.TxBps = routerRx
		if out.RxBps < 0 {
			out.RxBps = 0
		}
		if out.TxBps < 0 {
			out.TxBps = 0
		}
	}
	return out, nil
}

// PPPCounters membaca counter kumulatif semua interface PPPoE aktif.
// Dipakai kolektor bulanan; cukup 2 panggilan API per router.
func (c *Client) PPPCounters(ctx context.Context, tenantID, routerID xid.ID) (map[string]PPPCounter, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client, _, err := c.dial(ctx, tenantID, routerID)
	if err != nil {
		return nil, err
	}
	reply, err := client.Run("/interface/print", "?type=pppoe-in")
	if err != nil {
		return nil, err
	}
	out := make(map[string]PPPCounter, len(reply.Re))
	for _, re := range reply.Re {
		iface := strings.TrimSpace(re.Map["name"])
		u, ok := pppUsernameFromIface(iface)
		if !ok {
			continue
		}
		rx := parseIntLoose(re.Map["rx-byte"])
		tx := parseIntLoose(re.Map["tx-byte"])
		if rx < 0 {
			rx = 0
		}
		if tx < 0 {
			tx = 0
		}
		// Sama seperti PPPLiveTraffic: counter interface <pppoe-USER> bersudut
		// pandang router, jadi tukar agar Rx = download pelanggan.
		out[u] = PPPCounter{Username: u, Iface: iface, RxBytes: tx, TxBytes: rx}
	}
	return out, nil
}
