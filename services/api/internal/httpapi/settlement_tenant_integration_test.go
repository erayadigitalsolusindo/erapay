package httpapi

import "testing"

// Merchant melihat pembayaran yang belum di-settle hanya miliknya; tenant berasal dari sesi.
func TestMyUnsettledTenantIsolation(t *testing.T) {
	w := setup(t)
	id := w.succeeded(t, "T1", 10000) // merchant A

	list := func(email string) (int, []any) {
		c := w.client(t, email)
		r := w.do(t, c, "GET", "/v1/settlement/unsettled?limit=50&merchant_id="+w.midA, "", nil)
		if r.StatusCode != 200 {
			return r.StatusCode, nil
		}
		return 200, decode(t, r)["data"].([]any)
	}

	code, rows := list("owner@a.test")
	if code != 200 || len(rows) != 1 || rows[0].(map[string]any)["payment_id"] != id {
		t.Fatalf("owner A: %d %v", code, rows)
	}
	if rows[0].(map[string]any)["merchant_id"] != w.midA {
		t.Fatalf("foreign tenant row: %v", rows[0])
	}
	// Merchant B tidak melihat milik A, walau mengirim merchant_id A di query.
	if code, rows := list("owner@b.test"); code != 200 || len(rows) != 0 {
		t.Fatalf("owner B must not see A: %d %v", code, rows)
	}
	// Role tanpa settlement:read (kasir) ditolak; staf platform tidak punya sesi tenant.
	if code, _ := list("cashier@a.test"); code != 403 {
		t.Fatalf("cashier: %d", code)
	}
	if code, _ := list("finance@erapay.test"); code == 200 {
		t.Fatalf("platform session must not use tenant endpoint")
	}
}

// Menu merchant dinamis: tampilan tenant finance muncul untuk owner, review merchant tidak,
// dan matriks menolak memberikan menu platform murni ke role merchant.
func TestMerchantMenusDynamic(t *testing.T) {
	w := setup(t)
	menus := func(email string) map[string]bool {
		r := w.do(t, w.client(t, email), "GET", "/v1/menus", "", nil)
		if r.StatusCode != 200 {
			t.Fatalf("menus %s: %d", email, r.StatusCode)
		}
		out := map[string]bool{}
		for _, h := range decode(t, r)["data"].([]any) {
			out[h.(string)] = true
		}
		return out
	}
	m := menus("owner@a.test")
	for _, h := range []string{"/dashboard/finance/settlement", "/dashboard/finance/payouts", "/dashboard/finance/refunds", "/dashboard/merchant"} {
		if !m[h] {
			t.Fatalf("owner must see %s: %v", h, m)
		}
	}
	if m["/dashboard/finance/merchants"] || m["/dashboard/admin/access"] {
		t.Fatalf("owner must not see platform-only menus: %v", m)
	}

	var ownerRole string
	if err := w.db.QueryRow(t.Context(), `SELECT id::text FROM roles WHERE code='merchant_owner'`).Scan(&ownerRole); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.SetMenuAccess(t.Context(), "/dashboard/finance/merchants", ownerRole, true, "", ""); err == nil {
		t.Fatal("platform-only menu must not be assignable to a merchant role")
	}
	// Dicabut dari matriks -> hilang dari sidebar tanpa kode.
	if err := w.svc.SetMenuAccess(t.Context(), "/dashboard/finance/settlement", ownerRole, false, "", ""); err != nil {
		t.Fatal(err)
	}
	if menus("owner@a.test")["/dashboard/finance/settlement"] {
		t.Fatal("revoked menu still visible")
	}
}
