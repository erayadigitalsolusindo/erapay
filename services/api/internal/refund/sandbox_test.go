package refund

import (
	"context"
	"testing"
)

// The platform refund queue (Finance) never lists a sandbox merchant's refunds; the merchant's own list does.
// A sandbox payment is refundable without settlement, since sandbox funds are available at once.
func TestPlatformRefundQueueSkipsSandbox(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.db.Exec(ctx, `UPDATE merchants SET is_sandbox = true, parent_id = $1 WHERE id = $2`, e.a, e.b); err != nil {
		t.Fatal(err)
	}
	live := e.paid(t, e.a, 100_000, true)
	sbx := e.paid(t, e.b, 100_000, false)
	if _, _, err := e.req(e.a, e.ua, live, 10_000, "live-refund-01"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.req(e.b, e.ub, sbx, 10_000, "sbx-refund-001"); err != nil {
		t.Fatalf("sandbox refund: %v", err)
	}
	all, err := e.svc.List(ctx, "", "", "", 50)
	if err != nil || len(all) != 1 || all[0].MerchantID != e.a {
		t.Fatalf("platform queue: %+v %v", all, err)
	}
	if own, _ := e.svc.List(ctx, e.b, "", "", 50); len(own) != 1 {
		t.Fatalf("sandbox merchant's own refunds: %+v", own)
	}
}
