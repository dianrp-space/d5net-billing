package importer

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
	_ "github.com/go-sql-driver/mysql"
)

// ImportMySQLCustomers copies customers from a legacy MySQL ISP billing database
// that exposes a tbl_customers table (username, fullname, phonenumber, email).
func ImportMySQLCustomers(ctx context.Context, mysqlDSN string, st *store.Store, tenantID xid.ID) error {
	db, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		if strings.Contains(err.Error(), "unknown driver") || strings.Contains(err.Error(), "forgotten import") {
			return fmt.Errorf("%w — install with: go get github.com/go-sql-driver/mysql", err)
		}
		return fmt.Errorf("ping mysql: %w", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT username, fullname, phonenumber, email FROM tbl_customers`)
	if err != nil {
		return fmt.Errorf("query customers (need tbl_customers): %w", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var username, name, phone, email string
		if err := rows.Scan(&username, &name, &phone, &email); err != nil {
			continue
		}
		c := &store.Customer{
			TenantID: tenantID, CustomerCode: username, FullName: name, Phone: phone, Email: &email, IsActive: true, PortalEnabled: true,
		}
		if err := st.CreateCustomer(ctx, c); err != nil {
			slog.Warn("import customer skipped", "username", username, "err", err)
			continue
		}
		n++
	}
	slog.Info("imported mysql customers", "count", n)
	return nil
}
