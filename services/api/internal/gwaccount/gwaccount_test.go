package gwaccount

import (
	"context"
	"errors"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// fakeGW is a gateway adapter whose connection test is scripted. Only Code is real; the
// payment methods are never called by gwaccount.
type fakeGW struct {
	gateway.Adapter
	steps   []gateway.ConnectionStep
	gotRef  string
	gotDeep bool
}

func (f *fakeGW) Code() string { return "doku" }
func (f *fakeGW) TestConnection(_ context.Context, ref string, deep bool) []gateway.ConnectionStep {
	f.gotRef, f.gotDeep = ref, deep
	return f.steps
}

type reg struct{ a gateway.Adapter }

func (r reg) Get(code string) (gateway.Adapter, bool) { return r.a, code == "doku" }
func (r reg) Codes() []string                         { return []string{"doku"} }

// plainGW has no ConnectionTester.
type plainGW struct{ gateway.Adapter }

func (plainGW) Code() string { return "doku" }

func newSvc(t *testing.T, a gateway.Adapter) (*Service, string) {
	t.Helper()
	db := testdb.New(t)
	mid, _ := testdb.SeedMerchant(t, db, "Toko Uji")
	return NewService(db, reg{a}, "production"), mid
}

func TestUpsertActivatesAndAudits(t *testing.T) {
	s, mid := newSvc(t, &fakeGW{})
	ctx := context.Background()
	a, err := s.Upsert(ctx, UpsertInput{MerchantID: mid, Gateway: "doku", Environment: "production", GatewayRef: "111214", Status: "active", Note: "dikonfirmasi di dashboard DOKU"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ActivationStatus != "active" || a.GatewayRef != "111214" {
		t.Fatalf("%+v", a)
	}
	// Update in place: same row, new ref, suspended.
	b, err := s.Upsert(ctx, UpsertInput{MerchantID: mid, Gateway: "doku", Environment: "production", GatewayRef: "222", Status: "suspended", Note: "ditangguhkan"})
	if err != nil {
		t.Fatal(err)
	}
	if b.ID != a.ID || b.ActivationStatus != "suspended" || b.GatewayRef != "222" {
		t.Fatalf("%+v vs %+v", b, a)
	}
	ov, err := s.List(ctx, mid)
	if err != nil || len(ov.Accounts) != 1 || ov.ServerEnvironment != "production" || len(ov.Gateways) != 1 {
		t.Fatalf("%+v %v", ov, err)
	}
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='gateway_account.set' AND entity_id = $1`, mid).Scan(&n); err != nil || n != 2 {
		t.Fatalf("audit rows = %d, %v", n, err)
	}
}

func TestUpsertValidation(t *testing.T) {
	s, mid := newSvc(t, &fakeGW{})
	ctx := context.Background()
	ok := UpsertInput{MerchantID: mid, Gateway: "doku", Environment: "sandbox", GatewayRef: "111214", Status: "active", Note: "alasan"}
	cases := map[string]func(*UpsertInput){
		"short note":      func(i *UpsertInput) { i.Note = "x" },
		"unknown gateway": func(i *UpsertInput) { i.Gateway = "nope" },
		"bad env":         func(i *UpsertInput) { i.Environment = "staging" },
		"bad status":      func(i *UpsertInput) { i.Status = "enabled" },
		"bad ref chars":   func(i *UpsertInput) { i.GatewayRef = "1112 14; drop" },
		"active no ref":   func(i *UpsertInput) { i.GatewayRef = "" },
	}
	for name, mut := range cases {
		in := ok
		mut(&in)
		var ve *ValidationError
		if _, err := s.Upsert(ctx, in); !errors.As(err, &ve) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	missing := ok
	missing.MerchantID = "00000000-0000-0000-0000-000000000000"
	if _, err := s.Upsert(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown merchant: %v", err)
	}
}

func TestTestRunsAdapterAndAudits(t *testing.T) {
	f := &fakeGW{steps: []gateway.ConnectionStep{{Name: "credentials", OK: true}, {Name: "merchant", Code: "4004700", Message: "Invalid Merchant"}}}
	s, mid := newSvc(t, f)
	ctx := context.Background()
	if _, err := s.Upsert(ctx, UpsertInput{MerchantID: mid, Gateway: "doku", Environment: "production", GatewayRef: "BAD", Status: "pending_confirmation", Note: "uji"}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Test(ctx, TestInput{MerchantID: mid, Gateway: "doku", Environment: "production", Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || !res.Deep || len(res.Steps) != 2 {
		t.Fatalf("%+v", res)
	}
	if f.gotRef != "BAD" || !f.gotDeep {
		t.Fatalf("adapter got ref=%q deep=%v", f.gotRef, f.gotDeep)
	}
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='gateway_account.tested' AND entity_id = $1`, mid).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit rows = %d, %v", n, err)
	}
}

func TestTestGuards(t *testing.T) {
	s, mid := newSvc(t, &fakeGW{steps: []gateway.ConnectionStep{{Name: "credentials", OK: true}}})
	ctx := context.Background()
	if _, err := s.Test(ctx, TestInput{MerchantID: mid, Gateway: "doku", Environment: "sandbox"}); !errors.Is(err, ErrWrongEnv) {
		t.Fatalf("env mismatch: %v", err)
	}
	// Deep test without an account row is refused; shallow still works (credentials only).
	if _, err := s.Test(ctx, TestInput{MerchantID: mid, Gateway: "doku", Environment: "production", Deep: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deep without account: %v", err)
	}
	res, err := s.Test(ctx, TestInput{MerchantID: mid, Gateway: "doku", Environment: "production"})
	if err != nil || !res.OK {
		t.Fatalf("shallow: %+v %v", res, err)
	}
	s2, mid2 := newSvc(t, plainGW{})
	if _, err := s2.Test(ctx, TestInput{MerchantID: mid2, Gateway: "doku", Environment: "production"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported: %v", err)
	}
}
