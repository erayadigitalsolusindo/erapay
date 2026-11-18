package finpay

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// TestLiveSandbox talks to the real FINPAY sandbox. Run with FINPAY_SANDBOX_ID and FINPAY_SANDBOX_KEY set.
func TestLiveSandbox(t *testing.T) {
	id, key := os.Getenv("FINPAY_SANDBOX_ID"), os.Getenv("FINPAY_SANDBOX_KEY")
	if id == "" || key == "" {
		t.Skip("FINPAY_SANDBOX_ID/FINPAY_SANDBOX_KEY not set")
	}
	a, err := New(Config{BaseURL: SandboxURL, MerchantID: id, MerchantKey: key, CallbackURL: "https://example.com/webhooks/finpay"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if st := a.TestConnection(ctx, "", false)[0]; !st.OK {
		t.Fatalf("connection: %+v", st)
	}
	ref := "11111111-2222-4333-8444-" + time.Now().UTC().Format("060102150405")
	req := gateway.CreateRequest{Reference: ref, Method: "QRIS", Amount: 10000, Currency: "IDR", ExpiresAt: time.Now().Add(15 * time.Minute)}
	res, err := a.CreatePayment(ctx, req)
	if err != nil || res.QRPayload == "" {
		t.Fatalf("create: %+v %v", res, err)
	}
	again, err := a.CreatePayment(ctx, req) // idempotent: same QR
	if err != nil || again.QRPayload != res.QRPayload {
		t.Fatalf("duplicate create: %+v %v", again, err)
	}
	q, err := a.GetPayment(ctx, gateway.QueryRequest{Reference: ref})
	if err != nil || q.RawStatus != "PENDING" || q.Amount != 10000 {
		t.Fatalf("query: %+v %v", q, err)
	}
	t.Logf("ref=%s status=%s", ref, q.RawStatus)
}
