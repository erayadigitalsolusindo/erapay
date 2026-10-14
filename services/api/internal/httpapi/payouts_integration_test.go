package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const someUUID = "11111111-1111-4111-8111-111111111111"

// Executor endpoints: Finance executes an approved payout; without a disbursement
// gateway the API says so, and the dry-run submit leaves the confirmation to Finance.
func TestPayoutExecutorHTTP(t *testing.T) {
	w := setup(t)
	owner, fin := w.client(t, "owner@a.test"), w.client(t, "finance@erapay.test")

	r := w.do(t, owner, "POST", "/v1/bank-accounts", `{"bank_code":"014","account_number":"1234567890","account_name":"PT Merchant A"}`, nil)
	acctID := decode(t, r)["id"].(string)
	if r.StatusCode != 201 {
		t.Fatalf("add account: %d", r.StatusCode)
	}
	if r := w.do(t, fin, "POST", "/v1/admin/bank-accounts/"+acctID+"/verify", `{}`, nil); r.StatusCode != 200 {
		t.Fatalf("verify: %d", r.StatusCode)
	}
	id := w.succeeded(t, "PE1", 100000)
	pay := decode(t, w.do(t, owner, "GET", "/v1/payments/"+id, "", nil))
	fees := pay["fee"].(map[string]any)
	reported := num(fees, "merchant_net") + num(fees, "platform_fee") // settlement nets off MDR only
	if r := w.do(t, fin, "POST", "/v1/admin/settlement-batches", fmt.Sprintf(
		`{"gateway":"gw1","batch_ref":"PE-B1","currency":"IDR","settlement_date":"2026-10-12","reported_net":%d,"payment_ids":[%q]}`, reported, id), nil); r.StatusCode != 201 {
		t.Fatalf("settle: %d", r.StatusCode)
	}
	po := decode(t, w.do(t, owner, "POST", "/v1/payouts",
		fmt.Sprintf(`{"bank_account_id":%q,"amount":20000,"currency":"IDR"}`, acctID), map[string]string{"Idempotency-Key": "pe-key-1"}))
	if po["status"] != "pending_approval" {
		t.Fatalf("request: %v", po)
	}
	poID := po["id"].(string)
	if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/approve", `{}`, nil); r.StatusCode != 200 {
		t.Fatalf("approve: %d", r.StatusCode)
	}

	// No disbursement gateway configured: the endpoint is unavailable, nothing changed.
	if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/execute", `{}`, nil); r.StatusCode != 503 {
		t.Fatalf("execute without gateway: %d", r.StatusCode)
	}
	// Only Finance-level roles may execute (payout:execute).
	if r := w.do(t, owner, "POST", "/v1/admin/payouts/"+poID+"/execute", `{}`, nil); r.StatusCode != 403 {
		t.Fatalf("owner execute: %d", r.StatusCode)
	}

	d := &testDisburser{code: "gw1", transfer: gateway.TransferResult{GatewayReference: "GW-TRF-1"}}
	w.payouts.SetDisburser(d)
	r = w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/execute", `{}`, nil)
	po = decode(t, r)
	if r.StatusCode != 200 || po["status"] != "submitted" || po["gateway"] != "dry-run" || d.calls.Load() != 0 {
		t.Fatalf("dry-run submit: %d %v calls=%d", r.StatusCode, po, d.calls.Load())
	}
	// A repeated execute cannot double-send: the payout is no longer approved.
	if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/execute", `{}`, nil); r.StatusCode != 409 {
		t.Fatalf("second execute: %d", r.StatusCode)
	}
	// A dry-run payout has no gateway transfer to check: it stays manual.
	if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/check-status", `{}`, nil); r.StatusCode != 409 {
		t.Fatalf("check-status on dry-run: %d", r.StatusCode)
	}
	// Finance still confirms manually; the not-yet-instrumented payout stays the source of truth.
	if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/mark-paid", `{"bank_reference":"TRF-0001"}`, nil); r.StatusCode != 200 || decode(t, r)["status"] != "paid" {
		t.Fatalf("mark-paid: %d", r.StatusCode)
	}
}

