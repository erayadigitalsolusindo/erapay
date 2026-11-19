package merchant

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func TestEnableSandbox(t *testing.T) {
	db := testdb.New(t)
	r := NewRepo(db)
	ctx := context.Background()
	live, _ := testdb.SeedMerchant(t, db, "A")
	var plan string
	if err := db.QueryRow(ctx, `INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('std', 1, 'gw1', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from) VALUES ($1,$2, now() - interval '1 day')`, live, plan); err != nil {
		t.Fatal(err)
	}

	// 20 parallel enables: exactly one twin, one "created".
	var wg sync.WaitGroup
	var mu sync.Mutex
	ids, createdN := map[string]bool{}, 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, created, err := r.EnableSandbox(ctx, live, "", "req")
			if err != nil {
				t.Errorf("enable: %v", err)
				return
			}
			mu.Lock()
			ids[id] = true
			if created {
				createdN++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(ids) != 1 || createdN != 1 {
		t.Fatalf("twins=%d created=%d, want 1/1", len(ids), createdN)
	}
	var twin string
	for id := range ids {
		twin = id
	}
	if got, _ := r.SandboxOf(ctx, live); got != twin {
		t.Fatalf("SandboxOf = %q", got)
	}

	var sbx bool
	var status string
	var outlets, plans, members, audits int
	_ = db.QueryRow(ctx, `SELECT is_sandbox, onboarding_status FROM merchants WHERE id = $1`, twin).Scan(&sbx, &status)
	_ = db.QueryRow(ctx, `SELECT count(*) FROM merchant_outlets WHERE merchant_id = $1`, twin).Scan(&outlets)
	_ = db.QueryRow(ctx, `SELECT count(*) FROM merchant_pricing_assignments WHERE merchant_id = $1 AND plan_id = $2`, twin, plan).Scan(&plans)
	_ = db.QueryRow(ctx, `SELECT count(*) FROM merchant_users WHERE merchant_id = $1`, twin).Scan(&members)
	_ = db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'merchant.sandbox_enabled'`).Scan(&audits)
	if !sbx || status != "approved" || outlets != 1 || plans != 1 || members != 0 || audits != 1 {
		t.Fatalf("twin: sandbox=%v status=%s outlets=%d plans=%d members=%d audits=%d", sbx, status, outlets, plans, members, audits)
	}

	// A sandbox merchant cannot open a sandbox of its own, and unapproved or unknown merchants cannot either.
	if _, _, err := r.EnableSandbox(ctx, twin, "", "r"); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("twin of twin: %v", err)
	}
	draft := ""
	_ = db.QueryRow(ctx, `INSERT INTO merchants (legal_name, display_name) VALUES ('D','D') RETURNING id::text`).Scan(&draft)
	if _, _, err := r.EnableSandbox(ctx, draft, "", "r"); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("draft merchant: %v", err)
	}
	if _, _, err := r.EnableSandbox(ctx, "00000000-0000-0000-0000-000000000000", "", "r"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown merchant: %v", err)
	}
}

// The fake gateway is opt-in: off, a twin has no gateway account; on, enabling (again) gives it a priced plan
// and an active account exactly once, so a twin made before the switch is brought up to date.
func TestEnableSandboxFakeGateway(t *testing.T) {
	db := testdb.New(t)
	r := NewRepo(db)
	ctx := context.Background()
	live, _ := testdb.SeedMerchant(t, db, "A")
	var plan string
	if err := db.QueryRow(ctx, `INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('doku-a', 1, 'doku', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from) VALUES ($1,$2, now() - interval '1 day')`, live, plan); err != nil {
		t.Fatal(err)
	}
	count := func(q string, args ...any) (n int) {
		_ = db.QueryRow(ctx, q, args...).Scan(&n)
		return n
	}

	twin, _, err := r.EnableSandbox(ctx, live, "", "r")
	if err != nil {
		t.Fatal(err)
	}
	if count(`SELECT count(*) FROM merchant_gateway_accounts WHERE merchant_id = $1`, twin) != 0 {
		t.Fatal("fake gateway provisioned while the flag is off")
	}

	r.SetFakeGateway(true)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := r.EnableSandbox(ctx, live, "", "r"); err != nil {
				t.Errorf("enable again: %v", err)
			}
		}()
	}
	wg.Wait()
	if count(`SELECT count(*) FROM merchant_gateway_accounts WHERE merchant_id = $1 AND gateway = 'fake' AND environment = 'sandbox' AND activation_status = 'active' AND gateway_ref = 'FAKE'`, twin) != 1 {
		t.Fatal("fake account missing or duplicated")
	}
	if count(`SELECT count(*) FROM pricing_plans WHERE code = 'fake-qris-' || $1::text`, twin) != 1 ||
		count(`SELECT count(*) FROM merchant_pricing_assignments a JOIN pricing_plans p ON p.id = a.plan_id WHERE a.merchant_id = $1 AND p.gateway = 'fake' AND p.mdr_bps = 70 AND p.platform_bps = 100`, twin) != 1 {
		t.Fatal("fake plan missing, duplicated, or not priced like the DOKU plan")
	}
}
