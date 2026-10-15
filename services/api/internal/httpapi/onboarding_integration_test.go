package httpapi

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"sort"
	"strings"
	"sync"
	"testing"
)

func identityJSON(business bool) string {
	biz := ""
	if business {
		biz = `,"business_npwp":"1234567890123456","business_nib":"0123456789012"`
	}
	return `,"identity":{"id_number":"3175070101901234","place_of_birth":"Jakarta","date_of_birth":"1990-05-20","gender":"male","address":"Jl. Merdeka No. 10, Jakarta Pusat","npwp":"123456789012345"` + biz + `}`
}

// identityJSONWith builds an identity object from defaults plus overrides; an override of "" drops
// the field, so a case can test a missing (not merely wrong) value.
func identityJSONWith(business bool, overrides map[string]string) string {
	m := map[string]string{
		"id_number":      "3175070101901234",
		"place_of_birth": "Jakarta",
		"date_of_birth":  "1990-05-20",
		"gender":         "male",
		"address":        "Jl. Merdeka No. 10, Jakarta Pusat",
	}
	if business {
		m["business_npwp"] = "1234567890123456"
		m["business_nib"] = "0123456789012"
	}
	for k, v := range overrides {
		if v == "" {
			delete(m, k)
			continue
		}
		m[k] = v
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%q:%q", k, m[k]))
	}
	return `,"identity":{` + strings.Join(parts, ",") + `}`
}

func signupJSON(accountType, email, identity string) string {
	return fmt.Sprintf(`{"account_type":"%s","name":"Budi Santoso","business_name":"Toko Budi","email":"%s",`+
		`"phone":"0812 3456 7890","password":"abc12345xyz9","accept_terms":true%s}`, accountType, email, identity)
}

