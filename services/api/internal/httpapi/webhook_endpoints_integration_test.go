package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
)

func testBox(t *testing.T) *secretbox.Box {
	t.Helper()
	b, err := secretbox.New(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A public IP literal avoids DNS in tests while still passing the SSRF checks.
const okHook = `{"url":"https://93.184.216.34/erapay/hook"}`

func TestWebhookEndpointsHTTPLifecycle(t *testing.T) {
	w := setup(t)
	owner := w.client(t, "owner@a.test")

	r := w.do(t, owner, "POST", "/v1/webhook-endpoints", okHook, nil)
	ep := decode(t, r)
	secret, _ := ep["secret"].(string)
	if r.StatusCode != 201 || !strings.HasPrefix(secret, "whsec_") || r.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("create: %d %v", r.StatusCode, ep)
	}
	id := ep["id"].(string)

	// The secret is shown once: list never returns it.
	lr := w.do(t, owner, "GET", "/v1/webhook-endpoints", "", nil)
	raw, _ := json.Marshal(decode(t, lr))
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), `"secret"`) || !strings.Contains(string(raw), id) {
		t.Fatalf("list: %s", raw)
	}

	rot := decode(t, w.do(t, owner, "POST", "/v1/webhook-endpoints/"+id+"/rotate-secret", `{}`, nil))
	if rot["secret"] == nil || rot["secret"] == secret {
		t.Fatalf("rotate: %v", rot)
	}

	if r := w.do(t, owner, "GET", "/v1/webhook-deliveries?status=pending&limit=10", "", nil); r.StatusCode != 200 {
		t.Fatalf("deliveries: %d", r.StatusCode)
	}
	for _, q := range []string{"status=bogus", "limit=0", "limit=1000", "cursor=yesterday", "endpoint_id=not-a-uuid"} {
		if r := w.do(t, owner, "GET", "/v1/webhook-deliveries?"+q, "", nil); r.StatusCode != 400 {
			t.Errorf("%s: %d", q, r.StatusCode)
		}
	}

	if r := w.do(t, owner, "POST", "/v1/webhook-endpoints/"+id+"/disable", `{}`, nil); r.StatusCode != 204 {
		t.Fatalf("disable: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", "/v1/webhook-endpoints/"+id+"/disable", `{}`, nil); r.StatusCode != 404 {
		t.Fatalf("disable twice: %d", r.StatusCode)
	}
}

func TestWebhookEndpointsRejectUnsafeURLs(t *testing.T) {
	w := setup(t)
	owner := w.client(t, "owner@a.test")
	for _, u := range []string{
		"http://93.184.216.34/x", "https://127.0.0.1/x", "https://localhost/x", "https://10.0.0.1/x",
		"https://169.254.169.254/latest/meta-data", "https://[::1]/x", "https://user:pw@93.184.216.34/x", "ftp://93.184.216.34/x", "javascript:alert(1)",
	} {
		r := w.do(t, owner, "POST", "/v1/webhook-endpoints", `{"url":"`+u+`"}`, nil)
		if r.StatusCode != 400 {
			t.Errorf("%s: %d", u, r.StatusCode)
		}
	}
	if r := w.do(t, owner, "POST", "/v1/webhook-endpoints", `{}`, nil); r.StatusCode != 400 {
		t.Errorf("empty body: %d", r.StatusCode)
	}
}

func TestWebhookEndpointsPermissionsAndTenantIsolation(t *testing.T) {
	w := setup(t)
	owner, ownerB := w.client(t, "owner@a.test"), w.client(t, "owner@b.test")
	id := decode(t, w.do(t, owner, "POST", "/v1/webhook-endpoints", okHook, nil))["id"].(string)

	// Cashier lacks webhook:manage; unauthenticated is 401; platform session has no merchant.
	if r := w.do(t, w.client(t, "cashier@a.test"), "GET", "/v1/webhook-endpoints", "", nil); r.StatusCode != 403 {
		t.Errorf("cashier list: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "GET", "/v1/webhook-endpoints", "", nil); r.StatusCode != 401 {
		t.Errorf("anon list: %d", r.StatusCode)
	}
	if r := w.do(t, w.client(t, "integrator@a.test"), "GET", "/v1/webhook-endpoints", "", nil); r.StatusCode != 200 {
		t.Errorf("integrator list: %d", r.StatusCode)
	}

	// Merchant B sees nothing of A and cannot touch it (indistinguishable from missing).
	if raw, _ := json.Marshal(decode(t, w.do(t, ownerB, "GET", "/v1/webhook-endpoints", "", nil))); strings.Contains(string(raw), id) {
		t.Fatalf("B sees A's endpoint: %s", raw)
	}
	for _, p := range []string{"/v1/webhook-endpoints/" + id + "/rotate-secret", "/v1/webhook-endpoints/" + id + "/disable",
		"/v1/webhook-deliveries/" + id + "/redeliver", "/v1/webhook-endpoints/not-a-uuid/disable"} {
		if r := w.do(t, ownerB, "POST", p, `{}`, nil); r.StatusCode != 404 {
			t.Errorf("B POST %s: %d", p, r.StatusCode)
		}
	}
	// API keys never reach this management API (session-only).
	_, k := createKey(t, w, owner, `{"name":"k","scopes":["payment:read"]}`)
	if r := w.do(t, &http.Client{}, "GET", "/v1/webhook-endpoints", "", bearer(k["secret"].(string))); r.StatusCode != 401 {
		t.Errorf("api key on management endpoint: %d", r.StatusCode)
	}
}
