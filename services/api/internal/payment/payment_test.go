package payment

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/ledger"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func TestStateMachineTable(t *testing.T) {
	all := []Status{Created, Pending, Succeeded, Failed, Expired, Cancelled, Unknown}
	want := map[[2]Status]bool{
		{Created, Pending}: true, {Created, Failed}: true, {Created, Cancelled}: true, {Created, Unknown}: true,
		{Pending, Succeeded}: true, {Pending, Failed}: true, {Pending, Expired}: true, {Pending, Cancelled}: true, {Pending, Unknown}: true,
		{Unknown, Pending}: true, {Unknown, Succeeded}: true, {Unknown, Failed}: true, {Unknown, Expired}: true,
	}
	for _, a := range all {
		for _, b := range all {
			if got := CanTransition(a, b); got != want[[2]Status{a, b}] {
				t.Errorf("%s -> %s: got %v", a, b, got)
			}
		}
	}
	for _, f := range []Status{Succeeded, Failed, Expired, Cancelled} {
		if !f.IsFinal() {
			t.Errorf("%s must be final", f)
		}
	}
	if Unknown.IsFinal() || Pending.IsFinal() || Created.IsFinal() {
		t.Error("non-final marked final")
	}
	// Invariant #2: only gateway-evidenced sources may set a final status.
	for _, s := range all {
		for _, src := range []Source{SourceWebhook, SourceInquiry, SourceGatewayResponse} {
			if !SourceMaySet(src, s) {
				t.Errorf("%s should be allowed to set %s", src, s)
			}
		}
		if SourceMaySet(SourceInternal, s) == s.IsFinal() {
			t.Errorf("internal source and %s", s)
		}
		if SourceMaySet(Source("client"), s) {
			t.Errorf("client source accepted for %s", s)
		}
	}
}

type env struct {
	db  *pgxpool.Pool
	svc *Service
	led *ledger.Service
	mid string
	oid string
}

func setup(t *testing.T) env {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	mid, oid := testdb.SeedMerchant(t, db, "A")
	var plan string
	if err := db.QueryRow(ctx, `
		INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('std', 1, 'gw1', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
		VALUES ($1,$2, now() - interval '1 day')`, mid, plan); err != nil {
		t.Fatal(err)
	}
	return env{db, NewService(db), ledger.NewService(db), mid, oid}
}

func (e env) input(ref string, amount int64) CreateInput {
	return CreateInput{MerchantID: e.mid, OutletID: e.oid, MerchantReference: ref, Amount: amount,
		Currency: "IDR", PaymentMethod: "QRIS", Gateway: "gw1"}
}

func (e env) create(t *testing.T, ref string, amount int64) Payment {
	t.Helper()
	tx, _ := e.db.Begin(context.Background())
	defer tx.Rollback(context.Background()) //nolint:errcheck
	p, err := e.svc.CreateTx(context.Background(), tx, e.input(ref, amount))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return p
}

func (e env) move(t *testing.T, id string, to Status, src Source) Result {
	t.Helper()
	r, err := e.svc.Transition(context.Background(), TransitionInput{PaymentID: id, To: to, Source: src})
	if err != nil {
		t.Fatalf("%s -> %s: %v", id, to, err)
	}
	return r
}

func (e env) payable(t *testing.T) int64 {
	t.Helper()
	b, err := e.led.MerchantBalance(context.Background(), e.mid, "IDR")
	if err != nil {
		t.Fatal(err)
	}
	return b.Payable
}

func (e env) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateSnapshotsFeesServerSide(t *testing.T) {
	e := setup(t)
	p := e.create(t, "ORD-1", 1000)
	if p.Status != Created || p.Fee.MDRFee != 7 || p.Fee.PlatformFee != 10 || p.Fee.MerchantNet != 983 || p.Fee.Bearer != "merchant" {
		t.Fatalf("snapshot: %+v", p)
	}
	if p.Fee.MDRFee+p.Fee.PlatformFee+p.Fee.MerchantNet != p.Amount {
		t.Fatal("gross != mdr + platform + net")
	}
	// A later, different plan must not touch the stored snapshot (no retroactive change).
	if _, err := e.db.Exec(context.Background(), `UPDATE payments SET mdr_fee = 0, merchant_net = merchant_net + 7 WHERE id = $1`, p.ID); err == nil {
		t.Fatal("snapshot mutation allowed")
	}
	if _, err := e.db.Exec(context.Background(), `UPDATE payments SET amount = 5 WHERE id = $1`, p.ID); err == nil {
		t.Fatal("amount mutation allowed")
	}
	if _, err := e.db.Exec(context.Background(), `DELETE FROM payments WHERE id = $1`, p.ID); err == nil {
		t.Fatal("hard delete allowed")
	}
}

