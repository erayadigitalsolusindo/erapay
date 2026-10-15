package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func dewaPlatformCookie(t *testing.T, w *world, email string) map[string]string {
	t.Helper()
	uid, err := w.svc.CreateUser(t.Context(), email, itPW)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AddPlatformRole(t, w.db, uid, "dewa")
	lr := w.do(t, &http.Client{}, "POST", "/v1/platform/auth/login", `{"email":"`+email+`","password":"`+itPW+`"}`, nil)
	if lr.StatusCode != 200 {
		t.Fatalf("dewa login: %d", lr.StatusCode)
	}
	return map[string]string{"Cookie": platformCookie + "=" + readCookie(lr, platformCookie)}
}

func countAudit(t *testing.T, w *world, where string, args ...any) int {
	t.Helper()
	var n int
	if err := w.db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDewaImpersonation(t *testing.T) {
	w := setup(t)
	dewa := dewaPlatformCookie(t, w, "dewa@erapay.test")
	var dewaID string
	if err := w.db.QueryRow(t.Context(), `SELECT id::text FROM users WHERE email='dewa@erapay.test'`).Scan(&dewaID); err != nil {
		t.Fatal(err)
	}
	c := &http.Client{}

	// Alasan wajib; merchant harus ada; tanpa sesi panel ditolak.
	for _, tc := range []struct {
		body string
		hdr  map[string]string
		want int
	}{
		{`{"merchant_id":"` + w.midA + `","reason":"singkat"}`, dewa, 400},
		{`{"merchant_id":"00000000-0000-0000-0000-000000000000","reason":"cek tiket support #123"}`, dewa, 404},
		{`{"merchant_id":"bukan-uuid","reason":"cek tiket support #123"}`, dewa, 400},
		{`{"merchant_id":"` + w.midA + `","reason":"cek tiket support #123"}`, nil, 401},
	} {
		if r := w.do(t, c, "POST", "/v1/platform/impersonate", tc.body, tc.hdr); r.StatusCode != tc.want {
			t.Fatalf("impersonate %s: got %d want %d", tc.body, r.StatusCode, tc.want)
		}
	}
	if n := countAudit(t, w, `action='impersonation.start'`); n != 0 {
		t.Fatalf("failed attempts must not create start rows, got %d", n)
	}

	// Staf platform non-DEWA (super_admin) dan merchant tidak bisa memulai impersonasi.
	root := w.client(t, "root@erapay.test")
	if r := w.do(t, root, "POST", "/v1/platform/impersonate", `{"merchant_id":"`+w.midA+`","reason":"cek tiket support #123"}`, nil); r.StatusCode != 401 {
		t.Fatalf("super_admin impersonate: %d", r.StatusCode)
	}

	// Mulai impersonasi merchant A.
	r := w.do(t, c, "POST", "/v1/platform/impersonate", `{"merchant_id":"`+w.midA+`","reason":"cek tiket support #123"}`, dewa)
	if r.StatusCode != 200 {
		t.Fatalf("impersonate: %d", r.StatusCode)
	}
	tok := readCookie(r, sessionCookie)
	if tok == "" {
		t.Fatal("no tenant session cookie")
	}
	imp := map[string]string{"Cookie": sessionCookie + "=" + tok}
	if n := countAudit(t, w, `action='impersonation.start' AND actor_id=$1 AND entity_id=$2 AND metadata->>'reason'='cek tiket support #123'`, dewaID, w.midA); n != 1 {
		t.Fatalf("start audit rows: %d", n)
	}

	// /auth/me menandai impersonasi; tenant = A, bukan B.
	me := decode(t, w.do(t, c, "GET", "/v1/auth/me", "", imp))
	if me["impersonating"] != true || me["merchant_id"] != w.midA {
		t.Fatalf("me: %v", me)
	}
	if r := w.do(t, c, "GET", "/v1/merchant/profile", "", imp); r.StatusCode != 200 {
		t.Fatalf("profile: %d", r.StatusCode)
	}
	if menus := jsonStr(decode(t, w.do(t, c, "GET", "/v1/menus", "", imp))); !strings.Contains(menus, "/dashboard/api-keys") {
		t.Fatalf("impersonated sidebar should follow merchant_owner: %s", menus)
	}

	// Operasi sebagai tenant: tulis (membuat API key) lalu baca.
	if r := w.do(t, c, "POST", "/v1/api-keys", `{"name":"dibuat DEWA","scopes":["payment:read"]}`, imp); r.StatusCode >= 400 {
		t.Fatalf("create api key as tenant: %d", r.StatusCode)
	}
	if r := w.do(t, c, "GET", "/v1/api-keys", "", imp); r.StatusCode != 200 {
		t.Fatalf("list api keys: %d", r.StatusCode)
	}

	// Setiap request tercatat atas nama DEWA, bukan tenant; method + path ikut disimpan.
	if n := countAudit(t, w, `action='impersonation.request' AND actor_id=$1 AND metadata->>'method'='POST' AND metadata->>'path'='/v1/api-keys' AND metadata->>'mutating'='true'`, dewaID); n != 1 {
		t.Fatalf("mutating request audit rows: %d", n)
	}
	if n := countAudit(t, w, `action='impersonation.request' AND metadata->>'path'='/v1/auth/me'`); n != 0 {
		t.Fatalf("polling endpoints must stay quiet, got %d", n)
	}
	// Audit bawaan handler (api_key.created) juga memakai id DEWA sebagai aktor.
	if n := countAudit(t, w, `actor_id=$1 AND action LIKE 'api_key%'`, dewaID); n < 1 {
		t.Fatalf("handler audit should carry DEWA as actor, got %d", n)
	}

	// Akun DEWA sendiri (MFA) tertutup selama impersonasi.
	if r := w.do(t, c, "GET", "/v1/auth/mfa/status", "", imp); r.StatusCode != 403 {
		t.Fatalf("mfa while impersonating: %d", r.StatusCode)
	}
	// Sesi impersonasi tidak membuka panel platform maupun rute admin lintas-tenant.
	if r := w.do(t, c, "GET", "/v1/platform/admin/roles", "", imp); r.StatusCode != 401 {
		t.Fatalf("impersonation cookie must not open panel: %d", r.StatusCode)
	}
	if r := w.do(t, c, "GET", "/v1/admin/merchants", "", imp); r.StatusCode != 403 {
		t.Fatalf("impersonation must not reach /admin: %d", r.StatusCode)
	}

	// DEWA membaca jejak audit di panel.
	ar := decode(t, w.do(t, c, "GET", "/v1/platform/audit-logs?prefix=impersonation.&merchant_id="+w.midA, "", dewa))
	s := jsonStr(ar)
	for _, want := range []string{"impersonation.start", "impersonation.request", "dewa@erapay.test", "cek tiket support #123"} {
		if !strings.Contains(s, want) {
			t.Fatalf("audit-logs missing %q: %s", want, s)
		}
	}
	// Tenant biasa tidak bisa membaca endpoint audit panel.
	owner := w.client(t, "owner@a.test")
	if r := w.do(t, owner, "GET", "/v1/platform/audit-logs", "", nil); r.StatusCode != 401 {
		t.Fatalf("tenant reading platform audit: %d", r.StatusCode)
	}

	// Impersonasi baru menutup yang lama (satu tenant aktif) dan mencatat penutupannya.
	r2 := w.do(t, c, "POST", "/v1/platform/impersonate", `{"merchant_id":"`+w.midB+`","reason":"audit silang merchant B"}`, dewa)
	tokB := readCookie(r2, sessionCookie)
	if r2.StatusCode != 200 || tokB == "" {
		t.Fatalf("second impersonate: %d", r2.StatusCode)
	}
	if r := w.do(t, c, "GET", "/v1/merchant/profile", "", imp); r.StatusCode != 401 {
		t.Fatalf("replaced session should be dead: %d", r.StatusCode)
	}
	if n := countAudit(t, w, `action='impersonation.end' AND metadata->>'cause'='replaced'`); n != 1 {
		t.Fatalf("replaced end rows: %d", n)
	}

	// Logout menutup sesi dan mencatat impersonation.end.
	impB := map[string]string{"Cookie": sessionCookie + "=" + tokB}
	if r := w.do(t, c, "POST", "/v1/auth/logout", "{}", impB); r.StatusCode != 204 {
		t.Fatalf("logout: %d", r.StatusCode)
	}
	if n := countAudit(t, w, `action='impersonation.end' AND metadata->>'cause'='logout' AND entity_id=$1`, w.midB); n != 1 {
		t.Fatalf("logout end rows: %d", n)
	}

	// Cabut peran DEWA → sesi impersonasi aktif langsung mati.
	r3 := w.do(t, c, "POST", "/v1/platform/impersonate", `{"merchant_id":"`+w.midA+`","reason":"uji pencabutan peran"}`, dewa)
	impC := map[string]string{"Cookie": sessionCookie + "=" + readCookie(r3, sessionCookie)}
	if r := w.do(t, c, "GET", "/v1/merchant/profile", "", impC); r.StatusCode != 200 {
		t.Fatalf("pre-revoke: %d", r.StatusCode)
	}
	if _, err := w.db.Exec(t.Context(), `DELETE FROM user_roles WHERE user_id=$1`, dewaID); err != nil {
		t.Fatal(err)
	}
	if r := w.do(t, c, "GET", "/v1/merchant/profile", "", impC); r.StatusCode != 401 {
		t.Fatalf("revoked DEWA keeps tenant session: %d", r.StatusCode)
	}
}
