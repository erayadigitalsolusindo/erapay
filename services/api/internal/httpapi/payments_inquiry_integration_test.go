package httpapi

import (
	"net/http"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// TestPaymentInquiryHTTP: tombol "Cek status" menanyakan ulang status ke gateway
// berdasarkan nomor referensi dan menerapkan hasil (source=inquiry), hanya untuk
// transaksi tenant sendiri.
func TestPaymentInquiryHTTP(t *testing.T) {
	w := setup(t)
	c, auth := keyClient(t, w, `["payment:create","payment:read"]`)
	r, p := post(t, w, c, goodBody, "idem-inq-000001", auth)
	if r.StatusCode != 201 || p["status"] != "pending" {
		t.Fatalf("create: %d %v", r.StatusCode, p)
	}
	id := p["id"].(string)

	// Gateway menjawab PAID dengan amount yang sama → inquiry sukses, status jadi succeeded.
	w.gw.query = func(q gateway.QueryRequest) (gateway.QueryResult, error) {
		return gateway.QueryResult{RawStatus: "PAID", Amount: 10000, Currency: "IDR"}, nil
	}
	w.gw.mapStatus = func(s string) (gateway.Status, bool) {
		if s == "PAID" {
			return gateway.StatusSucceeded, true
		}
		return "", false
	}

	owner := w.client(t, "owner@a.test")
	if r := w.do(t, owner, "POST", "/v1/payments/"+id+"/inquiry", "", nil); r.StatusCode != 200 {
		t.Fatalf("inquiry: %d", r.StatusCode)
	}
	// Status terbaru sudah succeeded.
	got := decode(t, w.do(t, owner, "GET", "/v1/payments/"+id, "", nil))
	if got["status"] != "succeeded" {
		t.Fatalf("after inquiry status=%v", got["status"])
	}
	// Kepemilikan: tenant lain → 404 (bukan membocorkan keberadaan).
	if r := w.do(t, w.client(t, "owner@b.test"), "POST", "/v1/payments/"+id+"/inquiry", "", nil); r.StatusCode != 404 {
		t.Fatalf("B inquiry: %d", r.StatusCode)
	}
	// Tanpa izin read: key hanya create → 403.
	_, kc := createKey(t, w, owner, `{"name":"create-only","scopes":["payment:create"]}`)
	if r := w.do(t, &http.Client{}, "POST", "/v1/payments/"+id+"/inquiry", "", bearer(kc["secret"].(string))); r.StatusCode != 403 {
		t.Fatalf("create-only key: %d", r.StatusCode)
	}
	// Belum login → 401.
	if r := w.do(t, &http.Client{}, "POST", "/v1/payments/"+id+"/inquiry", "", nil); r.StatusCode != 401 {
		t.Fatalf("anon: %d", r.StatusCode)
	}
}
