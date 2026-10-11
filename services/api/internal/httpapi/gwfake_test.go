package httpapi

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testGW is a TEST-ONLY gateway adapter; it is never registered by production code.
type testGW struct {
	calls  atomic.Int32
	create func(gateway.CreateRequest) (gateway.CreateResult, error)
}

func (g *testGW) Code() string { return "gw1" }
func (g *testGW) Capabilities() gateway.Capabilities {
	return gateway.Capabilities{PaymentMethods: []string{"QRIS"}, Inquiry: true}
}
func (g *testGW) CreatePayment(_ context.Context, r gateway.CreateRequest) (gateway.CreateResult, error) {
	g.calls.Add(1)
	return g.create(r)
}
func (g *testGW) GetPayment(context.Context, gateway.QueryRequest) (gateway.QueryResult, error) {
	return gateway.QueryResult{}, gateway.ErrNotSupported
}
func (g *testGW) CancelPayment(context.Context, gateway.CancelRequest) error { return nil }
func (g *testGW) MapStatus(string) (gateway.Status, bool)                    { return "", false }

func newTestGW() *testGW {
	return &testGW{create: func(r gateway.CreateRequest) (gateway.CreateResult, error) {
		return gateway.CreateResult{GatewayReference: "GW-" + r.Reference, QRPayload: "QR-" + r.Reference}, nil
	}}
}

// seedPayments gives both merchants the same pricing plan; only merchant A has an active gateway account.
func seedPayments(t *testing.T, db *pgxpool.Pool, midA, midB string) {
	t.Helper()
	ctx := context.Background()
	var plan string
	if err := db.QueryRow(ctx, `
		INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('std', 1, 'gw1', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	for _, mid := range []string{midA, midB} {
		if _, err := db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
			VALUES ($1,$2, now() - interval '1 day')`, mid, plan); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO merchant_gateway_accounts (merchant_id, gateway, environment, gateway_ref, activation_status)
		VALUES ($1,'gw1','sandbox','MID-A','active')`, midA); err != nil {
		t.Fatal(err)
	}
}
