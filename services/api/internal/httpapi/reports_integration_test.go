package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestListPaymentsHTTP(t *testing.T) {
	w := setup(t)
	owner, ownerB, anon := w.client(t, "owner@a.test"), w.client(t, "owner@b.test"), &http.Client{}
	var ids []string
	for i := 1; i <= 5; i++ {
		ids = append(ids, w.succeeded(t, fmt.Sprintf("L%d", i), i*10000))
	}
	// A pending payment created through the public API carries a QR; a list must never show it.
	c, auth := keyClient(t, w, `["payment:create","payment:read"]`)
	if r, _ := post(t, w, c, `{"merchant_reference":"PEND","amount":5000,"currency":"IDR","payment_method":"QRIS"}`, "idem-list-pend01", auth); r.StatusCode != 201 {
		t.Fatalf("create pending: %d", r.StatusCode)
	}

	// Pagination: 6 payments, 4 per page, newest first, no repeats.
	p1 := decode(t, w.do(t, owner, "GET", "/v1/payments?limit=4", "", nil))
	d1 := p1["data"].([]any)
	if len(d1) != 4 || p1["next_cursor"] == nil {
		t.Fatalf("page 1: %d items, cursor %v", len(d1), p1["next_cursor"])
	}
	if first := d1[0].(map[string]any); first["merchant_reference"] != "PEND" || first["qr_code"] != nil {
		t.Fatalf("newest item or QR leak: %v", first)
	}
	p2 := decode(t, w.do(t, owner, "GET", "/v1/payments?limit=4&cursor="+p1["next_cursor"].(string), "", nil))
	d2 := p2["data"].([]any)
	if len(d2) != 2 || p2["next_cursor"] != nil {
		t.Fatalf("page 2: %d items, cursor %v", len(d2), p2["next_cursor"])
	}
	seen := map[string]bool{}
	for _, it := range append(d1, d2...) {
		id := it.(map[string]any)["id"].(string)
		if seen[id] {
			t.Fatalf("%s repeated across pages", id)
		}
		seen[id] = true
	}

	// Filters.
	if d := decode(t, w.do(t, owner, "GET", "/v1/payments?status=succeeded", "", nil))["data"].([]any); len(d) != 5 {
		t.Fatalf("status filter: %d", len(d))
	}
	if d := decode(t, w.do(t, owner, "GET", "/v1/payments?merchant_reference=ORD-L3", "", nil))["data"].([]any); len(d) != 1 || d[0].(map[string]any)["id"] != ids[2] {
		t.Fatalf("reference filter: %v", d)
	}
	today := time.Now().In(wib).Format("2006-01-02")
	if d := decode(t, w.do(t, owner, "GET", "/v1/payments?from="+today+"&to="+today, "", nil))["data"].([]any); len(d) != 6 {
		t.Fatalf("today's payments: %d", len(d))
	}
	if d := decode(t, w.do(t, owner, "GET", "/v1/payments?to=2000-01-01", "", nil))["data"].([]any); len(d) != 0 {
		t.Fatalf("old range: %d", len(d))
	}
	// An API key with payment:read lists the same tenant; tenant B sees nothing.
	if d := decode(t, w.do(t, anon, "GET", "/v1/payments", "", auth))["data"].([]any); len(d) != 6 {
		t.Fatalf("API key list: %d", len(d))
	}
	if d := decode(t, w.do(t, ownerB, "GET", "/v1/payments", "", nil))["data"].([]any); len(d) != 0 {
		t.Fatalf("tenant B sees %d payments", len(d))
	}

	// Validation and access.
	for _, q := range []string{"status=bogus", "limit=0", "limit=101", "limit=x", "cursor=!!!", "from=yesterday", "to=12/10/2026", "merchant_reference=" + strings.Repeat("a", 65)} {
		if r := w.do(t, owner, "GET", "/v1/payments?"+q, "", nil); r.StatusCode != 400 {
			t.Errorf("%q: %d", q, r.StatusCode)
		}
	}
	if r := w.do(t, anon, "GET", "/v1/payments", "", nil); r.StatusCode != 401 {
		t.Errorf("anon: %d", r.StatusCode)
	}
	_, k := createKey(t, w, owner, `{"name":"create-only","scopes":["payment:create"]}`)
	if r := w.do(t, anon, "GET", "/v1/payments", "", bearer(k["secret"].(string))); r.StatusCode != 403 {
		t.Errorf("key without payment:read: %d", r.StatusCode)
	}
	if r := w.do(t, w.client(t, "finance@erapay.test"), "GET", "/v1/payments", "", nil); r.StatusCode != 403 {
		t.Errorf("platform session: %d", r.StatusCode)
	}
}

