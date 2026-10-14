// Command bootstrap creates the first accounts (there is no public signup).
//
//	bootstrap platform-admin   create a super_admin
//	bootstrap finance          create a Finance user (settlement, payout and refund review)
//	bootstrap dev-merchant     create a demo merchant + owner (refused outside development)
//	bootstrap merchant         create a real merchant + owner (production-safe; names from env)
//	                           (optional ERAPAY_BOOTSTRAP_MDR_BPS + ERAPAY_BOOTSTRAP_PLATFORM_BPS assign a QRIS pricing plan)
//	bootstrap reset-password   set a new password for the existing user ERAPAY_BOOTSTRAP_EMAIL
//
// Credentials come only from ERAPAY_BOOTSTRAP_EMAIL / ERAPAY_BOOTSTRAP_PASSWORD;
// nothing secret is printed.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

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

// markVerified: accounts created here come from an operator, who vouches for the address
// (self sign-up users must verify by email instead).
func markVerified(ctx context.Context, db *pgxpool.Pool, uid string) error {
	_, err := db.Exec(ctx, `UPDATE users SET email_verified_at = now() WHERE id = $1`, uid)
	return err
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: bootstrap platform-admin|finance|merchant|dev-merchant|reset-password")
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
		if err := markVerified(ctx, db, uid); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = 'super_admin'`, uid); err != nil {
			return err
		}
		_ = audit.Record(ctx, db, audit.Entry{ActorID: uid, Action: "bootstrap.platform_admin", EntityType: "user", EntityID: uid})
	case "finance":
		uid, err := svc.CreateUser(ctx, email, pw)
		if err != nil {
			return err
		}
		if err := markVerified(ctx, db, uid); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = 'finance'`, uid); err != nil {
			return err
		}
		_ = audit.Record(ctx, db, audit.Entry{ActorID: uid, Action: "bootstrap.finance", EntityType: "user", EntityID: uid})
	case "dev-merchant":
		if env := os.Getenv("ERAPAY_ENV"); env != "" && env != "development" {
			return fmt.Errorf("dev-merchant refused when ERAPAY_ENV=%s", env)
		}
		uid, err := svc.CreateUser(ctx, email, pw)
		if err != nil {
			return err
		}
		if err := markVerified(ctx, db, uid); err != nil {
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
	case "merchant":
		// Real merchant for production onboarding (operator-run, e.g. an internal app such as ARUS).
		// Name comes from env; the owner user is ERAPAY_BOOTSTRAP_EMAIL. Created as 'approved' because
		// the operator running this command is the approver.
		legal := strings.TrimSpace(os.Getenv("ERAPAY_BOOTSTRAP_MERCHANT_LEGAL_NAME"))
		display := strings.TrimSpace(os.Getenv("ERAPAY_BOOTSTRAP_MERCHANT_NAME"))
		if legal == "" || display == "" {
			return fmt.Errorf("ERAPAY_BOOTSTRAP_MERCHANT_LEGAL_NAME and ERAPAY_BOOTSTRAP_MERCHANT_NAME are required")
		}
		outlet := strings.TrimSpace(os.Getenv("ERAPAY_BOOTSTRAP_OUTLET_NAME"))
		if outlet == "" {
			outlet = "Outlet Utama"
		}
		uid, err := svc.CreateUser(ctx, email, pw)
		if err != nil {
			return err
		}
		if err := markVerified(ctx, db, uid); err != nil {
			return err
		}
		var mid string
		if err := db.QueryRow(ctx, `INSERT INTO merchants (legal_name, display_name, onboarding_status)
				VALUES ($1,$2,'approved') RETURNING id::text`, legal, display).Scan(&mid); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `INSERT INTO merchant_outlets (merchant_id, name) VALUES ($1,$2)`, mid, outlet); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `INSERT INTO merchant_users (merchant_id, user_id, role_id)
				SELECT $1, $2, id FROM roles WHERE code = 'merchant_owner'`, mid, uid); err != nil {
			return err
		}
		// Optional pricing plan (QRIS via DOKU). Rates are a business decision, so there are no defaults:
		// both ERAPAY_BOOTSTRAP_MDR_BPS and ERAPAY_BOOTSTRAP_PLATFORM_BPS (basis points, 100 = 1%) must be set,
		// otherwise payments are refused until a plan is assigned. The MDR cap is optional too
		// (ERAPAY_BOOTSTRAP_MDR_CAP / ERAPAY_BOOTSTRAP_MDR_CAP_UPTO, minor units, 0 = uncapped).
		mdr, plat := os.Getenv("ERAPAY_BOOTSTRAP_MDR_BPS"), os.Getenv("ERAPAY_BOOTSTRAP_PLATFORM_BPS")
		if mdr != "" && plat != "" {
			mdrBPS, e1 := strconv.Atoi(mdr)
			platBPS, e2 := strconv.Atoi(plat)
			if e1 != nil || e2 != nil || mdrBPS < 0 || mdrBPS > 10000 || platBPS < 0 || platBPS > 10000 {
				return fmt.Errorf("ERAPAY_BOOTSTRAP_MDR_BPS / ERAPAY_BOOTSTRAP_PLATFORM_BPS must be integers 0..10000")
			}
			mdrCap, mdrCapUpTo := int64(0), int64(0)
			for _, e := range []struct {
				env string
				dst *int64
			}{{"ERAPAY_BOOTSTRAP_MDR_CAP", &mdrCap}, {"ERAPAY_BOOTSTRAP_MDR_CAP_UPTO", &mdrCapUpTo}} {
				if v := os.Getenv(e.env); v != "" {
					n, err := strconv.ParseInt(v, 10, 64)
					if err != nil || n < 0 {
						return fmt.Errorf("%s must be a non-negative integer (minor units)", e.env)
					}
					*e.dst = n
				}
			}
			if mdrCapUpTo > 0 && mdrCap == 0 {
				return fmt.Errorf("ERAPAY_BOOTSTRAP_MDR_CAP_UPTO requires ERAPAY_BOOTSTRAP_MDR_CAP > 0")
			}
			var planID string
			if err := db.QueryRow(ctx, `INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding, mdr_cap, mdr_cap_up_to, created_by)
					VALUES ('qris-'||$1, 1, 'doku', 'QRIS', $2, 'half_up', $3, 'half_up', $5, $6, $4) RETURNING id::text`, mid, mdrBPS, platBPS, uid, mdrCap, mdrCapUpTo).Scan(&planID); err != nil {
				return err
			}
			if _, err := db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from, created_by) VALUES ($1,$2,now(),$3)`, mid, planID, uid); err != nil {
				return err
			}
		} else {
			fmt.Fprintln(os.Stderr, "warning: no pricing plan assigned; payments will be refused until one is (set ERAPAY_BOOTSTRAP_MDR_BPS and ERAPAY_BOOTSTRAP_PLATFORM_BPS)")
		}
		_ = audit.Record(ctx, db, audit.Entry{ActorID: uid, Action: "bootstrap.merchant", EntityType: "merchant", EntityID: mid})
		fmt.Println("merchant_id:", mid) // not a secret; needed if the user belongs to several merchants at login
	case "reset-password":
		hash, err := auth.HashPassword(pw)
		if err != nil {
			return err
		}
		var uid string
		if err := db.QueryRow(ctx, `UPDATE users SET password_hash = $2 WHERE lower(email) = lower($1) RETURNING id::text`, email, hash).Scan(&uid); err != nil {
			return fmt.Errorf("user not found: %w", err)
		}
		if _, err := db.Exec(ctx, `UPDATE sessions SET revoked_at = now(), revoked_reason = 'password_reset' WHERE user_id = $1 AND revoked_at IS NULL`, uid); err != nil {
			return err
		}
		_ = audit.Record(ctx, db, audit.Entry{ActorID: uid, Action: "bootstrap.reset_password", EntityType: "user", EntityID: uid})
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	fmt.Println("ok")
	return nil
}
