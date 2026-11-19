package apikey

import (
	"context"
	"strings"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// On a production server a key's environment follows its merchant, and the key text, the key row and the
// merchant must agree on every request.
func TestKeyEnvironmentFollowsMerchant(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	live, _ := testdb.SeedMerchant(t, db, "A")
	var twin, uid string
	if err := db.QueryRow(ctx, `INSERT INTO merchants (legal_name, display_name, onboarding_status, is_sandbox, parent_id)
		VALUES ('A sbx','A sbx','approved',true,$1) RETURNING id::text`, live).Scan(&twin); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ('o@a.test','x') RETURNING id::text`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	s := NewService(db, "production")
	mkKey := func(mid string) Issued {
		k, err := s.Create(ctx, CreateInput{MerchantID: mid, CreatedBy: uid, Name: "k", Scopes: []string{"payment:read"}})
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	prd, sbx := mkKey(live), mkKey(twin)
	if !strings.HasPrefix(prd.Secret, "epk_prd_") || !strings.HasPrefix(sbx.Secret, "epk_sbx_") {
		t.Fatalf("formats: %s %s", prd.Secret, sbx.Secret)
	}
	if id, err := s.Verify(ctx, sbx.Secret); err != nil || id.MerchantID != twin || id.Environment != "sandbox" {
		t.Fatalf("sandbox key: %+v %v", id, err)
	}
	if id, err := s.Verify(ctx, prd.Secret); err != nil || id.MerchantID != live || id.Environment != "production" {
		t.Fatalf("production key: %+v %v", id, err)
	}

	// Rotation stays in the merchant's world.
	rot, err := s.Rotate(ctx, twin, sbx.ID, uid, 0, "r")
	if err != nil || !strings.HasPrefix(rot.Secret, "epk_sbx_") {
		t.Fatalf("rotate: %+v %v", rot, err)
	}

	// A row that disagrees with its merchant (or its text) is refused, not trusted.
	if _, err := db.Exec(ctx, `UPDATE api_keys SET environment = 'production' WHERE id = $1`, rot.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(ctx, rot.Secret); err != ErrInvalidKey {
		t.Fatalf("sandbox merchant key marked production: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE api_keys SET environment = 'sandbox' WHERE id = $1`, prd.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(ctx, prd.Secret); err != ErrInvalidKey {
		t.Fatalf("live merchant key marked sandbox: %v", err)
	}

	// A non-production server accepts only sandbox keys, even for a live merchant's production key.
	if _, err := db.Exec(ctx, `UPDATE api_keys SET environment = 'production' WHERE id = $1`, prd.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(db, "sandbox").Verify(ctx, prd.Secret); err != ErrInvalidKey {
		t.Fatalf("production key on a sandbox server: %v", err)
	}
}