func TestReportSummaryHTTP(t *testing.T) {
	w := setup(t)
	owner, ownerB, anon := w.client(t, "owner@a.test"), w.client(t, "owner@b.test"), &http.Client{}
	w.succeeded(t, "RS1", 100000)
	w.succeeded(t, "RS2", 50000)

	s := decode(t, w.do(t, owner, "GET", "/v1/reports/summary", "", nil))
	tot := s["totals"].(map[string]any)
	if num(tot, "succeeded") != 2 || num(tot, "gross") != 150000 || num(tot, "payments") != 2 {
		t.Fatalf("totals: %v", tot)
	}
	if num(tot, "gross") != num(tot, "mdr_fee")+num(tot, "platform_fee")+num(tot, "merchant_net") {
		t.Fatalf("gross != mdr + platform + net: %v", tot)
	}
	daily := s["daily"].([]any)
	today := time.Now().In(wib).Format("2006-01-02")
	last := daily[len(daily)-1].(map[string]any)
	if len(daily) != 30 || s["timezone"] != "Asia/Jakarta" || s["to"] != today || last["date"] != today || num(last, "count") != 2 || num(last, "gross") != 150000 {
		t.Fatalf("daily: %d points, last %v, to %v", len(daily), last, s["to"])
	}

	// Explicit period, a report:read key, and isolation.
	if r := w.do(t, owner, "GET", "/v1/reports/summary?from="+today+"&to="+today, "", nil); r.StatusCode != 200 || len(decode(t, r)["daily"].([]any)) != 1 {
		t.Fatalf("one-day period: %d", r.StatusCode)
	}
	_, rk := createKey(t, w, owner, `{"name":"rep","scopes":["report:read"]}`)
	if r := w.do(t, anon, "GET", "/v1/reports/summary", "", bearer(rk["secret"].(string))); r.StatusCode != 200 {
		t.Errorf("report:read key: %d", r.StatusCode)
	}
	_, pk := createKey(t, w, owner, `{"name":"pay","scopes":["payment:read"]}`)
	if r := w.do(t, anon, "GET", "/v1/reports/summary", "", bearer(pk["secret"].(string))); r.StatusCode != 403 {
		t.Errorf("payment:read key on the report: %d", r.StatusCode)
	}
	if r := w.do(t, w.client(t, "cashier@a.test"), "GET", "/v1/reports/summary", "", nil); r.StatusCode != 403 {
		t.Errorf("cashier: %d", r.StatusCode)
	}
	if r := w.do(t, anon, "GET", "/v1/reports/summary", "", nil); r.StatusCode != 401 {
		t.Errorf("anon: %d", r.StatusCode)
	}
	if tb := decode(t, w.do(t, ownerB, "GET", "/v1/reports/summary", "", nil))["totals"].(map[string]any); num(tb, "payments") != 0 || num(tb, "gross") != 0 {
		t.Fatalf("tenant B totals: %v", tb)
	}
	for _, q := range []string{"from=bogus", "to=2026-13-40", "from=2026-10-12&to=2026-10-01", "from=2020-01-01&to=2026-10-12", "currency=ID"} {
		if r := w.do(t, owner, "GET", "/v1/reports/summary?"+q, "", nil); r.StatusCode != 400 {
			t.Errorf("%q: %d", q, r.StatusCode)
		}
	}
}
