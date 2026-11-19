package midtrans

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// TestLiveSandbox talks to the Midtrans sandbox with MIDTRANS_SANDBOX_KEY (SB-Mid-server-...). It creates
// one unpaid QR of Rp10.000, inquires it by our order id, checks the duplicate behaviour, and expires
// nothing (an unpaid sandbox QR lapses by itself). The webhook signature secret is not exercised here.
func TestLiveSandbox(t *testing.T) {
	key := os.Getenv("MIDTRANS_SANDBOX_KEY")
	if key == "" {
		t.Skip("MIDTRANS_SANDBOX_KEY not set")
	}
	if !strings.HasPrefix(key, "SB-Mid-server-") {
		t.Fatal("refusing to run: not an SB-Mid-server- key")
	}
	a, err := New(Config{BaseURL: SandboxURL, ServerKey: key})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if st := a.TestConnection(ctx, "", false)[0]; !st.OK {
		t.Fatalf("connection: %+v", st)
	}
	ref := "erapay-live-" + time.Now().UTC().Format("060102150405")
	req := gateway.CreateRequest{Reference: ref, Method: "QRIS", Amount: 10000, Currency: "IDR", ExpiresAt: time.Now().Add(15 * time.Minute)}

	if _, err := a.GetPayment(ctx, gateway.QueryRequest{Reference: ref}); !gateway.IsAmbiguous(err) {
		t.Fatalf("inquiry before create must be ambiguous (404), got %v", err)
	}
	res, err := a.CreatePayment(ctx, req)
	if err != nil || res.QRPayload == "" || res.GatewayReference == "" {
		t.Fatalf("create: %+v %v", res, err)
	}
	t.Logf("created id=%s status=%s code=%s", res.GatewayReference, res.RawStatus, res.ResponseCode)

	q, err := a.GetPayment(ctx, gateway.QueryRequest{Reference: ref, GatewayReference: res.GatewayReference})
	if err != nil || q.Amount != 10000 || q.Currency != "IDR" {
		t.Fatalf("query: %+v %v", q, err)
	}
	if st, ok := a.MapStatus(q.RawStatus); !ok || st != gateway.StatusPending {
		t.Errorf("status %q -> %v %v", q.RawStatus, st, ok)
	}

	// Same order id and Idempotency-Key within 5 minutes: Midtrans replays the first answer. Informational only.
	again, err := a.CreatePayment(ctx, gateway.CreateRequest{Reference: ref, Method: "QRIS", Amount: 10000, Currency: "IDR", ExpiresAt: req.ExpiresAt})
	t.Logf("duplicate create (same Idempotency-Key replays the first answer): id=%q err=%v", again.GatewayReference, err)
}
