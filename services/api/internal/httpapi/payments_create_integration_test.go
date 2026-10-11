package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

func post(t *testing.T, w *world, c *http.Client, body, key string, extra map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	h := map[string]string{}
	if key != "" {
		h["Idempotency-Key"] = key
	}
	for k, v := range extra {
		h[k] = v
	}
	r := w.do(t, c, "POST", "/v1/payments", body, h)
	return r, decode(t, r)
}

const goodBody = `{"merchant_reference":"ORD-1","amount":10000,"currency":"IDR","payment_method":"QRIS"}`

func keyClient(t *testing.T, w *world, scopes string) (*http.Client, map[string]string) {
	t.Helper()
	_, k := createKey(t, w, w.client(t, "owner@a.test"), `{"name":"pos","scopes":`+scopes+`}`)
	return &http.Client{}, bearer(k["secret"].(string))
}

func TestCreatePaymentHTTPHappyPath(t *testing.T) {
	w := setup(t)
	c, auth := keyClient(t, w, `["payment:create","payment:read"]`)

	r, p := post(t, w, c, goodBody, "idem-http-0001", auth)
	fee, _ := p["fee"].(map[string]any)
	qr, _ := p["qr_code"].(map[string]any)
	if r.StatusCode != 201 || p["status"] != "pending" || p["gateway"] != "gw1" || p["amount"] != float64(10000) ||
		fee["mdr_fee"] != float64(70) || fee["platform_fee"] != float64(100) || fee["merchant_net"] != float64(9830) || fee["bearer"] != "merchant" ||
		qr["type"] != "provider_defined" || !strings.HasPrefix(qr["value"].(string), "QR-") {
		t.Fatalf("%d %v", r.StatusCode, p)
	}
	for _, leak := range []string{"merchant_id", "gateway_reference", "qr_payload"} {
		if _, ok := p[leak]; ok {
			t.Fatalf("response leaks %s", leak)
		}
	}

	// Same key + same body: 200, same payment, one gateway call.
	r2, p2 := post(t, w, c, goodBody, "idem-http-0001", auth)
	if r2.StatusCode != 200 || p2["id"] != p["id"] || w.gw.calls.Load() != 1 {
		t.Fatalf("replay: %d %v calls=%d", r2.StatusCode, p2["id"], w.gw.calls.Load())
	}
	// Whitespace/key order differences are the same request.
	r3, p3 := post(t, w, c, `{ "payment_method":"QRIS","currency":"IDR", "amount":10000,"merchant_reference":"ORD-1" }`, "idem-http-0001", auth)
	if r3.StatusCode != 200 || p3["id"] != p["id"] {
		t.Fatalf("canonical replay: %d", r3.StatusCode)
	}
	// Same key, different amount: conflict.
	if r4, e := post(t, w, c, strings.Replace(goodBody, "10000", "20000", 1), "idem-http-0001", auth); r4.StatusCode != 409 ||
		e["error"].(map[string]any)["code"] != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("conflict: %d %v", r4.StatusCode, e)
	}
	// Readable afterwards through GET with the same key.
	if r5 := w.do(t, c, "GET", "/v1/payments/"+p["id"].(string), "", auth); r5.StatusCode != 200 {
		t.Fatalf("get: %d", r5.StatusCode)
	}
}

func TestCreatePaymentHTTPRejectsClientControlledFields(t *testing.T) {
	w := setup(t)
	c, auth := keyClient(t, w, `["payment:create"]`)
	for i, extra := range []string{
		`"merchant_net":99999`, `"platform_fee":0`, `"mdr_fee":0`, `"status":"succeeded"`,
		`"gateway":"other"`, `"merchant_id":"` + w.midB + `"`, `"fee":{"merchant_net":1}`,
	} {
		body := strings.TrimSuffix(goodBody, "}") + "," + extra + "}"
		if r, _ := post(t, w, c, body, "idem-inject-000"+strconv.Itoa(i), auth); r.StatusCode != 400 {
			t.Errorf("%s accepted: %d", extra, r.StatusCode)
		}
	}
	if w.gw.calls.Load() != 0 {
		t.Fatal("gateway reached with an invalid request")
	}
}

