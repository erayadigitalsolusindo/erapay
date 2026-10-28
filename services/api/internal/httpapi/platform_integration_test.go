package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

const platformSetupToken = "plat-setup-token-0123456789abcdefghijklmnopqrstuvwxyz"

// platformWorld builds the smallest real router for the DEWA panel: only Auth is
// wired (the panel needs nothing else), against a live isolated database.
type platformWorld struct {
	srv *httptest.Server
	db  *pgxpool.Pool
}

func newPlatformWorld(t *testing.T) *platformWorld {
	t.Helper()
	db := testdb.New(t)
	svc := auth.NewService(db, auth.DefaultConfig())
	w := &platformWorld{db: db}
	w.srv = httptest.NewServer(NewRouter(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: fakePinger{}, Redis: fakePinger{},
		Auth: svc, PlatformSetupToken: platformSetupToken,
	}))
	t.Cleanup(w.srv.Close)
	return w
}

func (w *platformWorld) do(t *testing.T, c *http.Client, method, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	return doAgainst(t, c, w.srv.URL, method, path, body, hdr)
}

// doAgainst issues a request against base, closing the body on cleanup.
func doAgainst(t *testing.T, c *http.Client, base, method, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" || method != "GET" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// createDewa bootstraps the first DEWA via /v1/platform/setup.
func createDewa(t *testing.T, w *platformWorld, email string) {
	t.Helper()
	r := w.do(t, &http.Client{}, "POST", "/v1/platform/setup",
		`{"token":"`+platformSetupToken+`","email":"`+email+`","password":"`+itPW+`"}`, nil)
	if r.StatusCode != 201 {
		t.Fatalf("create dewa %s: %d", email, r.StatusCode)
	}
}

// readCookie extracts the named cookie value from a response's Set-Cookie headers.
func readCookie(r *http.Response, name string) string {
	for _, sc := range r.Header["Set-Cookie"] {
		for _, part := range strings.Split(sc, ";") {
			if strings.HasPrefix(part, name+"=") {
				return strings.TrimSpace(strings.TrimPrefix(part, name+"="))
			}
		}
	}
	return ""
}

// jsonStr compacts a decoded body for substring assertions.
func jsonStr(m any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func TestPlatformSetupFlow(t *testing.T) {
	w := newPlatformWorld(t)

	// Not done yet: setup is enabled.
	if b := decode(t, w.do(t, &http.Client{}, "GET", "/v1/platform/setup-status", "", nil)); b["enabled"] != true || b["done"] != false {
		t.Fatalf("setup-status: %v", b)
	}
	// Wrong token is rejected without hint.
	if r := w.do(t, &http.Client{}, "POST", "/v1/platform/setup",
		`{"token":"wrong","email":"dewa@erapay.test","password":"`+itPW+`"}`, nil); r.StatusCode != 401 {
		t.Fatalf("wrong token: %d", r.StatusCode)
	}
	// Correct token creates the first DEWA.
	createDewa(t, w, "dewa@erapay.test")
	// Now done; a second setup is refused.
	if r := w.do(t, &http.Client{}, "POST", "/v1/platform/setup",
		`{"token":"`+platformSetupToken+`","email":"dewa2@erapay.test","password":"`+itPW+`"}`, nil); r.StatusCode != 409 {
		t.Fatalf("second setup: %d", r.StatusCode)
	}
	if b := decode(t, w.do(t, &http.Client{}, "GET", "/v1/platform/setup-status", "", nil)); b["done"] != true {
		t.Fatalf("done flag not reflected: %v", b)
	}
}

func TestPlatformPortalAndSeparation(t *testing.T) {
	w := newPlatformWorld(t)
	createDewa(t, w, "dewa@erapay.test")

	// DEWA login sets the platform cookie.
	lr := w.do(t, &http.Client{}, "POST", "/v1/platform/auth/login",
		`{"email":"dewa@erapay.test","password":"`+itPW+`"}`, nil)
	if lr.StatusCode != 200 {
		t.Fatalf("dewa login: %d", lr.StatusCode)
	}
	tok := readCookie(lr, platformCookie)
	if tok == "" {
		t.Fatal("no platform cookie from login")
	}
	authHdr := map[string]string{"Cookie": platformCookie + "=" + tok}

	// Principal + RBAC endpoints work with the platform cookie (requires rbac:manage).
	me := decode(t, w.do(t, &http.Client{}, "GET", "/v1/platform/auth/me", "", authHdr))
	if !strings.Contains(jsonStr(me), "rbac:manage") {
		t.Fatalf("me missing rbac:manage: %s", jsonStr(me))
	}
	if r := w.do(t, &http.Client{}, "GET", "/v1/platform/admin/roles", "", authHdr); r.StatusCode != 200 {
		t.Fatalf("platform admin/roles: %d", r.StatusCode)
	}

	// The dashboard-mounted RBAC route is gone (moved to the panel): with a real
	// dashboard session the path must 404 (the old route required rbac:manage).
	dl := w.do(t, &http.Client{}, "POST", "/v1/auth/login",
		`{"email":"dewa@erapay.test","password":"`+itPW+`"}`, nil)
	dTok := readCookie(dl, sessionCookie)
	if dTok == "" {
		t.Fatalf("no dashboard session from /v1/auth/login (%d)", dl.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "GET", "/v1/admin/roles", "", map[string]string{"Cookie": sessionCookie + "=" + dTok}); r.StatusCode != 404 {
		t.Fatalf("old /v1/admin/roles should be gone: %d", r.StatusCode)
	}

	// Without the platform cookie the panel rejects the request.
	if r := w.do(t, &http.Client{}, "GET", "/v1/platform/admin/roles", "", nil); r.StatusCode != 401 {
		t.Fatalf("panel without platform cookie: %d", r.StatusCode)
	}

	// A platform user who is NOT DEWA (super_admin) cannot log into the panel:
	// same 401 as a stranger, so the API does not leak who is DEWA.
	svc := auth.NewService(w.db, auth.DefaultConfig())
	sid, err := svc.CreateUser(t.Context(), "super@erapay.test", itPW)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AddPlatformRole(t, w.db, sid, "super_admin")
	rej := w.do(t, &http.Client{}, "POST", "/v1/platform/auth/login",
		`{"email":"super@erapay.test","password":"`+itPW+`"}`, nil)
	if rej.StatusCode != 401 {
		t.Fatalf("non-dewa platform user accepted: %d", rej.StatusCode)
	}
	if readCookie(rej, platformCookie) != "" {
		t.Fatal("non-dewa got a platform cookie")
	}
}

// A wrong password or an unknown email is a 401 the panel can show ("Email atau kata sandi salah"),
// never a 500: the panel reads any non-401 failure as "service unavailable".
func TestPlatformLoginWrongCredentialsIs401(t *testing.T) {
	w := newPlatformWorld(t)
	createDewa(t, w, "dewa@erapay.test")
	for name, body := range map[string]string{
		"wrong password": `{"email":"dewa@erapay.test","password":"bukan-password-yang-benar"}`,
		"unknown email":  `{"email":"tidak-ada@erapay.test","password":"` + itPW + `"}`,
	} {
		r := w.do(t, &http.Client{}, "POST", "/v1/platform/auth/login", body, nil)
		if r.StatusCode != 401 {
			t.Errorf("%s: status %d, want 401", name, r.StatusCode)
		}
		if readCookie(r, platformCookie) != "" {
			t.Errorf("%s: got a platform cookie", name)
		}
	}
}

// The panel tells the user how many tries are left and how long a lockout lasts, and a locked
// email gets a shrinking "try again in" instead of a bare refusal.
func TestPlatformLoginReportsAttemptsLeftAndLockoutCountdown(t *testing.T) {
	w := newPlatformWorld(t)
	createDewa(t, w, "dewa@erapay.test")
	bad := `{"email":"dewa@erapay.test","password":"bukan-password-yang-benar"}`

	first := w.do(t, &http.Client{}, "POST", "/v1/platform/auth/login", bad, nil)
	if first.StatusCode != 401 {
		t.Fatalf("first failure: %d", first.StatusCode)
	}
	if got := first.Header.Get("X-Attempts-Left"); got != "4" {
		t.Errorf("X-Attempts-Left after 1st failure = %q, want 4", got)
	}
	if got := first.Header.Get("X-Lockout-Seconds"); got != "900" {
		t.Errorf("X-Lockout-Seconds = %q, want 900 (15 minutes)", got)
	}
	var last *http.Response
	for i := 2; i <= 5; i++ {
		last = w.do(t, &http.Client{}, "POST", "/v1/platform/auth/login", bad, nil)
		if last.StatusCode != 401 {
			t.Fatalf("failure %d: %d", i, last.StatusCode)
		}
	}
	if got := last.Header.Get("X-Attempts-Left"); got != "0" {
		t.Errorf("X-Attempts-Left after 5th failure = %q, want 0", got)
	}

	// Locked now, even for the right password.
	good := `{"email":"dewa@erapay.test","password":"` + itPW + `"}`
	one := w.do(t, &http.Client{}, "POST", "/v1/platform/auth/login", good, nil)
	if one.StatusCode != 429 {
		t.Fatalf("locked login: %d, want 429", one.StatusCode)
	}
	s1, err := strconv.Atoi(one.Header.Get("Retry-After"))
	if err != nil || s1 <= 0 || s1 > 900 {
		t.Fatalf("Retry-After = %q, want 1..900 seconds", one.Header.Get("Retry-After"))
	}
	time.Sleep(1100 * time.Millisecond)
	two := w.do(t, &http.Client{}, "POST", "/v1/platform/auth/login", good, nil)
	s2, err := strconv.Atoi(two.Header.Get("Retry-After"))
	if two.StatusCode != 429 || err != nil || s2 >= s1 {
		t.Fatalf("second click: status %d Retry-After %q, want less than %d", two.StatusCode, two.Header.Get("Retry-After"), s1)
	}
}

func TestPlatformUserDeleteAndStats(t *testing.T) {
	w := newPlatformWorld(t)
	createDewa(t, w, "dewa@erapay.test")
	lr := w.do(t, &http.Client{}, "POST", "/v1/platform/auth/login", `{"email":"dewa@erapay.test","password":"`+itPW+`"}`, nil)
	if lr.StatusCode != 200 {
		t.Fatalf("dewa login: %d", lr.StatusCode)
	}
	hdr := map[string]string{"Cookie": platformCookie + "=" + readCookie(lr, platformCookie)}

	// Statistik dashboard: hanya DEWA, dan angkanya terisi.
	if r := w.do(t, &http.Client{}, "GET", "/v1/platform/stats", "", nil); r.StatusCode != 401 {
		t.Fatalf("stats without cookie: %d", r.StatusCode)
	}
	st := decode(t, w.do(t, &http.Client{}, "GET", "/v1/platform/stats", "", hdr))
	if users, _ := st["users"].(map[string]any); users == nil || users["total"].(float64) < 1 {
		t.Fatalf("stats users: %v", st["users"])
	}
	if series, _ := st["audit"].(map[string]any)["series_24h"].([]any); len(series) != 24 {
		t.Fatalf("audit series: %v", st["audit"])
	}

	svc := auth.NewService(w.db, auth.DefaultConfig())
	victim, err := svc.CreateUser(t.Context(), "victim@erapay.test", itPW)
	if err != nil {
		t.Fatal(err)
	}
	idOf := func(email string) string {
		list, _ := decode(t, w.do(t, &http.Client{}, "GET", "/v1/platform/admin/users", "", hdr))["data"].([]any)
		for _, u := range list {
			if m := u.(map[string]any); m["email"] == email {
				return m["id"].(string)
			}
		}
		return ""
	}
	self := idOf("dewa@erapay.test")
	if self == "" || idOf("victim@erapay.test") != victim {
		t.Fatal("users list missing rows")
	}

	if r := w.do(t, &http.Client{}, "DELETE", "/v1/platform/admin/users/"+victim, "", nil); r.StatusCode != 401 {
		t.Fatalf("delete without cookie: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "DELETE", "/v1/platform/admin/users/not-a-uuid", "", hdr); r.StatusCode != 404 {
		t.Fatalf("delete bad id: %d", r.StatusCode)
	}
	// Tidak boleh menghapus diri sendiri (juga pemegang DEWA terakhir).
	if r := w.do(t, &http.Client{}, "DELETE", "/v1/platform/admin/users/"+self, "", hdr); r.StatusCode != 409 || errCode(t, r) != "USER_PROTECTED" {
		t.Fatalf("self delete: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "DELETE", "/v1/platform/admin/users/"+victim, "", hdr); r.StatusCode != 200 {
		t.Fatalf("delete victim: %d", r.StatusCode)
	}
	if idOf("victim@erapay.test") != "" {
		t.Fatal("victim still listed")
	}
	if r := w.do(t, &http.Client{}, "DELETE", "/v1/platform/admin/users/"+victim, "", hdr); r.StatusCode != 404 {
		t.Fatalf("delete twice: %d", r.StatusCode)
	}
}
