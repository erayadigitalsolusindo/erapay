package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func createKey(t *testing.T, w *world, c *http.Client, body string) (int, map[string]any) {
	t.Helper()
	r := w.do(t, c, "POST", "/v1/api-keys", body, nil)
	return r.StatusCode, decode(t, r)
}

func bearer(k string) map[string]string { return map[string]string{"Authorization": "Bearer " + k} }

func TestAPIKeyLifecycleHTTP(t *testing.T) {
	w := setup(t)
	owner := w.client(t, "owner@a.test")
	anon := &http.Client{}

	code, k := createKey(t, w, owner, `{"name":"POS kasir 1","scopes":["payment:create","payment:read"]}`)
	if code != 201 {
		t.Fatalf("create: %d %v", code, k)
	}
	secret := k["secret"].(string)
	id := k["id"].(string)
	if !strings.HasPrefix(secret, "epk_sbx_") {
		t.Fatalf("secret format: %s", secret)
	}

	// The key authenticates and resolves to merchant A only.
	r := w.do(t, anon, "GET", "/v1/api-keys/self", "", bearer(secret))
	self := decode(t, r)
	if r.StatusCode != 200 || self["merchant_id"] != w.midA || self["environment"] != "sandbox" {
		t.Fatalf("self: %d %v", r.StatusCode, self)
	}

	// The secret is never exposed again.
	lr := w.do(t, owner, "GET", "/v1/api-keys", "", nil)
	raw, _ := json.Marshal(decode(t, lr))
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "secret") || !strings.Contains(string(raw), id) {
		t.Fatalf("list leaks or misses: %s", raw)
	}

	// Rotate with grace 0: old dies, new works.
	rr := w.do(t, owner, "POST", "/v1/api-keys/"+id+"/rotate", `{}`, nil)
	nk := decode(t, rr)
	if rr.StatusCode != 201 || nk["secret"] == secret {
		t.Fatalf("rotate: %d %v", rr.StatusCode, nk)
	}
	if r := w.do(t, anon, "GET", "/v1/api-keys/self", "", bearer(secret)); r.StatusCode != 401 {
		t.Fatalf("old key after rotate: %d", r.StatusCode)
	}
	if r := w.do(t, anon, "GET", "/v1/api-keys/self", "", bearer(nk["secret"].(string))); r.StatusCode != 200 {
		t.Fatalf("new key: %d", r.StatusCode)
	}

	// Revoke -> 401 immediately.
	if r := w.do(t, owner, "POST", "/v1/api-keys/"+nk["id"].(string)+"/revoke", `{}`, nil); r.StatusCode != 204 {
		t.Fatalf("revoke: %d", r.StatusCode)
	}
	if r := w.do(t, anon, "GET", "/v1/api-keys/self", "", bearer(nk["secret"].(string))); r.StatusCode != 401 {
		t.Fatalf("revoked key: %d", r.StatusCode)
	}

	// Missing / malformed credentials.
	for _, h := range []map[string]string{nil, {"Authorization": "Bearer nope"}, {"Authorization": "Basic abc"}} {
		if r := w.do(t, anon, "GET", "/v1/api-keys/self", "", h); r.StatusCode != 401 {
			t.Fatalf("headers %v: %d", h, r.StatusCode)
		}
	}
}

func TestAPIKeyPermissionsAndScopeEscalation(t *testing.T) {
	w := setup(t)

	// Only roles with api_key:manage may manage keys.
	for _, who := range []string{"cashier@a.test", "integrator@a.test"} {
		c := w.client(t, who)
		if code, _ := createKey(t, w, c, `{"name":"x","scopes":["payment:read"]}`); code != 403 {
			t.Fatalf("%s created key: %d", who, code)
		}
		if r := w.do(t, c, "GET", "/v1/api-keys", "", nil); r.StatusCode != 403 {
			t.Fatalf("%s listed keys: %d", who, r.StatusCode)
		}
	}
	if code, _ := createKey(t, w, &http.Client{}, `{"name":"x","scopes":["payment:read"]}`); code != 401 {
		t.Fatalf("anon created key: %d", code)
	}

	// A key can never carry management / admin scopes, nor scopes the creator lacks.
	owner := w.client(t, "owner@a.test")
	for _, sc := range []string{`["api_key:manage"]`, `["user:manage"]`, `["gateway:manage"]`, `["merchant:write"]`, `[]`, `["nonsense"]`} {
		if code, _ := createKey(t, w, owner, `{"name":"x","scopes":`+sc+`}`); code != 400 {
			t.Fatalf("scopes %s accepted: %d", sc, code)
		}
	}
	if code, _ := createKey(t, w, owner, `{"name":"","scopes":["payment:read"]}`); code != 400 {
		t.Fatalf("empty name accepted: %d", code)
	}

	// A key cannot be used as a session/management credential.
	_, k := createKey(t, w, owner, `{"name":"pos","scopes":["payment:read"]}`)
	if r := w.do(t, &http.Client{}, "GET", "/v1/api-keys", "", bearer(k["secret"].(string))); r.StatusCode != 401 {
		t.Fatalf("api key used on management route: %d", r.StatusCode)
	}
}

func TestAPIKeyTenantIsolationHTTP(t *testing.T) {
	w := setup(t)
	a := w.client(t, "owner@a.test")
	b := w.client(t, "owner@b.test")

	_, ka := createKey(t, w, a, `{"name":"A key","scopes":["payment:read"]}`)
	_, kb := createKey(t, w, b, `{"name":"B key","scopes":["payment:read"]}`)
	idA, idB := ka["id"].(string), kb["id"].(string)

	// B can neither see, revoke nor rotate A's key (404, not 403: existence not revealed).
	raw, _ := json.Marshal(decode(t, w.do(t, b, "GET", "/v1/api-keys", "", nil)))
	if strings.Contains(string(raw), idA) || !strings.Contains(string(raw), idB) {
		t.Fatalf("B's key list: %s", raw)
	}
	for _, act := range []string{"revoke", "rotate"} {
		if r := w.do(t, b, "POST", "/v1/api-keys/"+idA+"/"+act, `{}`, nil); r.StatusCode != 404 {
			t.Fatalf("B %s A's key: %d", act, r.StatusCode)
		}
	}
	// A's key still works and is bound to A, regardless of spoofed headers.
	r := w.do(t, &http.Client{}, "GET", "/v1/api-keys/self?merchant_id="+w.midB, "",
		map[string]string{"Authorization": "Bearer " + ka["secret"].(string), "X-Merchant-ID": w.midB})
	if got := decode(t, r)["merchant_id"]; r.StatusCode != 200 || got != w.midA {
		t.Fatalf("A's key resolved to %v (%d)", got, r.StatusCode)
	}
}