func TestPayoutFlowHTTP(t *testing.T) {
	w := setup(t)
	owner, fin := w.client(t, "owner@a.test"), w.client(t, "finance@erapay.test")
	anon := &http.Client{}

	// Owner registers a destination; the full number never comes back.
	r := w.do(t, owner, "POST", "/v1/bank-accounts", `{"bank_code":"014","account_number":"1234567890","account_name":"PT Merchant A"}`, nil)
	acct := decode(t, r)
	if r.StatusCode != 201 || acct["status"] != "pending_verification" || acct["account_masked"] != "••••7890" || strings.Contains(fmt.Sprint(acct), "1234567890") {
		t.Fatalf("add account: %d %v", r.StatusCode, acct)
	}
	acctID := acct["id"].(string)
	body := func(amount int) string {
		return fmt.Sprintf(`{"bank_account_id":%q,"amount":%d,"currency":"IDR"}`, acctID, amount)
	}
	hdr := func(k string) map[string]string { return map[string]string{"Idempotency-Key": k} }

	// Fund merchant A with SETTLED money (payout only ever uses settled funds).
	id := w.succeeded(t, "PO1", 100000)
	pay := decode(t, w.do(t, owner, "GET", "/v1/payments/"+id, "", nil))
	fee := pay["fee"].(map[string]any)
	net, expected := num(fee, "merchant_net"), 100000-num(fee, "mdr_fee")

	// Not verified yet, then verified by Finance (the creator cannot).
	if r := w.do(t, owner, "POST", "/v1/payouts", body(20000), hdr("po-key-unverified")); r.StatusCode != 422 {
		t.Fatalf("unverified account: %d", r.StatusCode)
	}
	pend := decode(t, w.do(t, fin, "GET", "/v1/admin/bank-accounts?status=pending_verification", "", nil))["data"].([]any)
	if len(pend) != 1 {
		t.Fatalf("pending accounts: %v", pend)
	}
	if r := w.do(t, owner, "POST", "/v1/admin/bank-accounts/"+acctID+"/verify", `{}`, nil); r.StatusCode != 403 {
		t.Fatalf("merchant owner verifying: %d", r.StatusCode)
	}
	if r := w.do(t, fin, "POST", "/v1/admin/bank-accounts/"+acctID+"/verify", `{}`, nil); r.StatusCode != 200 {
		t.Fatalf("verify: %d", r.StatusCode)
	}

	// Unsettled funds cannot be paid out yet.
	if r := w.do(t, owner, "POST", "/v1/payouts", body(20000), hdr("po-key-unsettled")); r.StatusCode != 422 || decode(t, r)["error"].(map[string]any)["code"] != "INSUFFICIENT_BALANCE" {
		t.Fatalf("payout of unsettled funds: %d", r.StatusCode)
	}
	if r := w.do(t, fin, "POST", "/v1/admin/settlement-batches", fmt.Sprintf(
		`{"gateway":"gw1","batch_ref":"PO-B1","currency":"IDR","settlement_date":"2026-10-12","reported_net":%d,"payment_ids":[%q]}`, expected, id), nil); r.StatusCode != 201 {
		t.Fatalf("settle: %d", r.StatusCode)
	}

	// Request: header required, strict body, idempotent, conflict-aware, limited by available.
	if r := w.do(t, owner, "POST", "/v1/payouts", body(20000), nil); r.StatusCode != 400 {
		t.Fatalf("no Idempotency-Key: %d", r.StatusCode)
	}
	strict := fmt.Sprintf(`{"bank_account_id":%q,"amount":20000,"currency":"IDR","merchant_id":%q}`, acctID, w.midB)
	if r := w.do(t, owner, "POST", "/v1/payouts", strict, hdr("po-key-strict-1")); r.StatusCode != 400 {
		t.Fatalf("client-supplied merchant_id: %d", r.StatusCode)
	}
	r = w.do(t, owner, "POST", "/v1/payouts", body(20000), hdr("po-key-main-001"))
	po := decode(t, r)
	if r.StatusCode != 201 || po["status"] != "pending_approval" || po["dry_run"] != true || num(po, "amount") != 20000 {
		t.Fatalf("request: %d %v", r.StatusCode, po)
	}
	poID := po["id"].(string)
	if r := w.do(t, owner, "POST", "/v1/payouts", body(20000), hdr("po-key-main-001")); r.StatusCode != 200 || decode(t, r)["id"] != poID {
		t.Fatalf("replay: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", "/v1/payouts", body(20001), hdr("po-key-main-001")); r.StatusCode != 409 {
		t.Fatalf("same key, other body: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", "/v1/payouts", body(int(net)), hdr("po-key-toomuch1")); r.StatusCode != 422 {
		t.Fatalf("more than available: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", "/v1/payouts", body(5), hdr("po-key-toosmall")); r.StatusCode != 422 {
		t.Fatalf("below minimum: %d", r.StatusCode)
	}
	bal := decode(t, w.do(t, owner, "GET", "/v1/balance", "", nil))
	if num(bal, "in_payout") != 20000 || num(bal, "available") != net-20000 {
		t.Fatalf("balance: %v", bal)
	}

	// Money-out is for dashboard sessions with the right role, never API keys or other tenants.
	_, k := createKey(t, w, owner, `{"name":"k","scopes":["payment:create","payment:read","report:read"]}`)
	keyHdr := map[string]string{"Idempotency-Key": "po-key-apikey-1", "Authorization": "Bearer " + k["secret"].(string)}
	if r := w.do(t, anon, "POST", "/v1/payouts", body(20000), keyHdr); r.StatusCode != 401 {
		t.Fatalf("API key requesting a payout: %d", r.StatusCode)
	}
	if r := w.do(t, w.client(t, "cashier@a.test"), "POST", "/v1/payouts", body(20000), hdr("po-key-cashier1")); r.StatusCode != 403 {
		t.Fatalf("cashier requesting a payout: %d", r.StatusCode)
	}
	ownerB := w.client(t, "owner@b.test")
	if r := w.do(t, ownerB, "GET", "/v1/payouts/"+poID, "", nil); r.StatusCode != 404 {
		t.Fatalf("tenant B reads a payout of A: %d", r.StatusCode)
	}
	if r := w.do(t, ownerB, "POST", "/v1/payouts/"+poID+"/cancel", `{}`, nil); r.StatusCode != 404 {
		t.Fatalf("tenant B cancels a payout of A: %d", r.StatusCode)
	}
	if r := w.do(t, ownerB, "POST", "/v1/payouts", body(20000), hdr("po-key-foreign-1")); r.StatusCode != 422 {
		t.Fatalf("tenant B using an account of A: %d", r.StatusCode)
	}
	if lst := decode(t, w.do(t, ownerB, "GET", "/v1/payouts", "", nil))["data"].([]any); len(lst) != 0 {
		t.Fatalf("tenant B list: %v", lst)
	}
	if r := w.do(t, fin, "GET", "/v1/payouts", "", nil); r.StatusCode != 403 {
		t.Fatalf("platform session on a tenant route: %d", r.StatusCode)
	}

	// Approval: not the merchant, not the super admin; Finance only. Then the destination is revealed (audited).
	root := w.client(t, "root@erapay.test")
	for _, c := range []*http.Client{owner, root} {
		if r := w.do(t, c, "POST", "/v1/admin/payouts/"+poID+"/approve", `{}`, nil); r.StatusCode != 403 {
			t.Fatalf("unauthorised approve: %d", r.StatusCode)
		}
	}
	if r := w.do(t, fin, "GET", "/v1/admin/payouts/"+poID+"/destination", "", nil); r.StatusCode != 404 {
		t.Fatalf("destination before approval: %d", r.StatusCode)
	}
	if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/approve", `{}`, nil); r.StatusCode != 200 || decode(t, r)["status"] != "approved" {
		t.Fatalf("approve: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "GET", "/v1/admin/payouts/"+poID+"/destination", "", nil); r.StatusCode != 403 {
		t.Fatalf("merchant reading the destination: %d", r.StatusCode)
	}
	dr := w.do(t, fin, "GET", "/v1/admin/payouts/"+poID+"/destination", "", nil)
	if d := decode(t, dr); dr.StatusCode != 200 || d["account_number"] != "1234567890" || dr.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("destination: %d %v", dr.StatusCode, d)
	}
	if r := w.do(t, owner, "POST", "/v1/payouts/"+poID+"/cancel", `{}`, nil); r.StatusCode != 409 {
		t.Fatalf("merchant cancelling an approved payout: %d", r.StatusCode)
	}

	// Execution is recorded by Finance with the bank's reference; repeats are safe.
	if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/mark-paid", `{"bank_reference":""}`, nil); r.StatusCode != 400 {
		t.Fatalf("mark-paid without a reference: %d", r.StatusCode)
	}
	for i := 0; i < 2; i++ {
		if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/mark-paid", `{"bank_reference":"TRF-0001"}`, nil); r.StatusCode != 200 || decode(t, r)["status"] != "paid" {
			t.Fatalf("mark-paid #%d: %d", i, r.StatusCode)
		}
	}
	if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+poID+"/mark-paid", `{"bank_reference":"TRF-0002"}`, nil); r.StatusCode != 409 {
		t.Fatalf("mark-paid with another reference: %d", r.StatusCode)
	}
	got := decode(t, w.do(t, owner, "GET", "/v1/payouts/"+poID, "", nil))
	if got["status"] != "paid" || got["bank_reference"] != "TRF-0001" {
		t.Fatalf("merchant view: %v", got)
	}
	bal = decode(t, w.do(t, owner, "GET", "/v1/balance", "", nil))
	if num(bal, "in_payout") != 0 || num(bal, "payable") != net-20000 || num(bal, "available") != net-20000 {
		t.Fatalf("balance after paid: %v", bal)
	}
	if n := w.countRows(t, `SELECT count(*) FROM audit_logs WHERE action IN
		('payout.request','payout.approve','payout.mark_paid','payout.reveal_destination','bank_account.create','bank_account.verify')`); n != 6 { // create, verify, request, approve, reveal, mark_paid: refused and repeated calls add none
		t.Fatalf("audit rows = %d", n)
	}
}

