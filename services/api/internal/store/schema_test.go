package store_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/erayadigitalsolusindo/erapay/services/api/migrations"
)

// Test ini menyerang constraint database sungguhan. Dilewati bila ERAPAY_TEST_DATABASE_URL kosong.
// PERHATIAN: schema public database itu DIHAPUS lalu dibuat ulang; nama DB wajib mengandung "test".
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("ERAPAY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ERAPAY_TEST_DATABASE_URL tidak diset")
	}
	if !strings.Contains(url, "test") {
		t.Fatal("nama database uji harus mengandung 'test' (perlindungan data)")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("exec gagal: %v\n%s", err, q)
	}
}

func mustFail(t *testing.T, db *sql.DB, why, q string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), q, args...); err == nil {
		t.Fatalf("seharusnya ditolak database: %s", why)
	}
}

const insertPayment = `INSERT INTO payments
  (id, merchant_id, merchant_reference, gateway, payment_method, currency, amount, mdr_fee, platform_fee, merchant_net, gateway_reference, status)
  VALUES ($1, $2, $3, 'doku', 'QRIS', 'IDR', $4, $5, $6, $7, $8, $9)`

func TestSchemaInvariants(t *testing.T) {
	db := openTestDB(t)
	mA := "11111111-1111-1111-1111-111111111111"
	mB := "22222222-2222-2222-2222-222222222222"
	mustExec(t, db, `INSERT INTO merchants (id, code, legal_name) VALUES ($1,'a','A'), ($2,'b','B')`, mA, mB)

	t.Run("fee 1000 = 7 + 10 + 983 diterima", func(t *testing.T) {
		mustExec(t, db, insertPayment, "a0000000-0000-0000-0000-000000000001", mA, "r1", 1000, 7, 10, 983, "gw-1", "pending")
	})
	t.Run("selisih fee ditolak", func(t *testing.T) {
		mustFail(t, db, "gross != jumlah komponen", insertPayment,
			"a0000000-0000-0000-0000-000000000002", mA, "r2", 1000, 7, 10, 982, nil, "pending")
	})
	t.Run("nominal nol/negatif ditolak", func(t *testing.T) {
		mustFail(t, db, "amount 0", insertPayment, "a0000000-0000-0000-0000-000000000003", mA, "r3", 0, 0, 0, 0, nil, "pending")
		mustFail(t, db, "amount negatif", insertPayment, "a0000000-0000-0000-0000-000000000004", mA, "r4", -1000, 7, 10, -1017, nil, "pending")
	})
	t.Run("gateway_reference unik per gateway", func(t *testing.T) {
		mustFail(t, db, "duplikat (gateway, gateway_reference)", insertPayment,
			"a0000000-0000-0000-0000-000000000005", mB, "r5", 1000, 7, 10, 983, "gw-1", "pending")
	})
	t.Run("snapshot nominal/fee immutable", func(t *testing.T) {
		mustFail(t, db, "ubah amount", `UPDATE payments SET amount=2000, mdr_fee=14, platform_fee=20, merchant_net=1966 WHERE id=$1`, "a0000000-0000-0000-0000-000000000001")
		mustFail(t, db, "ubah merchant_id", `UPDATE payments SET merchant_id=$1 WHERE id=$2`, mB, "a0000000-0000-0000-0000-000000000001")
	})
	t.Run("status terminal tidak bisa dibalik, payment tidak bisa dihapus", func(t *testing.T) {
		mustExec(t, db, `UPDATE payments SET status='succeeded' WHERE id=$1`, "a0000000-0000-0000-0000-000000000001")
		mustFail(t, db, "succeeded → failed", `UPDATE payments SET status='failed' WHERE id=$1`, "a0000000-0000-0000-0000-000000000001")
		mustFail(t, db, "delete", `DELETE FROM payments WHERE id=$1`, "a0000000-0000-0000-0000-000000000001")
	})
	t.Run("idempotency key unik per merchant+operasi", func(t *testing.T) {
		q := `INSERT INTO idempotency_records (merchant_id, operation, key, request_hash, expires_at)
		      VALUES ($1,'create_payment','k1','\x01', now() + interval '1 day')`
		mustExec(t, db, q, mA)
		mustFail(t, db, "key sama, merchant sama", q, mA)
		mustExec(t, db, q, mB) // merchant lain boleh memakai key yang sama
	})
	t.Run("audit_logs append-only", func(t *testing.T) {
		mustExec(t, db, `INSERT INTO audit_logs (action, entity_type) VALUES ('x','y')`)
		mustFail(t, db, "update audit", `UPDATE audit_logs SET action='z'`)
		mustFail(t, db, "delete audit", `DELETE FROM audit_logs`)
	})
}
