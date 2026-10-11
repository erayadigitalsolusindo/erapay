package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
)

func TestGetPaymentHTTP(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	db := w.db

	var plan string
	if err := db.QueryRow(ctx, `
		INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('std', 1, 'gw1', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	for _, mid := range []string{w.midA, w.midB} {
		if _, err := db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
			VALUES ($1,$2, now() - interval '1 day')`, mid, plan); err != nil {
			t.Fatal(err)
		}
	}
	svc := payment.NewService(db)
	tx, _ := db.Begin(ctx)
	p, err := svc.CreateTx(ctx, tx, payment.CreateInput{MerchantID: w.midA, MerchantReference: "ORD-1",
		Amount: 1000, Currency: "IDR", PaymentMethod: "QRIS", Gateway: "gw1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	path := "/v1/payments/" + p.ID

	// Merchant A's owner (session) and A's key with payment:read can read it.
	owner := w.client(t, "owner@a.test")
	r := w.do(t, owner, "GET", path, "", nil)
	body := decode(t, r)
	fee, _ := body["fee"].(map[string]any)
	if r.StatusCode != 200 || body["status"] != "created" || fee["merchant_net"] != float64(983) || body["amount"] != float64(1000) {
		t.Fatalf("owner: %d %v", r.StatusCode, body)
	}
	if _, leaked := body["merchant_id"]; leaked {
		t.Fatal("merchant_id exposed")
	}
	_, k := createKey(t, w, owner, `{"name":"pos","scopes":["payment:read"]}`)
	if r := w.do(t, &http.Client{}, "GET", path, "", bearer(k["secret"].(string))); r.StatusCode != 200 {
		t.Fatalf("key read: %d", r.StatusCode)
	}
	// A key without payment:read is forbidden.
	_, kc := createKey(t, w, owner, `{"name":"create-only","scopes":["payment:create"]}`)
	if r := w.do(t, &http.Client{}, "GET", path, "", bearer(kc["secret"].(string))); r.StatusCode != 403 {
		t.Fatalf("create-only key: %d", r.StatusCode)
	}

	// Tenant B (session and key) gets 404, never A's data.
	b := w.client(t, "owner@b.test")
	if r := w.do(t, b, "GET", path, "", nil); r.StatusCode != 404 {
		t.Fatalf("B session: %d", r.StatusCode)
	}
	_, kb := createKey(t, w, b, `{"name":"pos-b","scopes":["payment:read"]}`)
	if r := w.do(t, &http.Client{}, "GET", path+"?merchant_id="+w.midA, "", map[string]string{
		"Authorization": "Bearer " + kb["secret"].(string), "X-Merchant-ID": w.midA}); r.StatusCode != 404 {
		t.Fatalf("B key: %d", r.StatusCode)
	}
	// Unauthenticated, and platform sessions (no tenant), are refused.
	if r := w.do(t, &http.Client{}, "GET", path, "", nil); r.StatusCode != 401 {
		t.Fatalf("anon: %d", r.StatusCode)
	}
	if r := w.do(t, w.client(t, "root@erapay.test"), "GET", path, "", nil); r.StatusCode != 403 {
		t.Fatalf("platform: %d", r.StatusCode)
	}
}
