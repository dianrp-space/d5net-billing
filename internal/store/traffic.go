package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

// MonthlyUsage adalah total pemakaian satu pelanggan dalam satu bulan.
type MonthlyUsage struct {
	CustomerID     xid.ID `json:"customer_id"`
	CustomerCode   string  `json:"customer_code,omitempty"`
	Month          string  `json:"month"`
	RxBytes        int64   `json:"rx_bytes"`
	TxBytes        int64   `json:"tx_bytes"`
	TotalBytes     int64   `json:"total_bytes"`
}

// TrafficMonthKey mengembalikan tanggal 1 awal bulan (UTC) untuk agregasi.
func TrafficMonthKey(t time.Time) time.Time {
	y, m, _ := t.UTC().Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// trafficDelta menghitung selisih counter dengan aman terhadap reset
// (interface PPPoE dibuat ulang saat sesi putus → counter mulai dari 0 lagi).
func trafficDelta(current, last int64) int64 {
	if current < 0 {
		current = 0
	}
	if last < 0 {
		last = 0
	}
	if current < last {
		return current
	}
	return current - last
}

// AccumulateTrafficSample mencatat satu sampel counter dan menambahkan
// selisihnya ke total bulan berjalan. Idempoten terhadap reconnect.
func (s *Store) AccumulateTrafficSample(ctx context.Context, tenantID, routerID, customerID, subscriptionID xid.ID, username string, rx, tx int64, at time.Time) error {
	username = strings.TrimSpace(username)
	if xid.IsNil(tenantID) || xid.IsNil(routerID) || xid.IsNil(customerID) || username == "" {
		return nil
	}
	if rx < 0 {
		rx = 0
	}
	if tx < 0 {
		tx = 0
	}
	month := TrafficMonthKey(at)
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	return s.withTenant(ctx, tenantID, func(ctx context.Context, txConn pgx.Tx) error {
		var lastRx, lastTx int64
		err := txConn.QueryRow(ctx, `
			SELECT rx_bytes, tx_bytes FROM traffic_last_sample
			WHERE tenant_id=$1 AND router_id=$2 AND username=$3
		`, tenantID, routerID, username).Scan(&lastRx, &lastTx)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		dRx := trafficDelta(rx, lastRx)
		dTx := trafficDelta(tx, lastTx)
		if dRx > 0 || dTx > 0 {
			if _, err := txConn.Exec(ctx, `
				INSERT INTO traffic_monthly (tenant_id, customer_id, subscription_id, username, month, rx_bytes, tx_bytes, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())
				ON CONFLICT (tenant_id, customer_id, month) DO UPDATE SET
					rx_bytes = traffic_monthly.rx_bytes + EXCLUDED.rx_bytes,
					tx_bytes = traffic_monthly.tx_bytes + EXCLUDED.tx_bytes,
					subscription_id = COALESCE(EXCLUDED.subscription_id, traffic_monthly.subscription_id),
					username = EXCLUDED.username,
					updated_at = NOW()
			`, tenantID, customerID, nullXID(subscriptionID), username, month, dRx, dTx); err != nil {
				return err
			}
			// Rincian harian untuk grafik portal (delta yang sama, tanpa polling tambahan).
			if _, err := txConn.Exec(ctx, `
				INSERT INTO traffic_daily (tenant_id, customer_id, day, rx_bytes, tx_bytes, updated_at)
				VALUES ($1,$2,$3,$4,$5,NOW())
				ON CONFLICT (tenant_id, customer_id, day) DO UPDATE SET
					rx_bytes = traffic_daily.rx_bytes + EXCLUDED.rx_bytes,
					tx_bytes = traffic_daily.tx_bytes + EXCLUDED.tx_bytes,
					updated_at = NOW()
			`, tenantID, customerID, day, dRx, dTx); err != nil {
				return err
			}
		}
		_, err = txConn.Exec(ctx, `
			INSERT INTO traffic_last_sample (tenant_id, router_id, username, rx_bytes, tx_bytes, sampled_at)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (tenant_id, router_id, username) DO UPDATE SET
				rx_bytes = EXCLUDED.rx_bytes,
				tx_bytes = EXCLUDED.tx_bytes,
				sampled_at = EXCLUDED.sampled_at
		`, tenantID, routerID, username, rx, tx, at)
		return err
	})
}

func nullXID(id xid.ID) any {
	if xid.IsNil(id) {
		return nil
	}
	return id
}

// MonthlyUsage mengembalikan total pemakaian satu pelanggan pada bulan tertentu.
// month kosong = bulan berjalan.
func (s *Store) MonthlyUsage(ctx context.Context, tenantID, customerID xid.ID, month time.Time) (*MonthlyUsage, error) {
	if month.IsZero() {
		month = s.TenantNow(ctx, tenantID)
	}
	month = TrafficMonthKey(month)
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	out := &MonthlyUsage{CustomerID: customerID, Month: month.Format("2006-01")}
	err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(rx_bytes),0), COALESCE(SUM(tx_bytes),0)
		FROM traffic_monthly WHERE tenant_id=$1 AND customer_id=$2 AND month=$3
	`, tenantID, customerID, month).Scan(&out.RxBytes, &out.TxBytes)
	if err != nil {
		return nil, err
	}
	out.TotalBytes = out.RxBytes + out.TxBytes
	return out, nil
}

// MonthlyUsageMany mengembalikan pemakaian beberapa pelanggan sekaligus
// (dipakai portal multi-akun). month kosong = bulan berjalan.
func (s *Store) MonthlyUsageMany(ctx context.Context, tenantID xid.ID, customerIDs []xid.ID, month time.Time) ([]MonthlyUsage, error) {
	if len(customerIDs) == 0 {
		return []MonthlyUsage{}, nil
	}
	if month.IsZero() {
		month = s.TenantNow(ctx, tenantID)
	}
	month = TrafficMonthKey(month)
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT tm.customer_id, COALESCE(c.customer_code,''), tm.month,
		       COALESCE(SUM(tm.rx_bytes),0), COALESCE(SUM(tm.tx_bytes),0)
		FROM traffic_monthly tm
		LEFT JOIN customers c ON c.id = tm.customer_id AND c.tenant_id = tm.tenant_id
		WHERE tm.tenant_id=$1 AND tm.customer_id = ANY($2) AND tm.month=$3
		GROUP BY tm.customer_id, c.customer_code, tm.month
	`, tenantID, customerIDs, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []MonthlyUsage{}
	for rows.Next() {
		var u MonthlyUsage
		var m time.Time
		if err := rows.Scan(&u.CustomerID, &u.CustomerCode, &m, &u.RxBytes, &u.TxBytes); err != nil {
			return nil, err
		}
		u.Month = m.Format("2006-01")
		u.TotalBytes = u.RxBytes + u.TxBytes
		list = append(list, u)
	}
	return list, rows.Err()
}