func TestCreatePaymentHTTPValidationAndAuth(t *testing.T) {
	w := setup(t)
	c, auth := keyClient(t, w, `["payment:create"]`)

	if r, _ := post(t, w, c, goodBody, "", auth); r.StatusCode != 400 {
		t.Errorf("missing Idempotency-Key: %d", r.StatusCode)
	}
	if r, _ := post(t, w, c, goodBody, "short", auth); r.StatusCode != 400 {
		t.Errorf("short key: %d", r.StatusCode)
	}
	for name, body := range map[string]string{
		"zero":     `{"merchant_reference":"R","amount":0,"currency":"IDR","payment_method":"QRIS"}`,
		"negative": `{"merchant_reference":"R","amount":-5,"currency":"IDR","payment_method":"QRIS"}`,
		"float":    `{"merchant_reference":"R","amount":100.5,"currency":"IDR","payment_method":"QRIS"}`,
		"string":   `{"merchant_reference":"R","amount":"100","currency":"IDR","payment_method":"QRIS"}`,
		"currency": `{"merchant_reference":"R","amount":100,"currency":"USD","payment_method":"QRIS"}`,
		"no ref":   `{"amount":100,"currency":"IDR","payment_method":"QRIS"}`,
		"not json": `nope`,
		"huge":     `{"merchant_reference":"R","amount":99999999999999999,"currency":"IDR","payment_method":"QRIS"}`,
	} {
		if r, _ := post(t, w, c, body, "idem-val-"+strings.ReplaceAll(name, " ", ""), auth); r.StatusCode != 400 {
			t.Errorf("%s: %d", name, r.StatusCode)
		}
	}

	if r, _ := post(t, w, &http.Client{}, goodBody, "idem-anon-0001", nil); r.StatusCode != 401 {
		t.Errorf("anon: %d", r.StatusCode)
	}
	// A read-only key cannot create.
	_, ro := keyClient(t, w, `["payment:read"]`)
	if r, _ := post(t, w, c, goodBody, "idem-readonly-1", ro); r.StatusCode != 403 {
		t.Errorf("read-only key: %d", r.StatusCode)
	}
	if w.gw.calls.Load() != 0 {
		t.Fatal("gateway reached on invalid/unauthorized requests")
	}
}

func TestCreatePaymentHTTPMethodUnavailableForMerchantB(t *testing.T) {
	w := setup(t) // merchant B has no active gateway account
	b := w.client(t, "owner@b.test")
	_, k := createKey(t, w, b, `{"name":"pos-b","scopes":["payment:create"]}`)
	r, e := post(t, w, &http.Client{}, goodBody, "idem-b-000001", bearer(k["secret"].(string)))
	if r.StatusCode != 422 || e["error"].(map[string]any)["code"] != "PAYMENT_METHOD_UNAVAILABLE" || w.gw.calls.Load() != 0 {
		t.Fatalf("%d %v calls=%d", r.StatusCode, e, w.gw.calls.Load())
	}
}

func TestCreatePaymentHTTPGatewayOutcomes(t *testing.T) {
	w := setup(t)
	c, auth := keyClient(t, w, `["payment:create","payment:read"]`)

	w.gw.create = func(gateway.CreateRequest) (gateway.CreateResult, error) {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Code: "4004700", HTTPStatus: 400}
	}
	r, p := post(t, w, c, goodBody, "idem-reject-001", auth)
	if r.StatusCode != 422 || p["error"].(map[string]any)["code"] != "GATEWAY_REJECTED" || p["payment"].(map[string]any)["status"] != "failed" {
		t.Fatalf("reject: %d %v", r.StatusCode, p)
	}

	w.gw.create = func(gateway.CreateRequest) (gateway.CreateResult, error) {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: 504}
	}
	before := w.gw.calls.Load()
	r, p = post(t, w, c, goodBody, "idem-timeout-01", auth)
	if r.StatusCode != 202 || p["status"] != "unknown" {
		t.Fatalf("timeout: %d %v", r.StatusCode, p)
	}
	if _, ok := p["qr_code"]; ok {
		t.Fatal("QR exposed for an unknown payment")
	}
	// The client retries the same request: still 202, no second gateway call.
	r, p2 := post(t, w, c, goodBody, "idem-timeout-01", auth)
	if r.StatusCode != 202 || p2["id"] != p["id"] || w.gw.calls.Load() != before+1 {
		t.Fatalf("retry after timeout: %d calls=%d", r.StatusCode, w.gw.calls.Load()-before)
	}
}

func TestCreatePaymentHTTPConcurrentSameKey(t *testing.T) {
	w := setup(t)
	c, auth := keyClient(t, w, `["payment:create"]`)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ids := map[string]bool{}
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", w.srv.URL+"/v1/payments", strings.NewReader(goodBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "idem-concurrent-1")
			req.Header.Set("Authorization", auth["Authorization"])
			resp, err := c.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			m := decode(t, resp)
			switch resp.StatusCode {
			case 200, 201:
				mu.Lock()
				ids[m["id"].(string)] = true
				mu.Unlock()
			case 409: // in progress
			default:
				t.Errorf("status %d %v", resp.StatusCode, m)
			}
		}()
	}
	wg.Wait()
	if len(ids) != 1 || w.gw.calls.Load() != 1 {
		t.Fatalf("payments=%d gatewayCalls=%d", len(ids), w.gw.calls.Load())
	}
}

// A session user without payment:create (integrator has it; cashier too) vs platform session: tenant required.
func TestCreatePaymentHTTPPlatformSessionForbidden(t *testing.T) {
	w := setup(t)
	root := w.client(t, "root@erapay.test")
	if r, _ := post(t, w, root, goodBody, "idem-root-00001", nil); r.StatusCode != 403 {
		t.Fatalf("platform session created a payment: %d", r.StatusCode)
	}
}
