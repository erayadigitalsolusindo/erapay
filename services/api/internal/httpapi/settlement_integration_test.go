package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
)

// succeeded creates a payment through the public API, then reports it paid the way a verified
// webhook would (via the domain service), and returns its id.
func (w *world) succeeded(t *testing.T, key string, amount int) string {
	t.Helper()
	c, auth := keyClient(t, w, `["payment:create","payment:read"]`)
	body := fmt.Sprintf(`{"merchant_reference":"ORD-%s","amount":%d,"currency":"IDR","payment_method":"QRIS"}`, key, amount)
	r, p := post(t, w, c, body, "idem-"+key+"-0001", auth)
	if r.StatusCode != 201 {
		t.Fatalf("create: %d %v", r.StatusCode, p)
	}
	id := p["id"].(string)
	if _, err := payment.NewService(w.db).Transition(context.Background(), payment.TransitionInput{
		PaymentID: id, To: payment.Succeeded, Source: payment.SourceWebhook}); err != nil {
		t.Fatal(err)
	}
	return id
}

func num(m map[string]any, k string) int64 { return int64(m[k].(float64)) }

func TestSettlementAndBalanceHTTP(t *testing.T) {
	w := setup(t)
	owner, fin := w.client(t, "owner@a.test"), w.client(t, "finance@erapay.test")

	bal := func(c *http.Client) map[string]any {
		r := w.do(t, c, "GET", "/v1/balance", "", nil)
		if r.StatusCode != 200 {
			t.Fatalf("balance: %d", r.StatusCode)
		}
		return decode(t, r)
	}
	if b := bal(owner); num(b, "payable") != 0 || num(b, "pending") != 0 || num(b, "available") != 0 {
		t.Fatalf("empty balance: %v", b)
	}

	id := w.succeeded(t, "A1", 10000)
	pay := decode(t, w.do(t, owner, "GET", "/v1/payments/"+id, "", nil))
	fee := pay["fee"].(map[string]any)
	expectedNet := 10000 - num(fee, "mdr_fee")
	if pay["status"] != "succeeded" || pay["settlement_status"] != "expected" {
		t.Fatalf("payment: %v", pay)
	}
	if b := bal(owner); num(b, "pending") != num(fee, "merchant_net") || num(b, "available") != 0 {
		t.Fatalf("pending balance: %v", b)
	}

	// Finance sees the payment as awaiting settlement.
	un := decode(t, w.do(t, fin, "GET", "/v1/admin/unsettled-payments?gateway=gw1&limit=50", "", nil))
	rows := un["data"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["payment_id"] != id || num(rows[0].(map[string]any), "expected_net") != expectedNet {
		t.Fatalf("unsettled: %v", un)
	}

	batch := func(ref string, net int64) *http.Response {
		return w.do(t, fin, "POST", "/v1/admin/settlement-batches", fmt.Sprintf(
			`{"gateway":"gw1","batch_ref":%q,"currency":"IDR","settlement_date":"2026-10-12","reported_net":%d,"payment_ids":[%q]}`, ref, net, id), nil)
	}
	// Wrong figure: 422, nothing booked, exception open.
	r := batch("DOKU-1", expectedNet+500)
	mm := decode(t, r)
	if r.StatusCode != 422 || mm["error"].(map[string]any)["code"] != "SETTLEMENT_MISMATCH" || num(mm, "difference") != 500 || num(mm, "expected_net") != expectedNet {
		t.Fatalf("mismatch: %d %v", r.StatusCode, mm)
	}
	if b := bal(owner); num(b, "available") != 0 {
		t.Fatalf("available after refused batch: %v", b)
	}
	ex := decode(t, w.do(t, fin, "GET", "/v1/admin/reconciliation-exceptions?status=open", "", nil))["data"].([]any)
	if len(ex) != 1 {
		t.Fatalf("exceptions: %v", ex)
	}

	// Right figure books; the identical retry is a 200 replay.
	if r := batch("DOKU-1", expectedNet); r.StatusCode != 201 {
		t.Fatalf("book: %d %v", r.StatusCode, decode(t, r))
	}
	if r := batch("DOKU-1", expectedNet); r.StatusCode != 200 {
		t.Fatalf("replay: %d", r.StatusCode)
	}
	if r := batch("DOKU-1", expectedNet+1); r.StatusCode != 409 {
		t.Fatalf("conflicting content: %d", r.StatusCode)
	}
	if r := batch("DOKU-2", expectedNet); r.StatusCode != 422 { // payment already settled
		t.Fatalf("double settle: %d", r.StatusCode)
	}
	pay = decode(t, w.do(t, owner, "GET", "/v1/payments/"+id, "", nil))
	if pay["status"] != "succeeded" || pay["settlement_status"] != "settled" {
		t.Fatalf("payment after settlement: %v", pay)
	}
	if b := bal(owner); num(b, "pending") != 0 || num(b, "available") != num(fee, "merchant_net") || num(b, "payable") != num(fee, "merchant_net") {
		t.Fatalf("balance after settlement: %v", b)
	}
	if b := bal(w.client(t, "owner@b.test")); num(b, "payable") != 0 {
		t.Fatalf("tenant B sees A's money: %v", b)
	}

	// Resolve the exception with a note (audited); a second resolve is 404.
	exID := ex[0].(map[string]any)["id"].(string)
	if r := w.do(t, fin, "POST", "/v1/admin/reconciliation-exceptions/"+exID+"/resolve", `{"note":"DOKU re-issued the report"}`, nil); r.StatusCode != 204 {
		t.Fatalf("resolve: %d", r.StatusCode)
	}
	if r := w.do(t, fin, "POST", "/v1/admin/reconciliation-exceptions/"+exID+"/resolve", `{"note":"again please"}`, nil); r.StatusCode != 404 {
		t.Fatalf("resolve twice: %d", r.StatusCode)
	}
	if n := w.countRows(t, `SELECT count(*) FROM audit_logs WHERE action IN ('settlement_batch.record','reconciliation_exception.open','reconciliation_exception.resolve')`); n != 3 {
		t.Fatalf("audit rows = %d, want 3", n)
	}
}

func (w *world) countRows(t *testing.T, q string) (n int) {
	t.Helper()
	if err := w.db.QueryRow(context.Background(), q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSettlementAccessControlAndValidationHTTP(t *testing.T) {
	w := setup(t)
	owner, fin := w.client(t, "owner@a.test"), w.client(t, "finance@erapay.test")
	anon := &http.Client{}
	id := w.succeeded(t, "AC", 10000)

	adminGets := []string{"/v1/admin/unsettled-payments", "/v1/admin/settlement-batches", "/v1/admin/reconciliation-exceptions"}
	for _, p := range adminGets {
		if r := w.do(t, anon, "GET", p, "", nil); r.StatusCode != 401 {
			t.Errorf("anon %s: %d", p, r.StatusCode)
		}
		for _, who := range []string{"owner@a.test", "cashier@a.test", "integrator@a.test"} { // merchant users are never admins
			if r := w.do(t, w.client(t, who), "GET", p, "", nil); r.StatusCode != 403 {
				t.Errorf("%s %s: %d", who, p, r.StatusCode)
			}
		}
		if r := w.do(t, fin, "GET", p, "", nil); r.StatusCode != 200 {
			t.Errorf("finance %s: %d", p, r.StatusCode)
		}
	}
	post := `{"gateway":"gw1","batch_ref":"R","currency":"IDR","settlement_date":"2026-10-12","reported_net":1,"payment_ids":["` + id + `"]}`
	if r := w.do(t, owner, "POST", "/v1/admin/settlement-batches", post, nil); r.StatusCode != 403 {
		t.Errorf("owner booking a batch: %d", r.StatusCode)
	}
	// The super admin can look at settlement? Not by default: money actions are Finance's.
	if r := w.do(t, w.client(t, "root@erapay.test"), "POST", "/v1/admin/settlement-batches", post, nil); r.StatusCode != 403 {
		t.Errorf("super_admin booking a batch: %d", r.StatusCode)
	}

	// Balance: cashier has no report:read; platform sessions have no tenant; API keys need the report:read scope.
	if r := w.do(t, w.client(t, "cashier@a.test"), "GET", "/v1/balance", "", nil); r.StatusCode != 403 {
		t.Errorf("cashier balance: %d", r.StatusCode)
	}
	if r := w.do(t, fin, "GET", "/v1/balance", "", nil); r.StatusCode != 403 {
		t.Errorf("platform balance: %d", r.StatusCode)
	}
	_, k := createKey(t, w, owner, `{"name":"rep","scopes":["report:read"]}`)
	if r := w.do(t, anon, "GET", "/v1/balance", "", bearer(k["secret"].(string))); r.StatusCode != 200 {
		t.Errorf("report:read key: %d", r.StatusCode)
	}
	_, k2 := createKey(t, w, owner, `{"name":"pos","scopes":["payment:read"]}`)
	if r := w.do(t, anon, "GET", "/v1/balance", "", bearer(k2["secret"].(string))); r.StatusCode != 403 {
		t.Errorf("payment:read key on balance: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "GET", "/v1/balance?currency=ID", "", nil); r.StatusCode != 400 {
		t.Errorf("bad currency: %d", r.StatusCode)
	}

	for name, body := range map[string]string{
		"bad date":      `{"gateway":"gw1","batch_ref":"R","currency":"IDR","settlement_date":"12/10/2026","reported_net":1,"payment_ids":["` + id + `"]}`,
		"bad uuid":      `{"gateway":"gw1","batch_ref":"R","currency":"IDR","settlement_date":"2026-10-12","reported_net":1,"payment_ids":["x"]}`,
		"unknown field": `{"gateway":"gw1","batch_ref":"R","currency":"IDR","settlement_date":"2026-10-12","reported_net":1,"payment_ids":["` + id + `"],"merchant_id":"` + w.midB + `"}`,
		"no payments":   `{"gateway":"gw1","batch_ref":"R","currency":"IDR","settlement_date":"2026-10-12","reported_net":1,"payment_ids":[]}`,
		"negative net":  `{"gateway":"gw1","batch_ref":"R","currency":"IDR","settlement_date":"2026-10-12","reported_net":-5,"payment_ids":["` + id + `"]}`,
	} {
		if r := w.do(t, fin, "POST", "/v1/admin/settlement-batches", body, nil); r.StatusCode != 400 {
			t.Errorf("%s: %d", name, r.StatusCode)
		}
	}
	for _, q := range []string{"limit=0", "limit=99999", "status=bogus"} {
		if r := w.do(t, fin, "GET", "/v1/admin/reconciliation-exceptions?"+q, "", nil); r.StatusCode != 400 {
			t.Errorf("%s: %d", q, r.StatusCode)
		}
	}
	if w.countRows(t, `SELECT count(*) FROM settlement_batches`) != 0 {
		t.Fatal("a refused request created a batch")
	}
}
