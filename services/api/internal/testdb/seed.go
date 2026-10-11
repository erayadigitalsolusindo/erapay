package testdb

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedMerchant inserts a merchant with one outlet and returns (merchantID, outletID).
func SeedMerchant(t testing.TB, db *pgxpool.Pool, name string) (string, string) {
	t.Helper()
	var mid, oid string
	ctx := context.Background()
	if err := db.QueryRow(ctx,
		`INSERT INTO merchants (legal_name, display_name, onboarding_status)
		 VALUES ($1, $1, 'approved') RETURNING id::text`, name).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx,
		`INSERT INTO merchant_outlets (merchant_id, name) VALUES ($1, $2) RETURNING id::text`,
		mid, name+" outlet").Scan(&oid); err != nil {
		t.Fatal(err)
	}
	return mid, oid
}

// AddMember grants user a merchant-scoped role (e.g. "merchant_owner", "cashier").
func AddMember(t testing.TB, db *pgxpool.Pool, merchantID, userID, roleCode string) {
	t.Helper()
	if _, err := db.Exec(context.Background(),
		`INSERT INTO merchant_users (merchant_id, user_id, role_id)
		 SELECT $1, $2, id FROM roles WHERE code = $3 AND scope = 'merchant'`,
		merchantID, userID, roleCode); err != nil {
		t.Fatal(err)
	}
}

// AddPlatformRole grants user a platform role (e.g. "super_admin").
func AddPlatformRole(t testing.TB, db *pgxpool.Pool, userID, roleCode string) {
	t.Helper()
	if _, err := db.Exec(context.Background(),
		`INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = $2 AND scope = 'platform'`,
		userID, roleCode); err != nil {
		t.Fatal(err)
	}
}