func TestCreateWithMDRCap(t *testing.T) {
	// The capped plan is a new version effective immediately: the checkout picks it over the
	// uncapped v1 because of effective_from DESC. BI QRIS rule: 0.7% capped at Rp500 for
	// gross Rp0-Rp100.000 (docs/integrations/doku.md §6); above the threshold, uncapped.
	e := setup(t)
	ctx := context.Background()
	var plan string
	if err := e.db.QueryRow(ctx, `
		INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding, mdr_cap, mdr_cap_up_to)
		VALUES ('std', 2, 'gw1', 'QRIS', 70, 'half_up', 100, 'half_up', 500, 100000) RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
		VALUES ($1,$2, now() - interval '1 second')`, e.mid, plan); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		ref      string
		amount   int64
		mdr, net int64
	}{
		{"ORD-CAP-LO", 50_000, 350, 50_000 - 350 - 500},
		{"ORD-CAP-AT", 100_000, 500, 100_000 - 500 - 1000},
		{"ORD-CAP-OVER", 100_001, 700, 100_001 - 700 - 1000},
	} {
		p := e.create(t, c.ref, c.amount)
		if p.Fee.MDRFee != c.mdr || p.Fee.MerchantNet != c.net {
			t.Errorf("%s: got %+v want mdr=%d net=%d", c.ref, p.Fee, c.mdr, c.net)
		}
		if e.count(t, "SELECT plan_version FROM payments WHERE id = $1", p.ID) != 2 {
			t.Errorf("%s: snapshot did not use the capped plan v2", c.ref)
		}
		if p.Fee.MDRFee+p.Fee.PlatformFee+p.Fee.MerchantNet != p.Amount {
			t.Errorf("%s: gross != mdr + platform + net", c.ref)
		}
	}
}

func TestCreateValidation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	bad := map[string]func(*CreateInput){
		"zero amount":     func(c *CreateInput) { c.Amount = 0 },
		"negative":        func(c *CreateInput) { c.Amount = -5 },
		"overflow":        func(c *CreateInput) { c.Amount = 1<<62 + 1 },
		"currency":        func(c *CreateInput) { c.Currency = "USD" },
		"empty reference": func(c *CreateInput) { c.MerchantReference = "" },
		"long reference":  func(c *CreateInput) { c.MerchantReference = string(make([]byte, 65)) },
		"expired":         func(c *CreateInput) { c.ExpiresAt = &past },
		"no gateway":      func(c *CreateInput) { c.Gateway = "" },
	}
	for name, mut := range bad {
		in := e.input("ORD", 1000)
		mut(&in)
		tx, _ := e.db.Begin(ctx)
		_, err := e.svc.CreateTx(ctx, tx, in)
		tx.Rollback(ctx) //nolint:errcheck
		if !errors.Is(err, ErrValidation) {
			t.Errorf("%s: got %v", name, err)
		}
	}

	try := func(mut func(*CreateInput)) error {
		in := e.input("ORD", 1000)
		mut(&in)
		tx, _ := e.db.Begin(ctx)
		defer tx.Rollback(ctx) //nolint:errcheck
		_, err := e.svc.CreateTx(ctx, tx, in)
		return err
	}
	if err := try(func(c *CreateInput) { c.PaymentMethod = "VA" }); !errors.Is(err, ErrNoPricing) {
		t.Errorf("no plan: %v", err)
	}
	if err := try(func(c *CreateInput) { c.Gateway = "gw2" }); !errors.Is(err, ErrNoPricing) {
		t.Errorf("other gateway: %v", err)
	}
	other, otherOutlet := testdb.SeedMerchant(t, e.db, "B")
	_ = other
	if err := try(func(c *CreateInput) { c.OutletID = otherOutlet }); !errors.Is(err, ErrValidation) {
		t.Errorf("foreign outlet: %v", err)
	}
	if _, err := e.db.Exec(ctx, `UPDATE merchants SET onboarding_status = 'suspended' WHERE id = $1`, e.mid); err != nil {
		t.Fatal(err)
	}
	if err := try(func(c *CreateInput) {}); !errors.Is(err, ErrMerchantNotActive) {
		t.Errorf("suspended merchant: %v", err)
	}
}

func TestGetIsTenantScoped(t *testing.T) {
	e := setup(t)
	p := e.create(t, "ORD-1", 1000)
	other, _ := testdb.SeedMerchant(t, e.db, "B")
	if _, err := e.svc.Get(context.Background(), other, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get: %v", err)
	}
	if got, err := e.svc.Get(context.Background(), e.mid, p.ID); err != nil || got.ID != p.ID {
		t.Fatalf("own get: %v", err)
	}
}

