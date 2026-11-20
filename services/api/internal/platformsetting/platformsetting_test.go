package platformsetting

import (
	"context"
	"errors"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func TestFeeDefaultsToZeroAndChangeIsValidatedAndAudited(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	s := NewService(db)
	if v, err := s.Int(ctx, QRISGateFee); err != nil || v != 0 {
		t.Fatalf("default = %d, %v; want 0", v, err)
	}
	for _, c := range []struct {
		v      int64
		reason string
	}{{-1, "alasan"}, {MaxFee + 1, "alasan"}, {100, "ab"}} {
		if err := s.SetFee(ctx, c.v, c.reason, "", ""); !errors.Is(err, ErrInvalid) {
			t.Fatalf("SetFee(%d,%q) = %v; want ErrInvalid", c.v, c.reason, err)
		}
	}
	if err := s.SetFee(ctx, 100, "biaya awal", "", "req-1"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Int(ctx, QRISGateFee); v != 100 {
		t.Fatalf("fee = %d; want 100", v)
	}
	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'platform_setting.update' AND metadata->>'old' = '0' AND metadata->>'new' = '100'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit rows = %d, %v; want 1", n, err)
	}
}
