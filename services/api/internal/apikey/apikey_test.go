package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func setup(t *testing.T) (*Service, string, string) {
	t.Helper()
	db := testdb.New(t)
	mid, _ := testdb.SeedMerchant(t, db, "A")
	var uid string
	if err := db.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash) VALUES ('o@a.test','x') RETURNING id::text`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	return NewService(db, "sandbox"), mid, uid
}

func mk(t *testing.T, s *Service, mid, uid string) Issued {
	t.Helper()
	k, err := s.Create(context.Background(), CreateInput{MerchantID: mid, CreatedBy: uid, Name: "POS 1",
		Scopes: []string{"payment:create", "payment:read"}})
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestCreateVerifyStoresOnlyHash(t *testing.T) {
	s, mid, uid := setup(t)
	ctx := context.Background()
	k := mk(t, s, mid, uid)
	if !strings.HasPrefix(k.Secret, "epk_sbx_"+k.Prefix+"_") {
		t.Fatalf("bad format: %s", k.Secret)
	}
	secret := k.Secret[strings.LastIndex(k.Secret, "_")+1:]

	var n int
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM api_keys WHERE secret_hash = $1 OR position($2 in prefix||name) > 0`,
		[]byte(secret), secret).Scan(&n)
	if n != 0 {
		t.Fatal("secret stored in plaintext")
	}
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE position($1 in metadata::text) > 0`, secret).Scan(&n)
	if n != 0 {
		t.Fatal("secret leaked into audit_logs")
	}

	id, err := s.Verify(ctx, k.Secret)
	if err != nil || id.MerchantID != mid || id.KeyID != k.ID || len(id.Scopes) != 2 {
		t.Fatalf("verify: %+v err=%v", id, err)
	}

	// Wrong secret, wrong prefix, malformed, wrong environment: all invalid.
	bad := []string{
		"epk_sbx_" + k.Prefix + "_" + strings.Repeat("0", 64),
		"epk_sbx_deadbeef_" + secret,
		"epk_prd_" + k.Prefix + "_" + secret,
		"epk_sbx_" + k.Prefix, "", "Bearer x", k.Secret + "x",
	}
	for _, b := range bad {
		if _, err := s.Verify(ctx, b); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("accepted %q: %v", b, err)
		}
	}
	// A production deployment never accepts sandbox keys.
	prod := NewService(s.db, "production")
	if _, err := prod.Verify(ctx, k.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("prod accepted sandbox key: %v", err)
	}
}

func TestRevokeAndExpiry(t *testing.T) {
	s, mid, uid := setup(t)
	ctx := context.Background()
	k := mk(t, s, mid, uid)
	if err := s.Revoke(ctx, mid, k.ID, uid, "test", "req"); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, mid, k.ID, uid, "again", "req"); err != nil { // idempotent
		t.Fatal(err)
	}
	if _, err := s.Verify(ctx, k.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("revoked key accepted: %v", err)
	}

	exp := time.Now().Add(time.Hour)
	k2, err := s.Create(ctx, CreateInput{MerchantID: mid, CreatedBy: uid, Name: "tmp", Scopes: []string{"payment:read"}, ExpiresAt: &exp})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(ctx, k2.Secret); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := s.Verify(ctx, k2.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("expired key accepted: %v", err)
	}
}

func TestRotate(t *testing.T) {
	s, mid, uid := setup(t)
	ctx := context.Background()

	// Immediate rotation: old dead, new alive, same scopes.
	old := mk(t, s, mid, uid)
	nk, err := s.Rotate(ctx, mid, old.ID, uid, 0, "req")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(ctx, old.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("old key still valid: %v", err)
	}
	id, err := s.Verify(ctx, nk.Secret)
	if err != nil || len(id.Scopes) != 2 || nk.Name != "POS 1" {
		t.Fatalf("new key: %+v err=%v", id, err)
	}
	if _, err := s.Rotate(ctx, mid, old.ID, uid, 0, "req"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rotating a revoked key: %v", err)
	}

	// Grace rotation: both valid until grace ends.
	old2 := mk(t, s, mid, uid)
	nk2, err := s.Rotate(ctx, mid, old2.ID, uid, time.Hour, "req")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []Issued{old2, nk2} {
		if _, err := s.Verify(ctx, k.Secret); err != nil {
			t.Fatalf("key invalid during grace: %v", err)
		}
	}
	s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := s.Verify(ctx, old2.Secret); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("old key valid after grace: %v", err)
	}
	if _, err := s.Rotate(ctx, mid, nk2.ID, uid, 25*time.Hour, "req"); err == nil {
		t.Fatal("grace > 24h accepted")
	}
}

func TestTenantScopedManagement(t *testing.T) {
	s, midA, uid := setup(t)
	ctx := context.Background()
	midB, _ := testdb.SeedMerchant(t, s.db, "B")
	kA := mk(t, s, midA, uid)

	if err := s.Revoke(ctx, midB, kA.ID, uid, "x", "r"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("B revoked A's key: %v", err)
	}
	if _, err := s.Rotate(ctx, midB, kA.ID, uid, 0, "r"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("B rotated A's key: %v", err)
	}
	if l, _ := s.List(ctx, midB); len(l) != 0 {
		t.Fatalf("B lists A's keys: %+v", l)
	}
	if _, err := s.Verify(ctx, kA.Secret); err != nil {
		t.Fatalf("A's key harmed: %v", err)
	}
}

func TestValidateScopes(t *testing.T) {
	has := func(s string) bool { return s == "payment:read" || s == "api_key:manage" }
	if err := ValidateScopes([]string{"payment:read"}, has); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{nil, {}, {"payment:create"}, {"api_key:manage"}, {"user:manage"}, {"payment:read", "gateway:manage"}} {
		if err := ValidateScopes(bad, has); !errors.Is(err, ErrBadScope) {
			t.Fatalf("accepted %v", bad)
		}
	}
}
