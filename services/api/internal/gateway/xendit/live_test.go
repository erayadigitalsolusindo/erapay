package xendit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// TestLiveSandbox talks to Xendit with a test-mode key (XENDIT_SANDBOX_KEY, xnd_development_...).
// It creates one unpaid QR of Rp10.000 and logs what the docs leave unclear (duplicate reference_id).
func TestLiveSandbox(t *testing.T) {
	key := os.Getenv("XENDIT_SANDBOX_KEY")
	if key == "" {
		t.Skip("XENDIT_SANDBOX_KEY not set")
	}
	if len(key) < 16 || key[:16] != "xnd_development_" {
		t.Fatal("refusing to run: not a xnd_development_ key")
	}
	a, err := New(Config{SecretKey: key})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if st := a.TestConnection(ctx, "", false)[0]; !st.OK {
		t.Fatalf("connection: %+v", st)
	}
	ref := "erapay-live-" + time.Now().UTC().Format("060102150405")
	req := gateway.CreateRequest{Reference: ref, Method: "QRIS", Amount: 10000, Currency: "IDR", ExpiresAt: time.Now().Add(15 * time.Minute)}
	res, err := a.CreatePayment(ctx, req)
	if err != nil || res.QRPayload == "" || res.GatewayReference == "" {
		t.Fatalf("create: %+v %v", res, err)
	}
	t.Logf("created id=%s status=%s", res.GatewayReference, res.RawStatus)
	q, err := a.GetPayment(ctx, gateway.QueryRequest{Reference: ref, GatewayReference: res.GatewayReference})
	if err != nil || q.Amount != 10000 {
		t.Fatalf("query: %+v %v", q, err)
	}
	if _, ok := a.MapStatus(q.RawStatus); !ok {
		t.Errorf("unmapped status %q", q.RawStatus)
	}
	t.Logf("query status=%s amount=%d", q.RawStatus, q.Amount)
	// Not asserted: the docs do not say what a repeated reference_id does for QRIS.
	again, err := a.CreatePayment(ctx, req)
	t.Logf("duplicate create: id=%q err=%v", again.GatewayReference, err)
}
