package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
)

// TestPaymentTimelineHTTP covers GET /v1/payments/{id}/timeline: ordering, the
// applied=false evidence row for a conflicting report, the empty-but-existing case,
// and every access-control rule the sibling payment endpoints enforce.
func TestPaymentTimelineHTTP(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	svc := payment.NewService(w.db)

	mk := func(ref string) payment.Payment {
		t.Helper()
		tx, err := w.db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		p, err := svc.CreateTx(ctx, tx, payment.CreateInput{MerchantID: w.midA, MerchantReference: ref,
			Amount: 1000, Currency: "IDR", PaymentMethod: "QRIS", Gateway: "gw1"})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return p
	}
	step := func(in payment.TransitionInput) payment.Result {
		t.Helper()
		r, err := svc.Transition(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	// A fresh payment has no transitions yet: 200 with an empty list, never null.
	fresh := mk("TL-FRESH")
	owner := w.client(t, "owner@a.test")
	r := w.do(t, owner, "GET", "/v1/payments/"+fresh.ID+"/timeline", "", nil)
	body := decode(t, r)
	if r.StatusCode != 200 {
		t.Fatalf("fresh: %d %v", r.StatusCode, body)
	}
	if items, ok := body["data"].([]any); !ok || len(items) != 0 {
		t.Fatalf("fresh timeline must be an empty list, got %v", body["data"])
	}

	// A payment that moved: created -> pending -> succeeded, then a late report that
	// contradicts the terminal status.
	p := mk("TL-1")
	step(payment.TransitionInput{PaymentID: p.ID, To: payment.Pending, Source: payment.SourceInternal,
		Reason: "qr generated"})
	step(payment.TransitionInput{PaymentID: p.ID, To: payment.Succeeded, Source: payment.SourceWebhook,
		Reason: "webhook", RawGatewayStatus: "SUCCESS", DedupKey: "tl:1"})
	conf := step(payment.TransitionInput{PaymentID: p.ID, To: payment.Failed, Source: payment.SourceGatewayResponse,
		Reason: "late report", RawGatewayStatus: "FAILED", DedupKey: "tl:2"})
	if !conf.Conflict || conf.Applied {
		t.Fatalf("expected a recorded conflict: %+v", conf)
	}

	path := "/v1/payments/" + p.ID + "/timeline"
	r = w.do(t, owner, "GET", path, "", nil)
	body = decode(t, r)
	if r.StatusCode != 200 {
		t.Fatalf("owner: %d %v", r.StatusCode, body)
	}
	items, _ := body["data"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(items), items)
	}
	first := items[0].(map[string]any)
	if first["from_status"] != "created" || first["to_status"] != "pending" || first["applied"] != true {
		t.Fatalf("event 1: %v", first)
	}
	second := items[1].(map[string]any)
	if second["to_status"] != "succeeded" || second["source"] != "webhook" || second["raw_gateway_status"] != "SUCCESS" {
		t.Fatalf("event 2: %v", second)
	}
	third := items[2].(map[string]any)
	if third["applied"] != false || third["from_status"] != "succeeded" || third["to_status"] != "failed" {
		t.Fatalf("event 3 must be the unapplied conflict: %v", third)
	}
	// Chronological: each event must not precede the one before it.
	t1 := first["created_at"].(string)
	t2 := second["created_at"].(string)
	t3 := third["created_at"].(string)
	if !(t1 <= t2 && t2 <= t3) {
		t.Fatalf("events out of order: %s %s %s", t1, t2, t3)
	}

	// Reading the timeline must never change the payment (it reports, not applies).
	r = w.do(t, owner, "GET", "/v1/payments/"+p.ID, "", nil)
	if r.StatusCode != 200 {
		t.Fatalf("re-read payment: %d", r.StatusCode)
	}
	if decode(t, r)["status"] != "succeeded" {
		t.Fatal("reading the timeline changed the payment status")
	}

	// API key with payment:read works; without it, 403.
	_, k := createKey(t, w, owner, `{"name":"pos-tl","scopes":["payment:read"]}`)
	if got := w.do(t, &http.Client{}, "GET", path, "", bearer(k["secret"].(string))); got.StatusCode != 200 {
		t.Fatalf("key read: %d", got.StatusCode)
	}
	_, kc := createKey(t, w, owner, `{"name":"create-only-tl","scopes":["payment:create"]}`)
	if got := w.do(t, &http.Client{}, "GET", path, "", bearer(kc["secret"].(string))); got.StatusCode != 403 {
		t.Fatalf("create-only key: %d", got.StatusCode)
	}

	// Tenant B must not learn whether the payment exists.
	if got := w.do(t, w.client(t, "owner@b.test"), "GET", path, "", nil); got.StatusCode != 404 {
		t.Fatalf("B session: %d", got.StatusCode)
	}
	_, kb := createKey(t, w, w.client(t, "owner@b.test"), `{"name":"pos-b-tl","scopes":["payment:read"]}`)
	if got := w.do(t, &http.Client{}, "GET", path, "", map[string]string{
		"Authorization": "Bearer " + kb["secret"].(string), "X-Merchant-ID": w.midA}); got.StatusCode != 404 {
		t.Fatalf("B key: %d", got.StatusCode)
	}

	// Unknown payment id is 404 for the owner too.
	if got := w.do(t, owner, "GET", "/v1/payments/00000000-0000-4000-8000-000000000000/timeline", "", nil); got.StatusCode != 404 {
		t.Fatalf("unknown id: %d", got.StatusCode)
	}
	// Anonymous and platform sessions are refused.
	if got := w.do(t, &http.Client{}, "GET", path, "", nil); got.StatusCode != 401 {
		t.Fatalf("anon: %d", got.StatusCode)
	}
	if got := w.do(t, w.client(t, "root@erapay.test"), "GET", path, "", nil); got.StatusCode != 403 {
		t.Fatalf("platform: %d", got.StatusCode)
	}
}
