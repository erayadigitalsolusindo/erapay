package httpapi

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"testing"
)

func regBody(email string) string {
	return `{"account_type":"business","name":"Budi Santoso","business_name":"Toko Budi","email":"` + email +
		`","phone":"0812 3456 7890","password":"abc12345xyz9","accept_terms":true}`
}

func errCode(t *testing.T, r *http.Response) string {
	t.Helper()
	e, _ := decode(t, r)["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func register(t *testing.T, w *world, email string) string {
	t.Helper()
	r := w.do(t, &http.Client{}, "POST", "/v1/auth/register", regBody(email), nil)
	m := decode(t, r)
	if r.StatusCode != 202 || m["status"] != "check_email" || len(m) != 1 {
		t.Fatalf("register %s: %d %v", email, r.StatusCode, m)
	}
	var mid string
	if err := w.db.QueryRow(t.Context(), `SELECT mu.merchant_id::text FROM merchant_users mu JOIN users u ON u.id = mu.user_id
		WHERE lower(u.email) = lower($1)`, email).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	return mid
}

// mailLink returns the token of the newest email to `to` whose link contains `path`.
func mailLink(t *testing.T, w *world, to, path string) string {
	t.Helper()
	sent := w.sent()
	for i := len(sent) - 1; i >= 0; i-- {
		if strings.EqualFold(sent[i].To, to) {
			if _, after, ok := strings.Cut(sent[i].Body, path+"?token="); ok {
				return strings.Fields(after)[0]
			}
		}
	}
	t.Fatalf("no %s email to %s in %d messages", path, to, len(sent))
	return ""
}

func verifyOwner(t *testing.T, w *world, email string) {
	t.Helper()
	r := w.do(t, &http.Client{}, "POST", "/v1/auth/verify-email", `{"token":"`+mailLink(t, w, email, "/verify-email")+`"}`, nil)
	if r.StatusCode != 200 {
		t.Fatalf("verify %s: %d", email, r.StatusCode)
	}
}

const signupPW = "abc12345xyz9"

// signedUp logs in with the password chosen at sign-up (w.client uses the seeded users' password).
func signedUp(t *testing.T, w *world, email string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if r := w.do(t, c, "POST", "/v1/auth/login", `{"email":"`+email+`","password":"`+signupPW+`"}`, nil); r.StatusCode != 200 {
		t.Fatalf("login %s: %d", email, r.StatusCode)
	}
	return c
}

func planCount(t *testing.T, w *world, mid string) int {
	t.Helper()
	var n int
	if err := w.db.QueryRow(t.Context(), `SELECT count(*) FROM merchant_pricing_assignments a JOIN pricing_plans p ON p.id = a.plan_id
		WHERE a.merchant_id = $1 AND p.code = 'qris-'||$1::text`, mid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Sign-up to approval, end to end: a submitted merchant can log in but cannot take payments;
// only super_admin can approve, and only with explicit fees.
func TestSignupReviewApproveHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "Budi@Toko.test") // email is normalized to lower case

	// Signing up never logs in, and the new owner is not yet approved.
	owner := signedUp(t, w, "budi@toko.test")
	prof := decode(t, w.do(t, owner, "GET", "/v1/merchant/profile", "", nil))
	if prof["id"] != mid || prof["onboarding_status"] != "submitted" {
		t.Fatalf("profile: %v", prof)
	}
	attempts := 0
	pay := func() (int, string) {
		attempts++ // a fresh key each time: a replayed key would just return the stored first answer
		r := w.do(t, owner, "POST", "/v1/payments", goodBody, map[string]string{"Idempotency-Key": fmt.Sprintf("idem-signup-%04d", attempts)})
		return r.StatusCode, errCode(t, r)
	}
	// Ops has already wired pricing and an active gateway account (as for the seeded merchants), so the
	// ONLY thing standing between this merchant and a payment is the onboarding status.
	if _, err := w.db.Exec(t.Context(), `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
		SELECT $1, id, now() - interval '1 day' FROM pricing_plans WHERE code = 'std'`, mid); err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.Exec(t.Context(), `INSERT INTO merchant_gateway_accounts (merchant_id, gateway, environment, gateway_ref, activation_status)
		VALUES ($1,'gw1','sandbox','MID-NEW','active')`, mid); err != nil {
		t.Fatal(err)
	}
	if s, c := pay(); s != 403 || c != "MERCHANT_NOT_ACTIVE" || w.gw.calls.Load() != 0 {
		t.Fatalf("payment before approval: %d %s gateway calls=%d", s, c, w.gw.calls.Load())
	}

	// Review queue: finance and super_admin see it; the applicant's owner details are present.
	fin := w.client(t, "finance@erapay.test")
	root := w.client(t, "root@erapay.test")
	list := decode(t, w.do(t, fin, "GET", "/v1/admin/merchants", "", nil))["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["id"] != mid || list[0].(map[string]any)["owner_email"] != "budi@toko.test" ||
		list[0].(map[string]any)["contact_phone"] != "+6281234567890" || list[0].(map[string]any)["legal_name"] != "Toko Budi" {
		t.Fatalf("queue: %v", list)
	}

	// Finance may review but not approve (approval sets fees).
	if r := w.do(t, fin, "POST", "/v1/admin/merchants/"+mid+"/approve", `{"mdr_bps":70,"platform_bps":100}`, nil); r.StatusCode != 403 {
		t.Fatalf("finance approve: %d", r.StatusCode)
	}
	// The owner's address is unproven, so approval is refused whatever the fees are.
	if r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", `{"mdr_bps":70,"platform_bps":100}`, nil); r.StatusCode != 409 || errCode(t, r) != "EMAIL_NOT_VERIFIED" {
		t.Fatalf("approve before verification: %d", r.StatusCode)
	}
	if list[0].(map[string]any)["owner_email_verified"] != false {
		t.Fatalf("queue should flag the unverified owner: %v", list[0])
	}
	verifyOwner(t, w, "budi@toko.test")
	// Fees have no default: missing or out-of-range values are refused and nothing changes.
	for _, b := range []string{`{}`, `{"mdr_bps":70}`, `{"mdr_bps":-1,"platform_bps":100}`, `{"mdr_bps":70,"platform_bps":10001}`} {
		if r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", b, nil); r.StatusCode != 400 {
			t.Fatalf("approve %s: %d", b, r.StatusCode)
		}
	}
	if planCount(t, w, mid) != 0 {
		t.Fatal("a refused approval left a pricing plan behind")
	}

	r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", `{"mdr_bps":70,"platform_bps":100}`, nil)
	if got := decode(t, r); r.StatusCode != 200 || got["status"] != "approved" {
		t.Fatalf("approve: %d %v", r.StatusCode, got)
	}
	var mdr, plat int
	if err := w.db.QueryRow(t.Context(), `SELECT p.mdr_bps, p.platform_bps FROM merchant_pricing_assignments a
		JOIN pricing_plans p ON p.id = a.plan_id WHERE a.merchant_id = $1`, mid).Scan(&mdr, &plat); err != nil || mdr != 70 || plat != 100 {
		t.Fatalf("pricing: %d %d %v", mdr, plat, err)
	}
	var audits int
	if err := w.db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action IN ('merchant.registered','merchant.approved')`, mid).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("audit rows: %d", audits)
	}

	// The onboarding gate is open now: the refusal is no longer MERCHANT_NOT_ACTIVE.
	if s, c := pay(); s == 403 || c == "MERCHANT_NOT_ACTIVE" {
		t.Fatalf("payment after approval still gated: %d %s", s, c)
	}
	// Approving again is refused and must not add a second plan.
	if r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", `{"mdr_bps":1,"platform_bps":1}`, nil); r.StatusCode != 409 {
		t.Fatalf("second approve: %d", r.StatusCode)
	}
	if planCount(t, w, mid) != 1 {
		t.Fatalf("plans after repeat: %d", planCount(t, w, mid))
	}
}

func TestRejectMerchantHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "ditolak@toko.test")
	root := w.client(t, "root@erapay.test")
	if r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/reject", `{"note":" "}`, nil); r.StatusCode != 400 {
		t.Fatalf("reject without note: %d", r.StatusCode)
	}
	r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/reject", `{"note":"NPWP tidak sesuai"}`, nil)
	if got := decode(t, r); r.StatusCode != 200 || got["status"] != "rejected" || got["review_note"] != "NPWP tidak sesuai" {
		t.Fatalf("reject: %d %v", r.StatusCode, got)
	}
	// Terminal: it can be neither approved nor rejected again, and no plan was ever assigned.
	if r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", `{"mdr_bps":70,"platform_bps":100}`, nil); r.StatusCode != 409 {
		t.Fatalf("approve after reject: %d", r.StatusCode)
	}
	if planCount(t, w, mid) != 0 {
		t.Fatal("rejected merchant has pricing")
	}
	// Seeded merchants are already approved, so they are not reviewable.
	if r := w.do(t, root, "POST", "/v1/admin/merchants/"+w.midA+"/reject", `{"note":"coba tolak"}`, nil); r.StatusCode != 409 {
		t.Fatalf("reject approved merchant: %d", r.StatusCode)
	}
}

// Many admins clicking at once, or a double submit: exactly one decision wins, one plan exists.
func TestApproveConcurrentHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "balapan@toko.test")
	root := w.client(t, "root@erapay.test")
	const n = 8
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path, body := "/approve", `{"mdr_bps":70,"platform_bps":100}`
			if i%4 == 3 { // a rejection racing the approvals
				path, body = "/reject", `{"note":"ditolak bersamaan"}`
			}
			codes[i] = w.do(t, root, "POST", "/v1/admin/merchants/"+mid+path, body, nil).StatusCode
		}()
	}
	wg.Wait()
	ok := 0
	for _, c := range codes {
		switch c {
		case 200:
			ok++
		case 409:
		default:
			t.Fatalf("unexpected status in %v", codes)
		}
	}
	var status string
	_ = w.db.QueryRow(t.Context(), `SELECT onboarding_status FROM merchants WHERE id = $1`, mid).Scan(&status)
	plans := planCount(t, w, mid)
	if ok != 1 || (status == "approved") != (plans == 1) || (status == "rejected") != (plans == 0) {
		t.Fatalf("winners=%d status=%s plans=%d codes=%v", ok, status, plans, codes)
	}
}

func TestRegisterValidationAndDuplicatesHTTP(t *testing.T) {
	w := setup(t)
	c := &http.Client{}
	bad := map[string]string{
		"11 chars":         `{"account_type":"individual","name":"Budi Santoso","email":"a@b.test","phone":"81234567890","password":"abcdefghijk","accept_terms":true}`,
		"short password":   `{"account_type":"individual","name":"Budi Santoso","email":"a@b.test","phone":"81234567890","password":"abc123","accept_terms":true}`,
		"bad email":        `{"account_type":"individual","name":"Budi Santoso","email":"bukan-email","phone":"81234567890","password":"abc123456789","accept_terms":true}`,
		"display name":     `{"account_type":"individual","name":"Budi Santoso","email":"Budi <a@b.test>","phone":"81234567890","password":"abc123456789","accept_terms":true}`,
		"bad phone":        `{"account_type":"individual","name":"Budi Santoso","email":"a@b.test","phone":"12345","password":"abc123456789","accept_terms":true}`,
		"terms":            `{"account_type":"individual","name":"Budi Santoso","email":"a@b.test","phone":"81234567890","password":"abc123456789","accept_terms":false}`,
		"business no name": `{"account_type":"business","name":"Budi Santoso","email":"a@b.test","phone":"81234567890","password":"abc123456789","accept_terms":true}`,
		"bad type":         `{"account_type":"admin","name":"Budi Santoso","email":"a@b.test","phone":"81234567890","password":"abc123456789","accept_terms":true}`,
		"unknown field":    `{"account_type":"individual","name":"Budi Santoso","email":"a@b.test","phone":"81234567890","password":"abc123456789","accept_terms":true,"merchant_id":"` + w.midA + `","status":"approved"}`,
	}
	for name, b := range bad {
		if r := w.do(t, c, "POST", "/v1/auth/register", b, nil); r.StatusCode != 400 {
			t.Fatalf("%s: %d", name, r.StatusCode)
		}
	}
	var users int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE email = 'a@b.test'`).Scan(&users)
	if users != 0 {
		t.Fatal("rejected input created a user")
	}

	// Individual: the owner's name doubles as the merchant name.
	r := w.do(t, c, "POST", "/v1/auth/register",
		`{"account_type":"individual","name":"Siti Aminah","email":"siti@toko.test","phone":"+62 812-3456-7890","password":"abc123456789","accept_terms":true}`, nil)
	if r.StatusCode != 202 {
		t.Fatalf("individual: %d", r.StatusCode)
	}
	// The email is taken (case-insensitive), the original account is untouched, and nothing orphaned.
	before := len(w.sent())
	r2 := w.do(t, c, "POST", "/v1/auth/register", regBody("SITI@toko.test"), nil)
	body := decode(t, r2)
	if r2.StatusCode != 202 || body["status"] != "check_email" || len(body) != 1 {
		t.Fatalf("duplicate must look exactly like a new sign-up: %d %v", r2.StatusCode, body)
	}
	// Only the real owner hears about it, and it carries no verification link.
	sent := w.sent()
	if len(sent) != before+1 || !strings.EqualFold(sent[before].To, "siti@toko.test") || strings.Contains(sent[before].Body, "token=") {
		t.Fatalf("duplicate notice: %v", sent[before:])
	}
	var merchants, owners int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM merchants WHERE legal_name = 'Toko Budi'`).Scan(&merchants)
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM merchant_users`).Scan(&owners)
	if merchants != 0 || owners != 5 { // 4 seeded owners/staff + Siti
		t.Fatalf("orphans: merchants=%d members=%d", merchants, owners)
	}
}

