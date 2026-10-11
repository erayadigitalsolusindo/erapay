// Command bootstrap creates the first accounts (there is no public signup).
//
//	bootstrap platform-admin   create a super_admin
//	bootstrap dev-merchant     create a demo merchant + owner (refused outside development)
//
// Credentials come only from ERAPAY_BOOTSTRAP_EMAIL / ERAPAY_BOOTSTRAP_PASSWORD;
// nothing secret is printed.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: bootstrap platform-admin|dev-merchant")
	}
	url, email, pw := os.Getenv("ERAPAY_DATABASE_URL"), os.Getenv("ERAPAY_BOOTSTRAP_EMAIL"), os.Getenv("ERAPAY_BOOTSTRAP_PASSWORD")
	if url == "" || email == "" || pw == "" {
		return fmt.Errorf("ERAPAY_DATABASE_URL, ERAPAY_BOOTSTRAP_EMAIL and ERAPAY_BOOTSTRAP_PASSWORD are required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer db.Close()
	svc := auth.NewService(db, auth.DefaultConfig())

	switch args[0] {
	case "platform-admin":
		uid, err := svc.CreateUser(ctx, email, pw)
		if err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = 'super_admin'`, uid); err != nil {
			return err
		}
		_ = audit.Record(ctx, db, audit.Entry{ActorID: uid, Action: "bootstrap.platform_admin", EntityType: "user", EntityID: uid})
	case "dev-merchant":
		if env := os.Getenv("ERAPAY_ENV"); env != "" && env != "development" {
			return fmt.Errorf("dev-merchant refused when ERAPAY_ENV=%s", env)
		}
		uid, err := svc.CreateUser(ctx, email, pw)
		if err != nil {
			return err
		}
		var mid string
		if err := db.QueryRow(ctx, `INSERT INTO merchants (legal_name, display_name, onboarding_status)
			VALUES ('Demo Merchant PT','Demo Merchant','approved') RETURNING id::text`).Scan(&mid); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `INSERT INTO merchant_outlets (merchant_id, name) VALUES ($1,'Outlet Utama')`, mid); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `INSERT INTO merchant_users (merchant_id, user_id, role_id)
			SELECT $1, $2, id FROM roles WHERE code = 'merchant_owner'`, mid, uid); err != nil {
			return err
		}
		_ = audit.Record(ctx, db, audit.Entry{ActorID: uid, Action: "bootstrap.dev_merchant", EntityType: "merchant", EntityID: mid})
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	fmt.Println("ok")
	return nil
}
