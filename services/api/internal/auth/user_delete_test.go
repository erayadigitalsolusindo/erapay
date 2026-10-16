package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func TestDeleteUser(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	mk := func(email string) string {
		t.Helper()
		id, err := s.CreateUser(ctx, email, pw)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	holder, other, actor := mk("holder@erapay.test"), mk("other@erapay.test"), mk("actor@erapay.test")

	var roleID string
	if err := db.QueryRow(ctx, `INSERT INTO roles (code, scope) VALUES ('t_dewa','platform') RETURNING id::text`).Scan(&roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_code) VALUES ($1,'rbac:manage')`, roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2)`, holder, roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2)`, other, roleID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, '\x01', now() + interval '1 hour')`, other); err != nil {
		t.Fatal(err)
	}

	// Tidak boleh menghapus diri sendiri.
	if _, err := s.DeleteUser(ctx, actor, actor, "r"); !errors.Is(err, ErrUserProtected) {
		t.Fatalf("self delete: %v", err)
	}
	// User tak dikenal.
	if _, err := s.DeleteUser(ctx, "00000000-0000-0000-0000-000000000000", actor, "r"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	// Pemegang rbac:manage kedua boleh dihapus; sesi dan peran ikut hilang.
	touched, err := s.DeleteUser(ctx, other, holder, "r")
	if err != nil {
		t.Fatalf("delete other: %v", err)
	}
	if touched["sessions.user_id"] != 1 || touched["user_roles.user_id"] != 1 {
		t.Fatalf("relations: %v", touched)
	}
	var n int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM users WHERE id = $1`, other).Scan(&n)
	if n != 0 {
		t.Fatal("user still exists")
	}
	_ = db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'platform.user.deleted' AND entity_id = $1`, other).Scan(&n)
	if n != 1 {
		t.Fatalf("audit rows: %d", n)
	}
	// Pemegang rbac:manage terakhir dilindungi.
	if _, err := s.DeleteUser(ctx, holder, actor, "r"); !errors.Is(err, ErrUserProtected) {
		t.Fatalf("last holder: %v", err)
	}
	// Jejak keuangan (pricing_plans append-only) menolak penghapusan, dan transaksi tidak meninggalkan efek samping.
	if _, err := db.Exec(ctx, `INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding, created_by)
		VALUES ('t-plan', 1, 'doku', 'QRIS', 70, 'half_up', 100, 'half_up', $1)`, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, '\x02', now() + interval '1 hour')`, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteUser(ctx, actor, holder, "r"); !errors.Is(err, ErrUserHasFinancial) {
		t.Fatalf("financial: %v", err)
	}
	_ = db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id = $1`, actor).Scan(&n)
	if n != 1 {
		t.Fatalf("rollback expected, sessions=%d", n)
	}
}

// Kueri statistik menelan error per-blok, jadi pastikan benar-benar mengembalikan data (bukan nol karena SQL salah).
func TestPlatformStatsAndListUsers(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	if _, err := s.CreateUser(ctx, "stat@erapay.test", pw); err != nil {
		t.Fatal(err)
	}
	st, err := s.PlatformStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Users.Total != 1 || st.Users.Active != 1 || st.RBAC.Roles == 0 || st.RBAC.Permissions == 0 {
		t.Fatalf("stats: %+v", st)
	}
	if len(st.Audit.Series) != 24 {
		t.Fatalf("series len %d", len(st.Audit.Series))
	}
	us, err := s.ListUsers(ctx, "")
	if err != nil || len(us) != 1 || us[0].CreatedAt == "" {
		t.Fatalf("list users: %v %+v", err, us)
	}
}