func TestPayoutAdminAccessControlHTTP(t *testing.T) {
	w := setup(t)
	anon := &http.Client{}
	paths := []struct{ m, p string }{
		{"GET", "/v1/admin/payouts"}, {"GET", "/v1/admin/bank-accounts"},
		{"POST", "/v1/admin/payouts/" + someUUID + "/approve"},
		{"POST", "/v1/admin/payouts/" + someUUID + "/mark-paid"},
		{"GET", "/v1/admin/payouts/" + someUUID + "/destination"},
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
	// Unknown and malformed ids are 404, never a 500.
	fin := w.client(t, "finance@erapay.test")
	for _, p := range []string{"/v1/admin/payouts/" + someUUID, "/v1/admin/payouts/not-a-uuid"} {
		if r := w.do(t, fin, "GET", p, "", nil); r.StatusCode != 404 {
			t.Errorf("%s: %d", p, r.StatusCode)
		}
	}
	for _, c := range []string{"approve", "reject", "mark-paid", "mark-failed"} {
		if r := w.do(t, fin, "POST", "/v1/admin/payouts/"+someUUID+"/"+c, `{"note":"abc","reason":"abc","bank_reference":"abc"}`, nil); r.StatusCode != 404 {
			t.Errorf("%s on an unknown payout: %d", c, r.StatusCode)
		}
	}
	if r := w.do(t, fin, "GET", "/v1/admin/payouts?limit=0", "", nil); r.StatusCode != 400 {
		t.Errorf("limit=0: %d", r.StatusCode)
	}
}
