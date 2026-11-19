package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

func TestFakeGateway(t *testing.T) {
	a := New()
	var _ gateway.Adapter = a
	ctx := context.Background()
	if !a.Capabilities().SupportsMethod("QRIS") || a.Capabilities().PlatformAccount {
		t.Fatalf("capabilities: %+v", a.Capabilities())
	}
	res, err := a.CreatePayment(ctx, gateway.CreateRequest{Reference: "p1", Amount: 1000, Currency: "IDR"})
	if err != nil || res.GatewayReference != "FAKE-p1" || res.QRPayload == "" {
		t.Fatalf("create: %+v %v", res, err)
	}
	if _, err := a.CreatePayment(ctx, gateway.CreateRequest{Reference: "p1", Amount: 0}); !gateway.IsRejected(err) {
		t.Fatalf("zero amount: %v", err)
	}
	q, _ := a.GetPayment(ctx, gateway.QueryRequest{})
	if st, ok := a.MapStatus(q.RawStatus); !ok || st != gateway.StatusPending {
		t.Fatal("polling must never move a payment")
	}
	for outcome, want := range map[string]gateway.Status{"paid": gateway.StatusSucceeded, "expired": gateway.StatusExpired, "failed": gateway.StatusFailed} {
		ev, err := a.SimulateEvent("p1", 1000, "IDR", outcome)
		if err != nil || ev.Status != want || !ev.StatusOK || ev.DedupKey != "fake:simulate:p1:"+outcome {
			t.Fatalf("%s: %+v %v", outcome, ev, err)
		}
	}
	if _, err := a.SimulateEvent("p1", 1, "IDR", "refunded"); !errors.Is(err, gateway.ErrUnknownOutcome) {
		t.Fatalf("unknown outcome: %v", err)
	}
	if _, ok := a.MapStatus("whatever"); ok {
		t.Fatal("unknown raw status must have no mapping")
	}
}