// Same Idempotency-Key => one payment, even with 30 parallel retries.
func TestCreateIdempotentConcurrent(t *testing.T) {
	e := setup(t)
	st := idempotency.NewStore(e.db)
	body := `{"merchant_reference":"ORD-9","amount":10000}`
	h, _ := idempotency.HashJSON([]byte(body), "POST", "/v1/payments")
	sc := idempotency.Scope{MerchantID: e.mid, Operation: "payment.create", Key: "idem-key-0001"}

	var wg sync.WaitGroup
	ids := make(chan string, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _, err := st.Do(context.Background(), sc, h, func(ctx context.Context, tx pgx.Tx) (idempotency.Response, error) {
				p, err := e.svc.CreateTx(ctx, tx, e.input("ORD-9", 10000))
				if err != nil {
					return idempotency.Response{}, err
				}
				return idempotency.Response{Status: 201, Body: []byte(`{"id":"` + p.ID + `"}`), ResourceType: "payment", ResourceID: p.ID}, nil
			})
			if err != nil {
				t.Errorf("do: %v", err)
				return
			}
			ids <- r.ResourceID
		}()
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		} else if id != first {
			t.Fatalf("different payments for one key: %s %s", first, id)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM payments`); n != 1 {
		t.Fatalf("payments = %d, want 1", n)
	}
}

func TestLifecycleSuccessPostsLedgerOnce(t *testing.T) {
	e := setup(t)
	p := e.create(t, "ORD-1", 1000)

	if _, err := e.svc.Transition(context.Background(), TransitionInput{PaymentID: p.ID, To: Succeeded, Source: SourceInternal}); !errors.Is(err, ErrSourceNotAllowed) {
		t.Fatalf("internal source set final status: %v", err)
	}
	if _, err := e.svc.Transition(context.Background(), TransitionInput{PaymentID: p.ID, To: Succeeded, Source: SourceWebhook}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("created -> succeeded must be invalid: %v", err)
	}
	if e.payable(t) != 0 {
		t.Fatal("ledger posted for non-success")
	}

	e.move(t, p.ID, Pending, SourceInternal)
	r := e.move(t, p.ID, Succeeded, SourceWebhook)
	if !r.Applied || r.Status != Succeeded {
		t.Fatalf("%+v", r)
	}
	if e.payable(t) != 983 {
		t.Fatalf("payable = %d, want 983", e.payable(t))
	}
	if r := e.move(t, p.ID, Succeeded, SourceWebhook); !r.NoOp {
		t.Fatalf("repeat should be no-op: %+v", r)
	}
	if e.payable(t) != 983 {
		t.Fatal("ledger posted twice")
	}
	got, _ := e.svc.Get(context.Background(), e.mid, p.ID)
	if got.Status != Succeeded || got.FinalAt == nil {
		t.Fatalf("%+v", got)
	}
	if n := e.count(t, `SELECT count(*) FROM payment_events WHERE payment_id=$1 AND applied`, p.ID); n != 2 {
		t.Fatalf("applied events = %d", n)
	}
}

// Timeout is `unknown`, not `failed`; inquiry then resolves it.
func TestTimeoutIsUnknownThenResolved(t *testing.T) {
	e := setup(t)
	p := e.create(t, "ORD-1", 5000)
	if r := e.move(t, p.ID, Unknown, SourceInternal); r.Status != Unknown {
		t.Fatalf("%+v", r)
	}
	got, _ := e.svc.Get(context.Background(), e.mid, p.ID)
	if got.FinalAt != nil || got.Status.IsFinal() {
		t.Fatal("unknown treated as final")
	}
	if r := e.move(t, p.ID, Pending, SourceInquiry); r.Status != Pending {
		t.Fatalf("%+v", r)
	}
	e.move(t, p.ID, Succeeded, SourceInquiry)
	if e.payable(t) == 0 {
		t.Fatal("no ledger after inquiry success")
	}
}

// A late callback must never flip a terminal status or move money; it is kept as evidence.
func TestLateConflictingReportIsRecordedNotApplied(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// expired, then a late "succeeded"
	p := e.create(t, "ORD-1", 1000)
	e.move(t, p.ID, Pending, SourceInternal)
	e.move(t, p.ID, Expired, SourceInquiry)
	r, err := e.svc.Transition(ctx, TransitionInput{PaymentID: p.ID, To: Succeeded, Source: SourceWebhook, RawGatewayStatus: "SUCCESS", DedupKey: "wh-1"})
	if err != nil || !r.Conflict || r.Applied || r.Status != Expired {
		t.Fatalf("%+v err=%v", r, err)
	}
	if e.payable(t) != 0 {
		t.Fatal("late success moved money")
	}
	if n := e.count(t, `SELECT count(*) FROM payment_events WHERE payment_id=$1 AND NOT applied AND to_status='succeeded'`, p.ID); n != 1 {
		t.Fatalf("conflict evidence events = %d", n)
	}
	// replay of the same webhook is deduped
	if r, _ := e.svc.Transition(ctx, TransitionInput{PaymentID: p.ID, To: Succeeded, Source: SourceWebhook, DedupKey: "wh-1"}); !r.Duplicate {
		t.Fatalf("%+v", r)
	}

	// succeeded, then a late "failed"
	q := e.create(t, "ORD-2", 1000)
	e.move(t, q.ID, Pending, SourceInternal)
	e.move(t, q.ID, Succeeded, SourceWebhook)
	if r := e.move(t, q.ID, Failed, SourceWebhook); !r.Conflict || r.Status != Succeeded {
		t.Fatalf("%+v", r)
	}
	if e.payable(t) != 983 {
		t.Fatalf("payable = %d", e.payable(t))
	}
}

func TestGatewayReferenceUniqueAndSetOnce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, b := e.create(t, "A", 1000), e.create(t, "B", 1000)
	if _, err := e.svc.Transition(ctx, TransitionInput{PaymentID: a.ID, To: Pending, Source: SourceGatewayResponse, GatewayReference: "REF-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Transition(ctx, TransitionInput{PaymentID: b.ID, To: Pending, Source: SourceGatewayResponse, GatewayReference: "REF-1"}); err == nil {
		t.Fatal("duplicate gateway reference accepted")
	}
	if _, err := e.svc.Transition(ctx, TransitionInput{PaymentID: a.ID, To: Succeeded, Source: SourceWebhook, GatewayReference: "OTHER"}); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, e.mid, a.ID)
	if got.GatewayReference != "REF-1" {
		t.Fatalf("gateway reference overwritten: %s", got.GatewayReference)
	}
}

// 30 parallel "succeeded" reports (webhook retries + inquiry): one applies, ledger posts once.
func TestConcurrentSuccessReportsPostOnce(t *testing.T) {
	e := setup(t)
	p := e.create(t, "ORD-1", 100000)
	e.move(t, p.ID, Pending, SourceInternal)

	var wg sync.WaitGroup
	var applied, noop atomic.Int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			src := SourceWebhook
			if i%5 == 0 {
				src = SourceInquiry
			}
			r, err := e.svc.Transition(context.Background(), TransitionInput{PaymentID: p.ID, To: Succeeded, Source: src,
				DedupKey: "evt-" + strconv.Itoa(i)})
			if err != nil {
				t.Errorf("transition: %v", err)
				return
			}
			if r.Applied {
				applied.Add(1)
			} else if r.NoOp {
				noop.Add(1)
			}
		}()
	}
	wg.Wait()
	if applied.Load() != 1 || noop.Load() != 29 {
		t.Fatalf("applied=%d noop=%d", applied.Load(), noop.Load())
	}
	want := int64(100000 - 700 - 1000)
	if e.payable(t) != want {
		t.Fatalf("payable = %d, want %d", e.payable(t), want)
	}
	if n := e.count(t, `SELECT count(*) FROM ledger_transactions`); n != 1 {
		t.Fatalf("ledger txns = %d", n)
	}
}

// Race "succeeded" against "expired": exactly one final status; money moves iff success won.
func TestConcurrentSuccessVsExpiryRace(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	for round := 0; round < 12; round++ {
		p := e.create(t, "R-"+strconv.Itoa(round), 1000)
		e.move(t, p.ID, Pending, SourceInternal)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			to := Succeeded
			if i%2 == 1 {
				to = Expired
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := e.svc.Transition(ctx, TransitionInput{PaymentID: p.ID, To: to, Source: SourceInquiry}); err != nil {
					t.Errorf("transition: %v", err)
				}
			}()
		}
		wg.Wait()
		got, _ := e.svc.Get(ctx, e.mid, p.ID)
		if !got.Status.IsFinal() {
			t.Fatalf("round %d: not final: %s", round, got.Status)
		}
		if n := e.count(t, `SELECT count(*) FROM payment_events WHERE payment_id=$1 AND applied AND to_status IN ('succeeded','expired')`, p.ID); n != 1 {
			t.Fatalf("round %d: %d final events applied", round, n)
		}
		posted := e.count(t, `SELECT count(*) FROM ledger_transactions WHERE dedup_key = $1`, "payment_succeeded:"+p.ID)
		if (got.Status == Succeeded) != (posted == 1) {
			t.Fatalf("round %d: status=%s ledger posted=%d", round, got.Status, posted)
		}
	}
}
