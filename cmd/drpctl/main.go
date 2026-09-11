package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dianrp-space/d5net-billing/internal/auth"
	"github.com/dianrp-space/d5net-billing/internal/config"
	"github.com/dianrp-space/d5net-billing/internal/db"
	"github.com/dianrp-space/d5net-billing/internal/importer"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	st := store.New(database.Pool)

	switch os.Args[1] {
	case "create-admin":
		createAdmin(ctx, st, os.Args[2:])
	case "create-tenant":
		createTenant(ctx, st, os.Args[2:])
	case "set-password":
		setPassword(ctx, st, os.Args[2:])
	case "import-mysql-customers":
		importMySQLCustomers(ctx, st, os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`drpctl commands:
  create-tenant         --slug SLUG --name NAME --email EMAIL --password PASS --full-name NAME
  create-admin          --email EMAIL --password PASS --full-name NAME --tenant-id UUID
  set-password          --email EMAIL --password PASS
  import-mysql-customers --mysql-dsn DSN --tenant-id UUID`)
}

func importMySQLCustomers(ctx context.Context, st *store.Store, args []string) {
	fs := flag.NewFlagSet("import-mysql-customers", flag.ExitOnError)
	dsn := fs.String("mysql-dsn", "", "MySQL DSN (user:pass@tcp(host:3306)/dbname)")
	tenantID := fs.String("tenant-id", "", "target tenant UUID")
	_ = fs.Parse(args)
	tid, err := xid.Parse(*tenantID)
	if *dsn == "" || err != nil || xid.IsNil(tid) {
		log.Fatal("--mysql-dsn and --tenant-id (UUID) required")
	}
	err = importer.ImportMySQLCustomers(ctx, *dsn, st, tid)
	if err != nil {
		if strings.Contains(err.Error(), "unknown driver") || strings.Contains(err.Error(), "forgotten import") {
			fmt.Fprintln(os.Stderr, "mysql driver missing: install with `go get github.com/go-sql-driver/mysql`")
			os.Exit(1)
		}
		log.Fatal(err)
	}
	fmt.Println("MySQL customer import completed")
}

func createTenant(ctx context.Context, st *store.Store, args []string) {
	fs := flag.NewFlagSet("create-tenant", flag.ExitOnError)
	slug := fs.String("slug", "", "")
	name := fs.String("name", "", "")
	email := fs.String("email", "", "")
	password := fs.String("password", "", "")
	fullName := fs.String("full-name", "Admin", "")
	_ = fs.Parse(args)

	hash, err := auth.HashPassword(*password)
	if err != nil {
		log.Fatal(err)
	}
	tid, uid, err := st.CreateTenantWithAdmin(ctx, *slug, *name, *email, hash, *fullName)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created tenant id=%s user id=%s\n", tid, uid)
	fmt.Println("Admin login: /admin/login")
	fmt.Println("Client login: /login")
}

func createAdmin(ctx context.Context, st *store.Store, args []string) {
	fs := flag.NewFlagSet("create-admin", flag.ExitOnError)
	email := fs.String("email", "", "")
	password := fs.String("password", "", "")
	fullName := fs.String("full-name", "Admin", "")
	tenantID := fs.String("tenant-id", "", "")
	_ = fs.Parse(args)

	hash, err := auth.HashPassword(*password)
	if err != nil {
		log.Fatal(err)
	}
	tid, err := xid.Parse(*tenantID)
	if err != nil || xid.IsNil(tid) {
		log.Fatal("--tenant-id UUID required")
	}
	var userID xid.ID
	err = st.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name) VALUES ($1,$2,$3) RETURNING id
	`, *email, hash, *fullName).Scan(&userID)
	if err != nil {
		log.Fatal(err)
	}
	var roleID xid.ID
	err = st.Pool.QueryRow(ctx, `SELECT id FROM roles WHERE tenant_id=$1 AND slug='admin'`, tid).Scan(&roleID)
	if err != nil {
		log.Fatal(err)
	}
	_, err = st.Pool.Exec(ctx, `INSERT INTO user_tenants (user_id, tenant_id, role_id) VALUES ($1,$2,$3)`, userID, tid, roleID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created admin user id=%s for tenant %s\n", userID, tid)
}

func setPassword(ctx context.Context, st *store.Store, args []string) {
	fs := flag.NewFlagSet("set-password", flag.ExitOnError)
	email := fs.String("email", "", "")
	password := fs.String("password", "", "")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	if *email == "" || *password == "" {
		log.Fatal("--email and --password required")
	}
	hash, err := auth.HashPassword(*password)
	if err != nil {
		log.Fatal(err)
	}
	if err := st.UpdateUserPassword(ctx, *email, hash); err != nil {
		log.Fatal(err)
	}
	fmt.Println("password updated")
}
