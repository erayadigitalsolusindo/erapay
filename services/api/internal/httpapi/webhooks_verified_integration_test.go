package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// verifyingGW is TEST-ONLY: it "authenticates" with a header so the HTTP path (signature
// rejection, apply, replay, concurrency) is exercised without any concrete gateway package.
type verifyingGW struct{ *testGW }

func (verifyingGW) VerifyWebhook(h http.Header, _ string, _ []byte) (gateway.WebhookEvent, error) {
	if h.Get("X-Test-Sig") != "valid" {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	return gateway.WebhookEvent{PaymentRef: h.Get("X-Ref"), RawStatus: "PAID", Status: gateway.StatusSucceeded, StatusOK: true,
		Amount: 10000, Currency: "IDR", DedupKey: "gw1:webhook:" + h.Get("X-Id")}, nil
}

func TestVerifiedWebhookAppliesOnceAndRejectsBadSignature(t *testing.T) {
	db := testdb.New(t)
	midA, outA := testdb.SeedMerchant(t, db, "A")
	midB, _ := testdb.SeedMerchant(t, db, "B")
	seedPayments(t, db, midA, midB)
	reg := gateway.NewRegistry()
	reg.Register(verifyingGW{newTestGW()})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := payment.NewService(db)
	flow := payment.NewOrchestrator(svc, db, idempotency.NewStore(db), reg, "sandbox", log)
	srv := httptest.NewServer(NewRouter(Deps{Log: log, DB: fakePinger{}, Redis: fakePinger{}, Gateways: reg, PaymentFlow: flow}))
	defer srv.Close()
	ctx := context.Background()

	newPayment := func(key string) string {
		h, _ := idempotency.HashJSON([]byte(`{"k":"`+key+`"}`), "POST", "/v1/payments")
		out, err := flow.Create(ctx, payment.CreateRequest{MerchantID: midA, OutletID: outA, MerchantReference: key,
			Amount: 10000, Currency: "IDR", PaymentMethod: "QRIS", IdempotencyKey: key, RequestHash: h})
		if err != nil || out.Payment.Status != payment.Pending {
			t.Fatalf("setup: %+v %v", out, err)
		}
		return out.Payment.ID
	}
	send := func(sig, ref, id string) int {
		req, _ := http.NewRequest("POST", srv.URL+"/webhooks/gw1", strings.NewReader(`{}`))
		req.Header.Set("X-Test-Sig", sig)
		req.Header.Set("X-Ref", ref)
		req.Header.Set("X-Id", id)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	status := func(id string) string {
		var s string
		if err := db.QueryRow(ctx, `SELECT status FROM payments WHERE id=$1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	journals := func() int {
		var n int
		_ = db.QueryRow(ctx, `SELECT count(*) FROM ledger_transactions`).Scan(&n)
		return n
	}

	p1 := newPayment("key-http-wh-0001")
	if c := send("forged", p1, "r1"); c != 400 || status(p1) != "pending" {
		t.Fatalf("bad signature: %d status=%s", c, status(p1))
	}
	if c := send("valid", p1, "r1"); c != 200 || status(p1) != "succeeded" || journals() != 1 {
		t.Fatalf("valid: %d status=%s journals=%d", c, status(p1), journals())
	}
	if c := send("valid", p1, "r1"); c != 200 || journals() != 1 {
		t.Fatalf("replay: %d journals=%d", c, journals())
	}

	// A notification for a payment we do not know is acknowledged (a retry cannot fix it) and changes nothing.
	if c := send("valid", "00000000-0000-4000-8000-00000000abcd", "r9"); c != 200 || journals() != 1 {
		t.Fatalf("unknown: %d journals=%d", c, journals())
	}

	// Concurrent retries of the same notification: one ledger posting.
	p2 := newPayment("key-http-wh-0002")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c := send("valid", p2, "r2"); c != 200 {
				t.Errorf("concurrent: %d", c)
			}
		}()
	}
	wg.Wait()
	if status(p2) != "succeeded" || journals() != 2 {
		t.Fatalf("after concurrency: status=%s journals=%d (want 2: one per payment)", status(p2), journals())
	}
}