func TestRegisterSameEmailConcurrentHTTP(t *testing.T) {
	w := setup(t)
	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = w.do(t, &http.Client{}, "POST", "/v1/auth/register", regBody("kembar@toko.test"), nil).StatusCode
		}()
	}
	wg.Wait()
	for _, c := range codes {
		if c != 202 { // every caller gets the same answer; only one account may exist
			t.Fatalf("codes: %v", codes)
		}
	}
	created := 1
	var users, merchants int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE email = 'kembar@toko.test'`).Scan(&users)
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM merchants WHERE legal_name = 'Toko Budi'`).Scan(&merchants)
	if created != 1 || users != 1 || merchants != 1 {
		t.Fatalf("created=%d users=%d merchants=%d codes=%v", created, users, merchants, codes)
	}
}

func TestRegisterRateLimitedPerIPHTTP(t *testing.T) {
	w := setup(t)
	c := &http.Client{}
	for i := range 5 {
		if r := w.do(t, c, "POST", "/v1/auth/register", regBody(fmt.Sprintf("u%d@toko.test", i)), nil); r.StatusCode != 202 {
			t.Fatalf("signup %d: %d", i, r.StatusCode)
		}
	}
	r := w.do(t, c, "POST", "/v1/auth/register", regBody("u6@toko.test"), nil)
	if r.StatusCode != 429 || r.Header.Get("Retry-After") == "" {
		t.Fatalf("6th: %d", r.StatusCode)
	}
	// An attacker cannot dodge the limit by spoofing X-Forwarded-For from an untrusted peer.
	r = w.do(t, c, "POST", "/v1/auth/register", regBody("u7@toko.test"), map[string]string{"X-Forwarded-For": "203.0.113.9"})
	if r.StatusCode != 429 {
		t.Fatalf("xff spoof: %d", r.StatusCode)
	}
}