func regBody(email string) string {
	return `{"account_type":"business","name":"Budi Santoso","business_name":"Toko Budi","email":"` + email +
		`","phone":"0812 3456 7890","password":"abc12345xyz9","accept_terms":true` + identityJSON(true) + `}`
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
	// Every body carries a valid identity so each failure is about the field it names; the
	// identity rules themselves are covered by TestRegisterIdentityValidationHTTP.
	ind := identityJSON(false)
	biz := identityJSON(true)
	// base returns the JSON without the outer closing brace, so a case can inject an extra field.
	base := func(iden, more, pw string) string {
		return `{"account_type":` + more + `"name":"Budi Santoso","email":"a@b.test","phone":"81234567890","password":"` + pw + `","accept_terms":true` + iden
	}
	bad := map[string]string{
		"11 chars":         base(ind, `"individual",`, "abcdefghijk") + `}`,
		"short password":   base(ind, `"individual",`, "abc123") + `}`,
		"bad email":        strings.Replace(base(ind, `"individual",`, "abc123456789"), `"a@b.test"`, `"bukan-email"`, 1) + `}`,
		"display name":     strings.Replace(base(ind, `"individual",`, "abc123456789"), `"a@b.test"`, `"Budi <a@b.test>"`, 1) + `}`,
		"bad phone":        strings.Replace(base(ind, `"individual",`, "abc123456789"), `"81234567890"`, `"12345"`, 1) + `}`,
		"terms":            strings.Replace(base(ind, `"individual",`, "abc123456789"), `"accept_terms":true`, `"accept_terms":false`, 1) + `}`,
		"business no name": base(biz, `"business","business_name":"T",`, "abc123456789") + `}`,
		"bad type":         base(ind, `"admin",`, "abc123456789") + `}`,
		"unknown field":    base(ind, `"individual",`, "abc123456789") + `,"merchant_id":"` + w.midA + `","status":"approved"}`,
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
		`{"account_type":"individual","name":"Siti Aminah","email":"siti@toko.test","phone":"+62 812-3456-7890","password":"abc123456789","accept_terms":true`+ind+`}`, nil)
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

// The data diri is OPTIONAL at sign-up (daftar minimal, 15 Okt 2026): an account with no identity is
// accepted as 'draft' and completes the data diri afterwards via the KYC wizard. Every rule still
// lives in Go; a rejected sign-up leaves nothing behind.
func TestRegisterIdentityValidationHTTP(t *testing.T) {
	w := setup(t)
	c := &http.Client{}
	cases := map[string]string{
		"short nik":                signupJSON("business", "a@toko.test", identityJSONWith(true, map[string]string{"id_number": "317507010190123"})),
		"bad dob":                  signupJSON("business", "a@toko.test", identityJSONWith(true, map[string]string{"date_of_birth": "20-05-1990"})),
		"underage":                 signupJSON("business", "a@toko.test", identityJSONWith(true, map[string]string{"date_of_birth": "2020-01-01"})),
		"gender":                   signupJSON("business", "a@toko.test", identityJSONWith(true, map[string]string{"gender": "lainnya"})),
		"short address":            signupJSON("business", "a@toko.test", identityJSONWith(true, map[string]string{"address": "Jl."})),
		"bad npwp":                 signupJSON("business", "a@toko.test", identityJSONWith(true, map[string]string{"npwp": "123"})),
		"business missing npwp":    signupJSON("business", "a@toko.test", identityJSONWith(true, map[string]string{"business_npwp": ""})),
		"business missing nib":     signupJSON("business", "a@toko.test", identityJSONWith(true, map[string]string{"business_nib": ""})),
		"business bad nib":         signupJSON("business", "a@toko.test", identityJSONWith(true, map[string]string{"business_nib": "123"})),
		"business fields on indiv": signupJSON("individual", "a@toko.test", identityJSONWith(false, map[string]string{"business_nib": "0123456789012"})),
	}
	for name, b := range cases {
		if r := w.do(t, c, "POST", "/v1/auth/register", b, nil); r.StatusCode != 400 {
			t.Fatalf("%s: %d", name, r.StatusCode)
		}
	}
	var users int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE email = 'a@toko.test'`).Scan(&users)
	if users != 0 {
		t.Fatal("rejected input created a user")
	}
	// Dots and spacing in the NIK and NPWP are normalized away; individual without NPWP is fine too.
	nikDots := signupJSON("business", "a@toko.test", `,"identity":{"id_number":"3175.0701.0190.1234","place_of_birth":"Jakarta","date_of_birth":"1990-05-20","gender":"male","address":"Jl. Merdeka No. 10","business_npwp":"12.345.678.9-012.345","business_nib":"0123456789012"}`)
	if r := w.do(t, c, "POST", "/v1/auth/register", nikDots, nil); r.StatusCode != 202 {
		t.Fatalf("dots in nik: %d", r.StatusCode)
	}
	// Daftar minimal (15 Okt 2026): no identity is accepted and the merchant starts as 'draft'
	// so the data diri is completed afterwards via the KYC wizard.
	minimal := `{"account_type":"business","name":"Citra Nusantara","business_name":"CV Citra Nusantara","email":"c@toko.test",` +
		`"phone":"0812 3456 7890","password":"abc12345xyz9","accept_terms":true}`
	if r := w.do(t, c, "POST", "/v1/auth/register", minimal, nil); r.StatusCode != 202 {
		t.Fatalf("minimal signup: %d", r.StatusCode)
	}
	var miniStatus string
	_ = w.db.QueryRow(t.Context(), `SELECT onboarding_status FROM merchants m
		JOIN merchant_users mu ON mu.merchant_id = m.id JOIN users u ON u.id = mu.user_id
		WHERE lower(u.email) = 'c@toko.test'`).Scan(&miniStatus)
	if miniStatus != "draft" {
		t.Fatalf("minimal signup must start as draft, got %q", miniStatus)
	}
	r := w.do(t, c, "POST", "/v1/auth/register", signupJSON("individual", "b@toko.test", identityJSONWith(false, nil)), nil)
	if r.StatusCode != 202 {
		t.Fatalf("individual without npwp: %d", r.StatusCode)
	}
	// An absent optional identifier must not render a mask with no digits ("••••"),
	// which would read as a number that exists.
	if view := decode(t, w.do(t, signedUp(t, w, "b@toko.test"), "GET", "/v1/merchant/identity", "", nil)); view["npwp_masked"] != nil || view["business_npwp_masked"] != nil {
		t.Fatalf("absent identifiers must not be masked: %v", view)
	}
	var sealed []byte
	err := w.db.QueryRow(t.Context(), `SELECT owner_id_enc FROM merchants WHERE legal_name = 'Toko Budi'`).Scan(&sealed)
	if err != nil || sealed == nil || string(sealed) == "3175070101901234" {
		t.Fatalf("NIK must be sealed at rest, got %v err %v", sealed, err)
	}
}

// A merchant that completed the KYC wizard has NIK, NPWP and NIB but no birth data, gender or
// personal address (the wizard never asks for them); it must still be approvable.
func TestApproveMerchantFromKYCWizardData(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "wizard@toko.test")
	verifyOwner(t, w, "wizard@toko.test")
	root := w.client(t, "root@erapay.test")
	if _, err := w.db.Exec(t.Context(), `UPDATE merchants SET
		owner_place_of_birth=NULL, owner_date_of_birth=NULL, owner_gender=NULL, owner_address=NULL
		WHERE id = $1`, mid); err != nil {
		t.Fatal(err)
	}
	if r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", `{"mdr_bps":70,"platform_bps":100}`, nil); r.StatusCode != 200 {
		t.Fatalf("approve wizard-style merchant: %d %s", r.StatusCode, errCode(t, r))
	}
}

// Approval cannot happen until the data diri is complete. A merchant that signed up without it
// (pre-existing row, ARUS-provisioned) completes it from its own session; the reviewer detail view
// is the only place the plaintext NIK/NPWP appear.
func TestIdentityGateAndCompletionHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "lengkap@toko.test")
	verifyOwner(t, w, "lengkap@toko.test")
	root := w.client(t, "root@erapay.test")
	owner := signedUp(t, w, "lengkap@toko.test")
	// Wipe the identity: the state a pre-feature or ARUS-provisioned merchant is in.
	if _, err := w.db.Exec(t.Context(), `UPDATE merchants SET
		owner_id_enc=NULL, owner_id_last4=NULL, owner_place_of_birth=NULL, owner_date_of_birth=NULL,
		owner_gender=NULL, owner_address=NULL, owner_npwp_enc=NULL, owner_npwp_last4=NULL,
		business_npwp_enc=NULL, business_npwp_last4=NULL, business_nib=NULL WHERE id = $1`, mid); err != nil {
		t.Fatal(err)
	}
	// The queue flags the missing data; approval is refused until the merchant completes it.
	list := decode(t, w.do(t, root, "GET", "/v1/admin/merchants", "", nil))["data"].([]any)
	if list[0].(map[string]any)["identity_complete"] != false {
		t.Fatalf("queue should flag the missing identity: %v", list[0])
	}
	if r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", `{"mdr_bps":70,"platform_bps":100}`, nil); r.StatusCode != 409 || errCode(t, r) != "IDENTITY_INCOMPLETE" {
		t.Fatalf("approve before identity: %d %s", r.StatusCode, errCode(t, r))
	}
	// The merchant's own view is masked and shows what is missing.
	got := decode(t, w.do(t, owner, "GET", "/v1/merchant/identity", "", nil))
	if got["complete"] != false || got["id_masked"] != nil || got["id_number"] != nil || got["place_of_birth"] != nil {
		t.Fatalf("empty identity view: %v", got)
	}
	// Access control first: only the merchant's own session may write it.
	if r := w.do(t, &http.Client{}, "POST", "/v1/merchant/identity", `{"identity":{}}`, nil); r.StatusCode != 401 {
		t.Fatalf("anon identity: %d", r.StatusCode)
	}
	if r := w.do(t, w.client(t, "cashier@a.test"), "POST", "/v1/merchant/identity", `{"identity":{}}`, nil); r.StatusCode != 403 {
		t.Fatalf("cashier identity: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", "/v1/merchant/identity", `{"identity":{},"merchant_id":"`+w.midA+`"}`, nil); r.StatusCode != 400 {
		t.Fatalf("tenant in body must be refused: %d", r.StatusCode)
	}
	// Completion validates as strictly as sign-up did.
	bad := map[string]string{
		"short nik": `{"identity":{"id_number":"123","place_of_birth":"Jakarta","date_of_birth":"1990-05-20","gender":"male","address":"Jl. Merdeka No. 10","business_npwp":"1234567890123456","business_nib":"0123456789012"}}`,
		"no nib":    `{"identity":{"id_number":"3175070101901234","place_of_birth":"Jakarta","date_of_birth":"1990-05-20","gender":"male","address":"Jl. Merdeka No. 10","business_npwp":"1234567890123456"}}`,
	}
	for name, b := range bad {
		if r := w.do(t, owner, "POST", "/v1/merchant/identity", b, nil); r.StatusCode != 400 {
			t.Fatalf("%s: %d", name, r.StatusCode)
		}
	}
	r := w.do(t, owner, "POST", "/v1/merchant/identity",
		`{"identity":{"id_number":"3175070101901234","place_of_birth":"Bandung","date_of_birth":"1985-11-30","gender":"female","address":"Jl. Asia Afrika No. 8","npwp":"987654321098765","business_npwp":"1234567890123456","business_nib":"0123456789012"}}`, nil)
	got = decode(t, r)
	if r.StatusCode != 200 || got["complete"] != true || got["id_masked"] != "••••1234" || got["place_of_birth"] != "Bandung" || got["id_number"] != nil {
		t.Fatalf("completion: %d %v", r.StatusCode, got)
	}
	// Sealed at rest, bound to the row, and audited without the plaintext.
	var sealed []byte
	if err := w.db.QueryRow(t.Context(), `SELECT owner_id_enc FROM merchants WHERE id = $1`, mid).Scan(&sealed); err != nil || string(sealed) == "3175070101901234" {
		t.Fatalf("NIK sealed at rest: %v %v", sealed, err)
	}
	var audits int
	if err := w.db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'merchant.identity_updated'`, mid).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("identity audits: %d %v", audits, err)
	}
	// Approval works now, and the reviewer detail view is where the plaintext data diri appears.
	if r := w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", `{"mdr_bps":70,"platform_bps":100}`, nil); r.StatusCode != 200 || errCode(t, r) != "" {
		t.Fatalf("approve after identity: %d %s", r.StatusCode, errCode(t, r))
	}
	det := decode(t, w.do(t, root, "GET", "/v1/admin/merchants/"+mid, "", nil))
	idm, _ := det["identity"].(map[string]any)
	if idm == nil || idm["id_number"] != "3175070101901234" || idm["npwp"] != "987654321098765" || idm["business_npwp"] != "1234567890123456" || idm["business_nib"] != "0123456789012" {
		t.Fatalf("reviewer detail: %v", idm)
	}
	// The identity is locked once the merchant is decided: every review endpoint refuses it afterwards.
	if r := w.do(t, owner, "POST", "/v1/merchant/identity", `{"identity":{"id_number":"3175070101901234","place_of_birth":"Jakarta","date_of_birth":"1990-05-20","gender":"male","address":"Jl. Merdeka No. 10","business_npwp":"1234567890123456","business_nib":"0123456789012"}}`, nil); r.StatusCode != 409 {
		t.Fatalf("identity locked after approval: %d", r.StatusCode)
	}
}
