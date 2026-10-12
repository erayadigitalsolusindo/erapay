package httpapi

import (
	"fmt"
	"net/http"
	"testing"
)

// settled makes a succeeded payment and settles it, so its money is available.
func (w *world) settled(t *testing.T, fin *http.Client, key string, amount int) (id string, net int64) {
	t.Helper()
	id = w.succeeded(t, key, amount)
	pay := decode(t, w.do(t, w.client(t, "owner@a.test"), "GET", "/v1/payments/"+id, "", nil))
	fee := pay["fee"].(map[string]any)
	if r := w.do(t, fin, "POST", "/v1/admin/settlement-batches", fmt.Sprintf(
		`{"gateway":"gw1","batch_ref":"RF-%s","currency":"IDR","settlement_date":"2026-10-12","reported_net":%d,"payment_ids":[%q]}`,
		key, int64(amount)-num(fee, "mdr_fee"), id), nil); r.StatusCode != 201 {
		t.Fatalf("settle %s: %d", key, r.StatusCode)
	}
	return id, num(fee, "merchant_net")
}

func TestRefundFlowHTTP(t *testing.T) {
	w := setup(t)
	owner, fin, ownerB := w.client(t, "owner@a.test"), w.client(t, "finance@erapay.test"), w.client(t, "owner@b.test")
	anon := &http.Client{}
	pid, _ := w.settled(t, fin, "R1", 100000)
	w.settled(t, fin, "R2", 100000) // headroom: fees are not refunded
	hdr := func(k string) map[string]string { return map[string]string{"Idempotency-Key": k} }
	body := func(amount int) string {
		return fmt.Sprintf(`{"amount":%d,"reason":"customer returned the item"}`, amount)
	}
	path := "/v1/payments/" + pid + "/refund"

	// Request rules.
	if r := w.do(t, owner, "POST", path, body(30000), nil); r.StatusCode != 400 {
		t.Fatalf("no Idempotency-Key: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", path, `{"amount":30000,"reason":"abc","merchant_id":"`+w.midB+`"}`, hdr("rf-key-strict-1")); r.StatusCode != 400 {
		t.Fatalf("client-supplied merchant_id: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", path, body(100001), hdr("rf-key-toomuch1")); r.StatusCode != 422 || decode(t, r)["error"].(map[string]any)["code"] != "REFUND_EXCEEDS_PAYMENT" {
		t.Fatalf("above the payment: %d", r.StatusCode)
	}
	r := w.do(t, owner, "POST", path, body(30000), hdr("rf-key-main-001"))
	rf := decode(t, r)
	if r.StatusCode != 201 || rf["status"] != "pending_approval" || num(rf, "amount") != 30000 || rf["payment_id"] != pid {
		t.Fatalf("request: %d %v", r.StatusCode, rf)
	}
	rfID := rf["id"].(string)
	if r := w.do(t, owner, "POST", path, body(30000), hdr("rf-key-main-001")); r.StatusCode != 200 || decode(t, r)["id"] != rfID {
		t.Fatalf("replay: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", path, body(30001), hdr("rf-key-main-001")); r.StatusCode != 409 {
		t.Fatalf("same key, other body: %d", r.StatusCode)
	}

	// The payment keeps its status; the refund shows up beside it, and the balance holds the money.
	pay := decode(t, w.do(t, owner, "GET", "/v1/payments/"+pid, "", nil))
	if pay["status"] != "succeeded" || num(pay, "refunding_amount") != 30000 || num(pay, "refunded_amount") != 0 {
		t.Fatalf("payment: %v", pay)
	}
	if bal := decode(t, w.do(t, owner, "GET", "/v1/balance", "", nil)); num(bal, "on_hold") != 30000 {
		t.Fatalf("balance: %v", bal)
	}

	// Who may do what.
	_, k := createKey(t, w, owner, `{"name":"k","scopes":["payment:create","payment:read","report:read"]}`)
	if r := w.do(t, anon, "POST", path, body(1000), map[string]string{"Idempotency-Key": "rf-key-apikey-1", "Authorization": "Bearer " + k["secret"].(string)}); r.StatusCode != 401 {
		t.Fatalf("API key requesting a refund: %d", r.StatusCode)
	}
	if r := w.do(t, w.client(t, "cashier@a.test"), "POST", path, body(1000), hdr("rf-key-cashier1")); r.StatusCode != 403 {
		t.Fatalf("cashier: %d", r.StatusCode)
	}
	if r := w.do(t, ownerB, "POST", path, body(1000), hdr("rf-key-foreign-1")); r.StatusCode != 404 {
		t.Fatalf("tenant B refunding a payment of A: %d", r.StatusCode)
	}
	if r := w.do(t, ownerB, "GET", "/v1/refunds/"+rfID, "", nil); r.StatusCode != 404 {
		t.Fatalf("tenant B reading a refund of A: %d", r.StatusCode)
	}
	if r := w.do(t, ownerB, "POST", "/v1/refunds/"+rfID+"/cancel", `{}`, nil); r.StatusCode != 404 {
		t.Fatalf("tenant B cancelling: %d", r.StatusCode)
	}
	if lst := decode(t, w.do(t, ownerB, "GET", "/v1/refunds", "", nil))["data"].([]any); len(lst) != 0 {
		t.Fatalf("tenant B list: %v", lst)
	}
	root := w.client(t, "root@erapay.test")
	for _, c := range []*http.Client{owner, root} {
		if r := w.do(t, c, "POST", "/v1/admin/refunds/"+rfID+"/approve", `{}`, nil); r.StatusCode != 403 {
			t.Fatalf("unauthorised approve: %d", r.StatusCode)
		}
	}

	// Finance approves, then records the return of the money.
	if r := w.do(t, fin, "POST", "/v1/admin/refunds/"+rfID+"/mark-refunded", `{"bank_reference":"RF-0001"}`, nil); r.StatusCode != 409 {
		t.Fatalf("mark-refunded before approval: %d", r.StatusCode)
	}
	if r := w.do(t, fin, "POST", "/v1/admin/refunds/"+rfID+"/approve", `{}`, nil); r.StatusCode != 200 || decode(t, r)["status"] != "approved" {
		t.Fatalf("approve: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", "/v1/refunds/"+rfID+"/cancel", `{}`, nil); r.StatusCode != 409 {
		t.Fatalf("cancel after approval: %d", r.StatusCode)
	}
	if r := w.do(t, fin, "POST", "/v1/admin/refunds/"+rfID+"/mark-refunded", `{"bank_reference":""}`, nil); r.StatusCode != 400 {
		t.Fatalf("no reference: %d", r.StatusCode)
	}
	for i := 0; i < 2; i++ {
		if r := w.do(t, fin, "POST", "/v1/admin/refunds/"+rfID+"/mark-refunded", `{"bank_reference":"RF-0001"}`, nil); r.StatusCode != 200 || decode(t, r)["status"] != "refunded" {
			t.Fatalf("mark-refunded #%d: %d", i, r.StatusCode)
		}
	}
	if r := w.do(t, fin, "POST", "/v1/admin/refunds/"+rfID+"/mark-refunded", `{"bank_reference":"RF-0002"}`, nil); r.StatusCode != 409 {
		t.Fatalf("other reference: %d", r.StatusCode)
	}
	pay = decode(t, w.do(t, owner, "GET", "/v1/payments/"+pid, "", nil))
	if pay["status"] != "succeeded" || num(pay, "refunded_amount") != 30000 || num(pay, "refunding_amount") != 0 {
		t.Fatalf("payment after refund: %v", pay)
	}
	if bal := decode(t, w.do(t, owner, "GET", "/v1/balance", "", nil)); num(bal, "on_hold") != 0 {
		t.Fatalf("balance after refund: %v", bal)
	}
	lst := decode(t, w.do(t, owner, "GET", "/v1/refunds?payment_id="+pid+"&status=refunded", "", nil))["data"].([]any)
	if len(lst) != 1 {
		t.Fatalf("merchant list: %v", lst)
	}
	if n := w.countRows(t, `SELECT count(*) FROM audit_logs WHERE action IN ('refund.request','refund.approve','refund.mark_refunded')`); n != 3 {
		t.Fatalf("audit rows = %d", n)
	}
}

func TestRefundAdminAccessControlHTTP(t *testing.T) {
	w := setup(t)
	anon := &http.Client{}
	paths := []struct{ m, p string }{
		{"GET", "/v1/admin/refunds"}, {"GET", "/v1/admin/refunds/" + someUUID},
		{"POST", "/v1/admin/refunds/" + someUUID + "/approve"}, {"POST", "/v1/admin/refunds/" + someUUID + "/mark-refunded"},
	}
	for _, c := range paths {
		if r := w.do(t, anon, c.m, c.p, `{}`, nil); r.StatusCode != 401 {
			t.Errorf("anon %s %s: %d", c.m, c.p, r.StatusCode)
		}
		for _, who := range []string{"owner@a.test", "cashier@a.test", "integrator@a.test"} {
			if r := w.do(t, w.client(t, who), c.m, c.p, `{}`, nil); r.StatusCode != 403 {
				t.Errorf("%s %s %s: %d", who, c.m, c.p, r.StatusCode)
			}
		}
	}
	fin := w.client(t, "finance@erapay.test")
	for _, p := range []string{"/v1/admin/refunds/" + someUUID, "/v1/admin/refunds/not-a-uuid"} {
		if r := w.do(t, fin, "GET", p, "", nil); r.StatusCode != 404 {
			t.Errorf("%s: %d", p, r.StatusCode)
		}
	}
	for _, a := range []string{"approve", "reject", "mark-refunded", "mark-failed"} {
		if r := w.do(t, fin, "POST", "/v1/admin/refunds/"+someUUID+"/"+a, `{"note":"abc","reason":"abc","bank_reference":"abc"}`, nil); r.StatusCode != 404 {
			t.Errorf("%s unknown refund: %d", a, r.StatusCode)
		}
	}
	if r := w.do(t, w.client(t, "owner@a.test"), "POST", "/v1/payments/not-a-uuid/refund", `{"amount":1,"reason":"abc"}`, map[string]string{"Idempotency-Key": "rf-key-baduuid-1"}); r.StatusCode != 404 {
		t.Errorf("malformed payment id: %d", r.StatusCode)
	}
}
