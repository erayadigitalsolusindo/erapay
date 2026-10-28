package routing

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

type fakeAdapter struct {
	code     string
	platform bool
	methods  []string
}

func (f fakeAdapter) Code() string { return f.code }
func (f fakeAdapter) Capabilities() gateway.Capabilities {
	return gateway.Capabilities{PaymentMethods: f.methods, PlatformAccount: f.platform}
}
func (fakeAdapter) CreatePayment(context.Context, gateway.CreateRequest) (gateway.CreateResult, error) {
	return gateway.CreateResult{}, nil
}
func (fakeAdapter) GetPayment(context.Context, gateway.QueryRequest) (gateway.QueryResult, error) {
	return gateway.QueryResult{}, nil
}
func (fakeAdapter) CancelPayment(context.Context, gateway.CancelRequest) error { return nil }
func (fakeAdapter) MapStatus(string) (gateway.Status, bool)                    { return "", false }

func setup(t *testing.T, adapters ...fakeAdapter) (*Service, *pgxpool.Pool) {
	t.Helper()
	db := testdb.New(t)
	reg := gateway.NewRegistry()
	for _, a := range adapters {
		reg.Register(a)
	}
	return NewService(db, reg, "sandbox"), db
}

var both = []fakeAdapter{
	{code: "doku", methods: []string{"QRIS"}},
	{code: "ids", platform: true, methods: []string{"QRIS"}},
}

func set(s *Service, gw string, version int) (Overview, error) {
	return s.Set(context.Background(), SetInput{Environment: "sandbox", Gateway: gw, ExpectVersion: version, Note: "uji jalur"})
}

func TestNoChoiceByDefaultAndChoiceIsStored(t *testing.T) {
	s, db := setup(t, both...)
	ctx := context.Background()
	if gw, err := s.Preferred(ctx); err != nil || gw != "" {
		t.Fatalf("default: %q %v", gw, err)
	}
	ov, err := s.Overview(ctx)
	if err != nil || ov.Gateway != "" || ov.Version != 0 || len(ov.Options) != 2 || !ov.Options[0].Available || !ov.Options[1].PlatformAccount {
		t.Fatalf("overview: %+v %v", ov, err)
	}

	ov, err = set(s, "ids", 0)
	if err != nil || ov.Gateway != "ids" || ov.Version != 1 || ov.UpdatedAt == nil {
		t.Fatalf("set ids: %+v %v", ov, err)
	}
	if gw, _ := s.Preferred(ctx); gw != "ids" {
		t.Fatalf("preferred = %q", gw)
	}
	ov, err = set(s, "doku", 1)
	if err != nil || ov.Gateway != "doku" || ov.Version != 2 {
		t.Fatalf("switch back: %+v %v", ov, err)
	}

	var n int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'payment_routing.set'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("audit rows = %d (%v)", n, err)
	}
}

func TestSetRefusesWhatCannotWork(t *testing.T) {
	// Only DOKU registered: choosing IDS would make every payment fall back, so it is refused.
	s, _ := setup(t, both[0])
	var ve *ValidationError
	if _, err := set(s, "ids", 0); !errors.As(err, &ve) {
		t.Fatalf("unregistered gateway accepted: %v", err)
	}
	if _, err := set(s, "midtrans", 0); !errors.As(err, &ve) {
		t.Fatalf("unknown gateway accepted: %v", err)
	}
	if _, err := s.Set(context.Background(), SetInput{Environment: "sandbox", Gateway: "doku", Note: "x"}); !errors.As(err, &ve) {
		t.Fatalf("short note accepted: %v", err)
	}
	if _, err := s.Set(context.Background(), SetInput{Environment: "production", Gateway: "doku", Note: "uji jalur"}); !errors.Is(err, ErrWrongEnv) {
		t.Fatalf("other environment: %v", err)
	}
	// A gateway that cannot carry QRIS is not a choice either.
	s2, _ := setup(t, fakeAdapter{code: "doku", methods: []string{"VA"}})
	if _, err := set(s2, "doku", 0); !errors.As(err, &ve) {
		t.Fatalf("gateway without QRIS accepted: %v", err)
	}
	if gw, _ := s.Preferred(context.Background()); gw != "" {
		t.Fatalf("a refused change was stored: %q", gw)
	}
}

func TestStaleFormAndNoOpAreRefused(t *testing.T) {
	s, _ := setup(t, both...)
	if _, err := set(s, "ids", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := set(s, "doku", 0); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version: %v", err)
	}
	var ve *ValidationError
	if _, err := set(s, "ids", 1); !errors.As(err, &ve) {
		t.Fatalf("same gateway again: %v", err)
	}
	if ov, _ := s.Overview(context.Background()); ov.Gateway != "ids" || ov.Version != 1 {
		t.Fatalf("state changed by refused calls: %+v", ov)
	}
}

// Two admins pressing Save from the same form: exactly one change lands, the other is told to reload.
func TestConcurrentSavesFromTheSameFormApplyOnce(t *testing.T) {
	s, db := setup(t, both...)
	if _, err := set(s, "doku", 0); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var won, conflicts, other atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := set(s, "ids", 1)
			switch {
			case err == nil:
				won.Add(1)
			case errors.Is(err, ErrConflict):
				conflicts.Add(1)
			default:
				other.Add(1)
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 || conflicts.Load() != 9 {
		t.Fatalf("won=%d conflicts=%d other=%d", won.Load(), conflicts.Load(), other.Load())
	}
	var n int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE action = 'payment_routing.set'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("audit rows = %d (%v)", n, err)
	}
	if ov, _ := s.Overview(context.Background()); ov.Gateway != "ids" || ov.Version != 2 {
		t.Fatalf("final state %+v", ov)
	}
}