// DailyUsage adalah total pemakaian satu hari (digabung semua akun bila multi).
type DailyUsage struct {
	Day        string `json:"day"`
	RxBytes    int64  `json:"rx_bytes"`
	TxBytes    int64  `json:"tx_bytes"`
	TotalBytes int64  `json:"total_bytes"`
}

// DailyUsageMonth mengembalikan rincian harian untuk satu bulan (format "2006-01"),
// diurut dari tanggal 1. Hari tanpa pemakaian tidak muncul (frontend mengisi nol).
func (s *Store) DailyUsageMonth(ctx context.Context, tenantID xid.ID, customerIDs []xid.ID, month string) ([]DailyUsage, error) {
	list := []DailyUsage{}
	if len(customerIDs) == 0 {
		return list, nil
	}
	parsed, err := time.Parse("2006-01", strings.TrimSpace(month))
	if err != nil {
		return list, nil
	}
	start := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return list, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT day, COALESCE(SUM(rx_bytes),0), COALESCE(SUM(tx_bytes),0)
		FROM traffic_daily
		WHERE tenant_id=$1 AND customer_id = ANY($2) AND day >= $3 AND day < $4
		GROUP BY day ORDER BY day
	`, tenantID, customerIDs, start, end)
	if err != nil {
		return list, err
	}
	defer rows.Close()
	for rows.Next() {
		var u DailyUsage
		var d time.Time
		if err := rows.Scan(&d, &u.RxBytes, &u.TxBytes); err != nil {
			return list, err
		}
		u.Day = d.Format("2006-01-02")
		u.TotalBytes = u.RxBytes + u.TxBytes
		list = append(list, u)
	}
	return list, rows.Err()
}

// LiveSub adalah langganan PPPoE aktif milik pelanggan untuk live traffic.
type LiveSub struct {
	SubscriptionID xid.ID `json:"subscription_id"`
	CustomerID     xid.ID `json:"customer_id"`
	RouterID       xid.ID `json:"router_id"`
	RouterName     string `json:"router_name,omitempty"`
	Username       string `json:"username"`
	ServiceType    string `json:"service_type"`
	Status         string `json:"status"`
}

// CustomerLiveSubs mengembalikan langganan PPPoE pelanggan yang terhubung ke
// router (untuk tombol Live traffic admin). Dibatasi ke PPPoE dulu.
func (s *Store) CustomerLiveSubs(ctx context.Context, tenantID, customerID xid.ID) ([]LiveSub, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT s.id, s.customer_id, s.router_id, COALESCE(r.name,''),
		       s.username, s.service_type, s.status
		FROM subscriptions s
		LEFT JOIN routers r ON r.id = s.router_id AND r.tenant_id = s.tenant_id
		WHERE s.tenant_id=$1 AND s.customer_id=$2 AND s.router_id IS NOT NULL
		  AND s.service_type='pppoe' AND s.status IN ('active','overdue','suspended')
		ORDER BY s.id DESC
	`, tenantID, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []LiveSub{}
	for rows.Next() {
		var l LiveSub
		if err := rows.Scan(&l.SubscriptionID, &l.CustomerID, &l.RouterID, &l.RouterName, &l.Username, &l.ServiceType, &l.Status); err != nil {
			return nil, err
		}
		list = append(list, l)
	}
	return list, rows.Err()
}

// SubscriptionByRouterUsername memetakan sesi router ke langganan billing
// (untuk akumulasi trafik ke pelanggan yang benar).
func (s *Store) SubscriptionByRouterUsername(ctx context.Context, tenantID, routerID xid.ID, username string) (subID, customerID xid.ID, err error) {
	username = strings.TrimSpace(username)
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return xid.Nil(), xid.Nil(), err
	}
	err = s.Pool.QueryRow(ctx, `
		SELECT id, customer_id FROM subscriptions
		WHERE tenant_id=$1 AND router_id=$2 AND username=$3
		  AND status IN ('active','overdue','suspended')
		ORDER BY CASE status WHEN 'active' THEN 0 WHEN 'overdue' THEN 1 ELSE 2 END, id DESC
		LIMIT 1
	`, tenantID, routerID, username).Scan(&subID, &customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return xid.Nil(), xid.Nil(), ErrNotFound
	}
	return subID, customerID, err
}