// The review endpoints are cross-tenant. A merchant session (even an owner, who holds merchant:read)
// must never reach them, and neither may anonymous callers or the other platform roles for approval.
func clientFor(t *testing.T, w *world, email string) *http.Client {
	if email == "baru@toko.test" || email == "revisi@toko.test" {
		return signedUp(t, w, email)
	}
	return w.client(t, email)
}

func TestMerchantReviewAccessControlHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "baru@toko.test")
	paths := []struct{ method, path, body string }{
		{"GET", "/v1/admin/merchants", ""},
		{"GET", "/v1/admin/merchants/" + mid, ""},
		{"POST", "/v1/admin/merchants/" + mid + "/approve", `{"mdr_bps":70,"platform_bps":100}`},
		{"POST", "/v1/admin/merchants/" + mid + "/reject", `{"note":"tidak boleh"}`},
	}
	for _, p := range paths {
		if r := w.do(t, &http.Client{}, p.method, p.path, p.body, nil); r.StatusCode != 401 {
			t.Fatalf("anon %s: %d", p.path, r.StatusCode)
		}
		for _, who := range []string{"owner@a.test", "cashier@a.test", "baru@toko.test"} {
			if r := w.do(t, clientFor(t, w, who), p.method, p.path, p.body, nil); r.StatusCode != 403 {
				t.Fatalf("%s %s: %d", who, p.path, r.StatusCode)
			}
		}
	}
	var status string
	_ = w.db.QueryRow(t.Context(), `SELECT onboarding_status FROM merchants WHERE id = $1`, mid).Scan(&status)
	if status != "submitted" || planCount(t, w, mid) != 0 {
		t.Fatalf("an unauthorized call changed state: %s", status)
	}
	root := w.client(t, "root@erapay.test")
	if r := w.do(t, root, "GET", "/v1/admin/merchants/not-a-uuid", "", nil); r.StatusCode != 404 {
		t.Fatalf("bad id: %d", r.StatusCode)
	}
	if r := w.do(t, root, "GET", "/v1/admin/merchants?status=bogus", "", nil); r.StatusCode != 400 {
		t.Fatalf("bad status: %d", r.StatusCode)
	}
	if r := w.do(t, root, "GET", "/v1/admin/merchants/00000000-0000-0000-0000-000000000000", "", nil); r.StatusCode != 404 {
		t.Fatalf("unknown id: %d", r.StatusCode)
	}
}
