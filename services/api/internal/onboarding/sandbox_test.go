package onboarding

import (
	"context"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// The approval/review queues never list sandbox twins.
func TestReviewListSkipsSandboxMerchants(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, nil, Config{})
	ctx := context.Background()
	live, _ := testdb.SeedMerchant(t, db, "Live")
	if _, err := db.Exec(ctx, `INSERT INTO merchants (legal_name, display_name, onboarding_status, is_sandbox, parent_id)
		VALUES ('Live (Sandbox)','Live (Sandbox)','approved',true,$1)`, live); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(ctx, "approved", 50)
	if err != nil || len(got) != 1 || got[0].ID != live {
		t.Fatalf("approved list: %+v %v", got, err)
	}
}
