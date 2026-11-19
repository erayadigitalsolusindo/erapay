package auth

import (
	"context"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// The DEWA panel counts and searches real tenants only; sandbox twins stay out of stats and the impersonation picker.
func TestPlatformPanelIgnoresSandboxMerchants(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	ctx := context.Background()
	live, _ := testdb.SeedMerchant(t, db, "Toko Sandbox Test")
	if _, err := db.Exec(ctx, `INSERT INTO merchants (legal_name, display_name, onboarding_status, is_sandbox, parent_id)
		VALUES ('Toko Sandbox Test (Sandbox)','Toko Sandbox Test (Sandbox)','approved',true,$1)`, live); err != nil {
		t.Fatal(err)
	}
	st, err := s.PlatformStats(ctx)
	if err != nil || st.Merchants["approved"] != 1 {
		t.Fatalf("stats: %+v %v", st.Merchants, err)
	}
	found, err := s.SearchMerchants(ctx, "Sandbox Test", 50)
	if err != nil || len(found) != 1 || found[0].ID != live {
		t.Fatalf("picker: %+v %v", found, err)
	}
}
