package payment

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// at sets created_at (and final_at for final payments) so tests control which day a payment falls on.
func (e env) at(t *testing.T, id string, created, final time.Time) {
	t.Helper()
	if _, err := e.db.Exec(context.Background(), `UPDATE payments SET created_at = $2::timestamptz,
		final_at = CASE WHEN final_at IS NULL THEN NULL ELSE $3::timestamptz END WHERE id = $1`, id, created, final); err != nil {
		t.Fatal(err)
	}
}

func ids(ps []Payment) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func TestListPaginatesNewestFirstWithoutGapsOrRepeats(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	var all []string
	for i := 0; i < 11; i++ {
		p := e.create(t, "ORD-"+strconv.Itoa(i), 10_000)
		// Two payments share a timestamp: the id tiebreak must keep the order total.
		e.at(t, p.ID, base.Add(time.Duration(i/2)*time.Minute), base)
		all = append(all, p.ID)
	}

	var got []string
	cursor := ""
	pages := 0
	for {
		items, next, err := e.svc.List(ctx, ListFilter{MerchantID: e.mid, Limit: 4, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ids(items)...)
		pages++
		if next == "" {
			break
		}
		cursor = next
		// A payment arriving mid-way must not shift the remaining pages.
		if pages == 1 {
			e.create(t, "LATE", 10_000)
		}
	}
	if pages != 3 || len(got) != 11 {
		t.Fatalf("pages=%d items=%d", pages, len(got))
	}
	seen := map[string]bool{}
	for _, id := range got {
		if seen[id] {
			t.Fatalf("payment %s on two pages", id)
		}
		seen[id] = true
	}
	for _, id := range all {
		if !seen[id] {
			t.Fatalf("payment %s missing from the pages", id)
		}
	}
	// Newest first.
	first, _, _ := e.svc.List(ctx, ListFilter{MerchantID: e.mid, Limit: 100})
	for i := 1; i < len(first); i++ {
		if first[i].CreatedAt.After(first[i-1].CreatedAt) {
			t.Fatal("not sorted newest first")
		}
	}
}

func TestListFiltersAndValidation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a := e.create(t, "A-1", 10_000)
	b := e.create(t, "B-1", 20_000)
	e.move(t, a.ID, Pending, SourceInternal)
	e.move(t, a.ID, Succeeded, SourceWebhook)
	e.move(t, b.ID, Pending, SourceInternal)
	day := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	e.at(t, a.ID, day, day)

	list := func(f ListFilter) []Payment {
		t.Helper()
		f.MerchantID = e.mid
		items, _, err := e.svc.List(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		return items
	}
	if got := list(ListFilter{Status: Succeeded}); len(got) != 1 || got[0].ID != a.ID || got[0].SettlementStatus != "expected" {
		t.Fatalf("status filter: %+v", got)
	}
	if got := list(ListFilter{Reference: "B-1"}); len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("reference filter: %+v", got)
	}
	if got := list(ListFilter{Reference: "b-1"}); len(got) != 0 {
		t.Fatal("reference match must be exact")
	}
	from, to := day.Add(-time.Hour), day.Add(time.Hour)
	if got := list(ListFilter{From: &from, To: &to}); len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("date range: %+v", got)
	}
	// `to` is exclusive: a payment created exactly at `to` is not included.
	if got := list(ListFilter{From: &from, To: &day}); len(got) != 0 {
		t.Fatal("to bound must be exclusive")
	}
	// Pending payments never leak their QR payload in a list.
	e.db.Exec(ctx, `UPDATE payments SET qr_payload = 'QR-SECRET' WHERE id = $1`, b.ID) //nolint:errcheck
	if got := list(ListFilter{Status: Pending}); len(got) != 1 || got[0].QRCode != nil {
		t.Fatalf("QR leaked in list: %+v", got)
	}

	for _, bad := range []ListFilter{{MerchantID: e.mid, Status: "bogus"}, {MerchantID: e.mid, Cursor: "!!!"}, {MerchantID: e.mid, Cursor: "bm9wZQ"}} {
		if _, _, err := e.svc.List(ctx, bad); !errors.Is(err, ErrValidation) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestListIsTenantScoped(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	mine := e.create(t, "MINE", 10_000)
	other, _ := testdb.SeedMerchant(t, e.db, "B")
	items, _, err := e.svc.List(ctx, ListFilter{MerchantID: other})
	if err != nil || len(items) != 0 {
		t.Fatalf("tenant B sees %d payments: %v", len(items), err)
	}
	// A cursor taken from tenant A cannot be used to read tenant B's rows either.
	_, next, _ := e.svc.List(ctx, ListFilter{MerchantID: e.mid, Limit: 1})
	_ = next
	if items, _, _ := e.svc.List(ctx, ListFilter{MerchantID: e.mid}); len(items) != 1 || items[0].ID != mine.ID {
		t.Fatal("tenant A lost its own payment")
	}
}

func TestSummarizeTotalsDaysAndRefunds(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	wib, _ := time.LoadLocation(ReportTimezone)

	mk := func(ref string, amount int64, final Status, created, finalAt time.Time) string {
		p := e.create(t, ref, amount)
		e.move(t, p.ID, Pending, SourceInternal)
		if final != Pending {
			e.move(t, p.ID, final, SourceWebhook)
		}
		e.at(t, p.ID, created, finalAt)
		return p.ID
	}
	// Day boundaries in WIB: 23:30 on the 11th and 00:30 on the 12th are different days, although only
	// one hour apart (and the 12th 00:30 WIB is still the 11th in UTC).
	d11 := time.Date(2026, 10, 11, 23, 30, 0, 0, wib)
	d12 := time.Date(2026, 10, 12, 0, 30, 0, 0, wib)
	d10 := time.Date(2026, 10, 10, 12, 0, 0, 0, wib)
	p1 := mk("S1", 100_000, Succeeded, d11, d11)
	mk("S2", 50_000, Succeeded, d12, d12)
	mk("F1", 70_000, Failed, d10, d10)
	mk("P1", 30_000, Pending, d12, d12)
	mk("OLD", 999_000, Succeeded, time.Date(2026, 8, 1, 12, 0, 0, 0, wib), time.Date(2026, 8, 1, 12, 0, 0, 0, wib)) // outside the period

	var maker, checker string
	for i, dst := range []*string{&maker, &checker} {
		if err := e.db.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id::text`, "u"+strconv.Itoa(i)+"@x.test").Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.db.Exec(ctx, `INSERT INTO refunds (merchant_id, payment_id, amount, currency, reason, status, requested_by, approved_by, approved_at, bank_reference, refunded_at)
		VALUES ($1, $2, 12000, 'IDR', 'test refund', 'refunded', $3, $4, now(), 'REF-1', $5)`, e.mid, p1, maker, checker, d12); err != nil {
		t.Fatal(err)
	}

	s, err := e.svc.Summarize(ctx, e.mid, "IDR", time.Date(2026, 10, 10, 0, 0, 0, 0, wib), time.Date(2026, 10, 12, 0, 0, 0, 0, wib))
	if err != nil {
		t.Fatal(err)
	}
	tt := s.Totals
	if tt.Payments != 4 || tt.Succeeded != 2 || tt.Failed != 1 || tt.Pending != 1 {
		t.Fatalf("counts: %+v", tt)
	}
	if tt.Gross != 150_000 || tt.MDRFee != 700+350 || tt.PlatformFee != 1000+500 || tt.MerchantNet != 150_000-1050-1500 {
		t.Fatalf("money: %+v", tt)
	}
	if tt.Gross != tt.MDRFee+tt.PlatformFee+tt.MerchantNet {
		t.Fatal("gross != mdr + platform + net")
	}
	if tt.Refunded != 12_000 {
		t.Fatalf("refunded = %d", tt.Refunded)
	}
	if s.From != "2026-10-10" || s.To != "2026-10-12" || s.Timezone != ReportTimezone || len(s.Daily) != 3 {
		t.Fatalf("shape: %+v", s)
	}
	want := map[string][2]int64{"2026-10-10": {0, 0}, "2026-10-11": {1, 100_000}, "2026-10-12": {1, 50_000}}
	for _, d := range s.Daily {
		if w := want[d.Date]; d.Count != w[0] || d.Gross != w[1] {
			t.Errorf("day %s: %+v want %v", d.Date, d, w)
		}
	}

	// Another tenant sees nothing; bad periods are refused.
	other, _ := testdb.SeedMerchant(t, e.db, "B")
	if so, err := e.svc.Summarize(ctx, other, "IDR", d10, d12); err != nil || so.Totals.Payments != 0 || so.Totals.Gross != 0 {
		t.Fatalf("tenant B summary: %+v %v", so, err)
	}
	if _, err := e.svc.Summarize(ctx, e.mid, "IDR", d12, d10); !errors.Is(err, ErrValidation) {
		t.Fatalf("reversed period: %v", err)
	}
	if _, err := e.svc.Summarize(ctx, e.mid, "IDR", d10.AddDate(-2, 0, 0), d10); !errors.Is(err, ErrValidation) {
		t.Fatalf("period over a year: %v", err)
	}
}
