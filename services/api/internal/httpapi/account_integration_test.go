package httpapi

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/onboarding"
)

func postJSON(t *testing.T, w *world, path, body string) *http.Response {
	t.Helper()
	return w.do(t, &http.Client{}, "POST", path, body, nil)
}

func mailsTo(w *world, to string) (n int) {
	for _, m := range w.sent() {
		if strings.EqualFold(m.To, to) {
			n++
		}
	}
	return n
}

func TestEmailVerificationHTTP(t *testing.T) {
	w := setup(t)

	// A failing mail relay must not lose the sign-up: the account exists and a new link can be requested.
	w.mail.Fail = errors.New("smtp down")
	register(t, w, "gagal@toko.test")
	if mailsTo(w, "gagal@toko.test") != 0 {
		t.Fatal("mail was recorded despite the failure")
	}
	w.mail.Fail = nil
	if r := postJSON(t, w, "/v1/auth/resend-verification", `{"email":"gagal@toko.test"}`); r.StatusCode != 202 {
		t.Fatalf("resend: %d", r.StatusCode)
	}
	first := mailLink(t, w, "gagal@toko.test", "/verify-email")

	// A newer link replaces the older one.
	if r := postJSON(t, w, "/v1/auth/resend-verification", `{"email":"gagal@toko.test"}`); r.StatusCode != 202 {
		t.Fatalf("resend 2: %d", r.StatusCode)
	}
	second := mailLink(t, w, "gagal@toko.test", "/verify-email")
	if first == second {
		t.Fatal("two links with the same token")
	}
	for name, tok := range map[string]string{"superseded": first, "garbage": "nope", "empty": ""} {
		r := postJSON(t, w, "/v1/auth/verify-email", `{"token":"`+tok+`"}`)
		if r.StatusCode != 400 || errCode(t, r) != "INVALID_TOKEN" {
			t.Fatalf("%s token: %d", name, r.StatusCode)
		}
	}

	// A password-reset token is not a verification token.
	postJSON(t, w, "/v1/auth/forgot-password", `{"email":"gagal@toko.test"}`)
	reset := mailLink(t, w, "gagal@toko.test", "/reset-password")
	if r := postJSON(t, w, "/v1/auth/verify-email", `{"token":"`+reset+`"}`); r.StatusCode != 400 {
		t.Fatalf("purpose confusion: %d", r.StatusCode)
	}

	// Expired.
	if _, err := w.db.Exec(t.Context(), `UPDATE email_tokens SET expires_at = now() - interval '1 second' WHERE purpose = 'verify_email'`); err != nil {
		t.Fatal(err)
	}
	if r := postJSON(t, w, "/v1/auth/verify-email", `{"token":"`+second+`"}`); r.StatusCode != 400 {
		t.Fatalf("expired: %d", r.StatusCode)
	}
	postJSON(t, w, "/v1/auth/resend-verification", `{"email":"gagal@toko.test"}`)
	third := mailLink(t, w, "gagal@toko.test", "/verify-email")
	if r := postJSON(t, w, "/v1/auth/verify-email", `{"token":"`+third+`"}`); r.StatusCode != 200 {
		t.Fatalf("verify: %d", r.StatusCode)
	}
	if r := postJSON(t, w, "/v1/auth/verify-email", `{"token":"`+third+`"}`); r.StatusCode != 400 {
		t.Fatalf("token reuse: %d", r.StatusCode)
	}
	var ok bool
	_ = w.db.QueryRow(t.Context(), `SELECT email_verified_at IS NOT NULL FROM users WHERE email = 'gagal@toko.test'`).Scan(&ok)
	if !ok {
		t.Fatal("not verified")
	}
	// Already verified, unknown, and malformed addresses: same 202, no mail, nothing to learn.
	n := len(w.sent())
	for _, e := range []string{"gagal@toko.test", "tidak-ada@toko.test"} {
		if r := postJSON(t, w, "/v1/auth/resend-verification", `{"email":"`+e+`"}`); r.StatusCode != 202 {
			t.Fatalf("resend %s: %d", e, r.StatusCode)
		}
	}
	if len(w.sent()) != n {
		t.Fatal("resend mailed an address that needs nothing")
	}
}

func TestForgotAndResetPasswordHTTP(t *testing.T) {
	w := setup(t)

	// The answer for an existing and an unknown address is byte-identical.
	rk := postJSON(t, w, "/v1/auth/forgot-password", `{"email":"cashier@a.test"}`)
	ru := postJSON(t, w, "/v1/auth/forgot-password", `{"email":"hantu@a.test"}`)
	bk, _ := io.ReadAll(rk.Body)
	bu, _ := io.ReadAll(ru.Body)
	if rk.StatusCode != 202 || ru.StatusCode != 202 || string(bk) != string(bu) {
		t.Fatalf("forgot answers differ: %d %q vs %d %q", rk.StatusCode, bk, ru.StatusCode, bu)
	}
	if mailsTo(w, "cashier@a.test") != 1 || mailsTo(w, "hantu@a.test") != 0 {
		t.Fatalf("mails: known=%d unknown=%d", mailsTo(w, "cashier@a.test"), mailsTo(w, "hantu@a.test"))
	}
	tok := mailLink(t, w, "cashier@a.test", "/reset-password")
	old := w.client(t, "cashier@a.test") // a live session that the reset must end
	if r := w.do(t, old, "GET", "/v1/auth/me", "", nil); r.StatusCode != 200 {
		t.Fatalf("me before: %d", r.StatusCode)
	}

	// A rejected password does not burn the token.
	if r := postJSON(t, w, "/v1/auth/reset-password", `{"token":"`+tok+`","password":"short"}`); r.StatusCode != 400 {
		t.Fatalf("weak password: %d", r.StatusCode)
	}
	if r := postJSON(t, w, "/v1/auth/reset-password", `{"token":"bogus","password":"a-brand-new-password"}`); r.StatusCode != 400 || errCode(t, r) != "INVALID_TOKEN" {
		t.Fatalf("bogus token: %d", r.StatusCode)
	}
	if r := postJSON(t, w, "/v1/auth/reset-password", `{"token":"`+tok+`","password":"a-brand-new-password"}`); r.StatusCode != 200 {
		t.Fatalf("reset: %d", r.StatusCode)
	}
	if r := w.do(t, old, "GET", "/v1/auth/me", "", nil); r.StatusCode != 401 {
		t.Fatalf("old session survived the reset: %d", r.StatusCode)
	}
	if r := postJSON(t, w, "/v1/auth/login", `{"email":"cashier@a.test","password":"`+itPW+`"}`); r.StatusCode != 401 {
		t.Fatalf("old password still works: %d", r.StatusCode)
	}
	if r := postJSON(t, w, "/v1/auth/login", `{"email":"cashier@a.test","password":"a-brand-new-password"}`); r.StatusCode != 200 {
		t.Fatalf("new password: %d", r.StatusCode)
	}
	if r := postJSON(t, w, "/v1/auth/reset-password", `{"token":"`+tok+`","password":"another-new-password"}`); r.StatusCode != 400 {
		t.Fatalf("token reuse: %d", r.StatusCode)
	}
	if mailsTo(w, "cashier@a.test") != 2 { // reset link + "password changed" notice
		t.Fatalf("mails to cashier: %d", mailsTo(w, "cashier@a.test"))
	}
	var audits int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action = 'auth.password_reset'`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("audit rows: %d", audits)
	}
	// A reset proves control of the mailbox, so it also verifies the address.
	var ok bool
	_ = w.db.QueryRow(t.Context(), `SELECT email_verified_at IS NOT NULL FROM users WHERE email = 'cashier@a.test'`).Scan(&ok)
	if !ok {
		t.Fatal("reset did not verify the address")
	}
}

// The same reset link used from several tabs at once: exactly one password wins.
func TestResetTokenConcurrentHTTP(t *testing.T) {
	w := setup(t)
	postJSON(t, w, "/v1/auth/forgot-password", `{"email":"integrator@a.test"}`)
	tok := mailLink(t, w, "integrator@a.test", "/reset-password")
	const n = 6
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = postJSON(t, w, "/v1/auth/reset-password", fmt.Sprintf(`{"token":%q,"password":"parallel-password-%d"}`, tok, i)).StatusCode
		}()
	}
	wg.Wait()
	winner, wins := -1, 0
	for i, c := range codes {
		switch c {
		case 200:
			winner, wins = i, wins+1
		case 400:
		default:
			t.Fatalf("codes: %v", codes)
		}
	}
	if wins != 1 {
		t.Fatalf("winners=%d codes=%v", wins, codes)
	}
	// Log the winner in first: if the winner came last, the five wrong
	// attempts before it would trip the per-email login rate limit (429).
	r := postJSON(t, w, "/v1/auth/login", fmt.Sprintf(`{"email":"integrator@a.test","password":"parallel-password-%d"}`, winner))
	if r.StatusCode != 200 {
		t.Fatalf("winner password %d login=%d, want 200", winner, r.StatusCode)
	}
	for i := range n {
		if i == winner {
			continue
		}
		r := postJSON(t, w, "/v1/auth/login", fmt.Sprintf(`{"email":"integrator@a.test","password":"parallel-password-%d"}`, i))
		if r.StatusCode != 401 {
			t.Fatalf("password %d login=%d, want 401", i, r.StatusCode)
		}
	}
}

func TestEmailEndpointsThrottleHTTP(t *testing.T) {
	w := setup(t)
	// Per address: a victim cannot be mail-bombed. The 4th request still answers 202 but sends nothing.
	for i := range 5 {
		if r := postJSON(t, w, "/v1/auth/forgot-password", `{"email":"owner@a.test"}`); r.StatusCode != 202 {
			t.Fatalf("request %d: %d", i, r.StatusCode)
		}
	}
	if got := mailsTo(w, "owner@a.test"); got != 3 {
		t.Fatalf("mails to one address: %d, want 3", got)
	}
	// Per IP: 10 per hour in total, then 429 (and that says nothing about any address).
	for i := range 5 { // 5 more, all different addresses: 10 in total
		postJSON(t, w, "/v1/auth/forgot-password", fmt.Sprintf(`{"email":"x%d@a.test"}`, i))
	}
	r := postJSON(t, w, "/v1/auth/forgot-password", `{"email":"y@a.test"}`)
	if r.StatusCode != 429 || r.Header.Get("Retry-After") == "" {
		t.Fatalf("over the IP limit: %d", r.StatusCode)
	}
}

func TestSignupDisabledWithoutMailerHTTP(t *testing.T) {
	w := setup(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(NewRouter(Deps{Log: log, DB: fakePinger{}, Redis: fakePinger{}, Auth: w.svc,
		Onboarding: onboarding.NewService(w.db, nil, onboarding.Config{Log: log})}))
	defer srv.Close()
	for _, p := range []string{"/v1/auth/register", "/v1/auth/forgot-password", "/v1/auth/reset-password", "/v1/auth/verify-email"} {
		r, err := http.Post(srv.URL+p, "application/json", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 404 && r.StatusCode != 405 {
			t.Fatalf("%s must not exist without a mailer: %d", p, r.StatusCode)
		}
	}
}

// start-review and request-changes belong to reviewers; the applicant answers with resubmit.
func TestReviewStatesAndResubmitHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "revisi@toko.test")
	verifyOwner(t, w, "revisi@toko.test")
	fin := w.client(t, "finance@erapay.test")
	root := w.client(t, "root@erapay.test")
	act := func(c *http.Client, action, body string) *http.Response {
		return w.do(t, c, "POST", "/v1/admin/merchants/"+mid+"/"+action, body, nil)
	}
	status := func() string {
		var s string
		_ = w.db.QueryRow(t.Context(), `SELECT onboarding_status FROM merchants WHERE id = $1`, mid).Scan(&s)
		return s
	}

	if r := act(fin, "start-review", ""); r.StatusCode != 200 || status() != "under_review" {
		t.Fatalf("start-review: %d %s", r.StatusCode, status())
	}
	if r := act(fin, "start-review", ""); r.StatusCode != 409 {
		t.Fatalf("start-review twice: %d", r.StatusCode)
	}
	if r := act(fin, "request-changes", `{"note":" "}`); r.StatusCode != 400 {
		t.Fatalf("changes without a note: %d", r.StatusCode)
	}
	if r := act(fin, "request-changes", `{"note":"Nama usaha tidak sesuai NPWP"}`); r.StatusCode != 200 || status() != "action_required" {
		t.Fatalf("request-changes: %d %s", r.StatusCode, status())
	}

	owner := signedUp(t, w, "revisi@toko.test")
	prof := decode(t, w.do(t, owner, "GET", "/v1/merchant/profile", "", nil))
	if prof["onboarding_status"] != "action_required" || prof["review_note"] != "Nama usaha tidak sesuai NPWP" {
		t.Fatalf("applicant sees: %v", prof)
	}
	// Only action_required can be resubmitted, only by the merchant's own session, never by staff of another merchant.
	if r := w.do(t, w.client(t, "owner@a.test"), "POST", "/v1/merchant/resubmit", `{}`, nil); r.StatusCode != 409 {
		t.Fatalf("approved merchant resubmit: %d", r.StatusCode)
	}
	if r := w.do(t, w.client(t, "cashier@a.test"), "POST", "/v1/merchant/resubmit", `{}`, nil); r.StatusCode != 403 {
		t.Fatalf("cashier resubmit: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "POST", "/v1/merchant/resubmit", `{}`, nil); r.StatusCode != 401 {
		t.Fatalf("anon resubmit: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", "/v1/merchant/resubmit", `{"legal_name":"x","contact_phone":"0812 1111 2222"}`, nil); r.StatusCode != 400 {
		t.Fatalf("bad legal name: %d", r.StatusCode)
	}
	if r := w.do(t, owner, "POST", "/v1/merchant/resubmit", `{"merchant_id":"`+w.midA+`"}`, nil); r.StatusCode != 400 {
		t.Fatalf("tenant in body must be refused: %d", r.StatusCode)
	}
	r := w.do(t, owner, "POST", "/v1/merchant/resubmit", `{"legal_name":"PT Toko Budi Sejahtera","contact_phone":"0812 1111 2222"}`, nil)
	got := decode(t, r)
	if r.StatusCode != 200 || got["status"] != "submitted" || got["legal_name"] != "PT Toko Budi Sejahtera" ||
		got["display_name"] != "PT Toko Budi Sejahtera" || got["contact_phone"] != "+6281211112222" || got["review_note"] != nil {
		t.Fatalf("resubmit: %d %v", r.StatusCode, got)
	}
	if r := w.do(t, owner, "POST", "/v1/merchant/resubmit", `{}`, nil); r.StatusCode != 409 {
		t.Fatalf("resubmit twice: %d", r.StatusCode)
	}

	// Staff of a merchant cannot drive the review; finance cannot approve; only super_admin decides.
	for _, who := range []string{"owner@a.test", "revisi@toko.test"} {
		if r := act(clientFor(t, w, who), "start-review", ""); r.StatusCode != 403 {
			t.Fatalf("%s start-review: %d", who, r.StatusCode)
		}
	}
	if r := act(fin, "approve", `{"mdr_bps":70,"platform_bps":100}`); r.StatusCode != 403 {
		t.Fatalf("finance approve: %d", r.StatusCode)
	}
	// An application sent back for changes can still be decided directly by super_admin.
	act(fin, "request-changes", `{"note":"Lampirkan NIB"}`)
	if r := act(root, "approve", `{"mdr_bps":70,"platform_bps":100}`); r.StatusCode != 200 || status() != "approved" {
		t.Fatalf("approve after changes: %d %s", r.StatusCode, status())
	}
	// Decided is final for every review action.
	for _, a := range []string{"start-review", "request-changes"} {
		if r := act(fin, a, `{"note":"terlambat"}`); r.StatusCode != 409 {
			t.Fatalf("%s on an approved merchant: %d", a, r.StatusCode)
		}
	}
}

// The sign-up limit is configuration (ERAPAY_REGISTER_PER_IP_HOUR): a shared office address can be given more room.
func TestRegisterLimitIsConfigurableHTTP(t *testing.T) {
	w := setup(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for limit, want := range map[int]int{2: 2, 7: 7} {
		w.db.Exec(t.Context(), `DELETE FROM register_attempts`) //nolint:errcheck
		srv := httptest.NewServer(NewRouter(Deps{Log: log, DB: fakePinger{}, Redis: fakePinger{}, Auth: w.svc,
			Onboarding: onboarding.NewService(w.db, w.mailNotifier(), onboarding.Config{RegisterPerIP: limit, Box: testBox(t), Log: log})}))
		ok := 0
		for i := range 10 {
			r, err := http.Post(srv.URL+"/v1/auth/register", "application/json",
				strings.NewReader(regBody(fmt.Sprintf("cfg%d-%d@toko.test", limit, i))))
			if err != nil {
				t.Fatal(err)
			}
			r.Body.Close()
			if r.StatusCode == 202 {
				ok++
			} else if r.StatusCode != 429 {
				t.Fatalf("status %d", r.StatusCode)
			}
		}
		srv.Close()
		if ok != want {
			t.Fatalf("limit %d allowed %d sign-ups", limit, ok)
		}
	}
}

func TestThrottleTrimsOldRowsHTTP(t *testing.T) {
	w := setup(t)
	if _, err := w.db.Exec(t.Context(), `INSERT INTO register_attempts (kind, ip, created_at) VALUES ('forgot','127.0.0.1', now() - interval '2 days'), ('forgot','127.0.0.1', now() - interval '10 minutes')`); err != nil {
		t.Fatal(err)
	}
	postJSON(t, w, "/v1/auth/forgot-password", `{"email":"a@a.test"}`)
	var old, recent int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE created_at < now() - interval '1 day'), count(*) FILTER (WHERE created_at >= now() - interval '1 day') FROM register_attempts`).Scan(&old, &recent)
	if old != 0 || recent != 2 { // the 10-minute-old row (still inside the window) and the new attempt
		t.Fatalf("old=%d recent=%d", old, recent)
	}
}

// A slow mail relay must not show up in the response time: otherwise "forgot password" would be slower for
// real accounts than for unknown addresses, which tells an attacker which addresses are registered.
func TestEmailRequestsDoNotWaitForTheRelayHTTP(t *testing.T) {
	w := setup(t)
	w.mail.Delay = 600 * time.Millisecond
	for _, email := range []string{"cashier@a.test", "hantu@a.test"} {
		start := time.Now()
		r := postJSON(t, w, "/v1/auth/forgot-password", `{"email":"`+email+`"}`)
		if r.StatusCode != 202 || time.Since(start) > 400*time.Millisecond {
			t.Fatalf("%s: %d after %v", email, r.StatusCode, time.Since(start))
		}
	}
	if mailsTo(w, "cashier@a.test") != 1 { // delivered in the background, and only for the real account
		t.Fatal("the reset email never arrived")
	}
}

// Persetujuan bisa dicabut (approved -> under_review | rejected) hanya oleh pemegang merchant:approve,
// dengan alasan; sekali saja (konkuren), dan setelah itu merchant bisa disetujui ulang (plan versi 2).
func TestRevokeApprovalHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "cabut@toko.test")
	verifyOwner(t, w, "cabut@toko.test")
	fin := w.client(t, "finance@erapay.test")
	root := w.client(t, "root@erapay.test")
	act := func(c *http.Client, action, body string) *http.Response {
		return w.do(t, c, "POST", "/v1/admin/merchants/"+mid+"/"+action, body, nil)
	}
	status := func() string {
		var s string
		_ = w.db.QueryRow(t.Context(), `SELECT onboarding_status FROM merchants WHERE id = $1`, mid).Scan(&s)
		return s
	}
	if r := act(root, "revoke", `{"to":"rejected","note":"belum disetujui"}`); r.StatusCode != 409 {
		t.Fatalf("revoke sebelum approved: %d", r.StatusCode)
	}
	if r := act(root, "approve", `{"mdr_bps":70,"platform_bps":100}`); r.StatusCode != 200 {
		t.Fatalf("approve: %d", r.StatusCode)
	}
	if r := act(fin, "revoke", `{"to":"rejected","note":"dokumen palsu"}`); r.StatusCode != 403 {
		t.Fatalf("finance revoke: %d", r.StatusCode)
	}
	if r := act(root, "revoke", `{"to":"approved","note":"dokumen palsu"}`); r.StatusCode != 400 {
		t.Fatalf("to tidak valid: %d", r.StatusCode)
	}
	if r := act(root, "revoke", `{"to":"rejected","note":" "}`); r.StatusCode != 400 {
		t.Fatalf("tanpa alasan: %d", r.StatusCode)
	}
	if status() != "approved" {
		t.Fatalf("status berubah oleh permintaan tidak sah: %s", status())
	}
	// Dua pencabutan serentak: tepat satu yang menang.
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() { codes <- act(root, "revoke", `{"to":"under_review","note":"cek ulang dokumen"}`).StatusCode }()
	}
	a, b := <-codes, <-codes
	if !(a == 200 && b == 409) && !(a == 409 && b == 200) {
		t.Fatalf("revoke konkuren: %d %d", a, b)
	}
	if status() != "under_review" {
		t.Fatalf("status = %s", status())
	}
	var n int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='merchant.approval_revoked' AND entity_id=$1`, mid).Scan(&n)
	if n != 1 {
		t.Fatalf("audit revoke = %d", n)
	}
	// Disetujui ulang: plan harga versi berikutnya, bukan bentrok unique(code, version).
	if r := act(root, "approve", `{"mdr_bps":80,"platform_bps":100}`); r.StatusCode != 200 || status() != "approved" {
		t.Fatalf("approve ulang: %d %s", r.StatusCode, status())
	}
	var plans int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM pricing_plans WHERE code = 'qris-'||$1::text`, mid).Scan(&plans)
	if plans != 2 {
		t.Fatalf("plans = %d, want 2", plans)
	}
	// Setiap persetujuan juga membuat paket IDS yang setara, supaya transaksi lewat IDS punya harga.
	var idsPlans int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM pricing_plans WHERE gateway = 'ids' AND code = 'ids-qris-'||$1::text`, mid).Scan(&idsPlans)
	if idsPlans != 2 {
		t.Fatalf("ids plans = %d, want 2", idsPlans)
	}
	// Ditolak: merchant boleh memperbaiki lagi.
	if r := act(root, "revoke", `{"to":"rejected","note":"transaksi mencurigakan"}`); r.StatusCode != 200 || status() != "rejected" {
		t.Fatalf("revoke -> rejected: %d %s", r.StatusCode, status())
	}
}

// Admin (merchant:approve) bisa memverifikasi email pemilik secara manual; alasan wajib, tercatat, idempoten.
func TestVerifyEmailManuallyHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "manual@toko.test")
	root := w.client(t, "root@erapay.test")
	fin := w.client(t, "finance@erapay.test") // merchant:review cukup
	act := func(c *http.Client, body string) *http.Response {
		return w.do(t, c, "POST", "/v1/admin/merchants/"+mid+"/verify-email", body, nil)
	}
	verified := func() bool {
		var ok bool
		_ = w.db.QueryRow(t.Context(), `SELECT COALESCE(bool_or(u.email_verified_at IS NOT NULL), false) FROM merchant_users mu
			JOIN users u ON u.id = mu.user_id WHERE mu.merchant_id = $1`, mid).Scan(&ok)
		return ok
	}
	if r := act(w.client(t, "owner@a.test"), `{"note":"konfirmasi via telepon"}`); r.StatusCode != 403 {
		t.Fatalf("staf merchant: %d", r.StatusCode)
	}
	if r := act(root, `{"note":" "}`); r.StatusCode != 400 || verified() {
		t.Fatalf("tanpa alasan: %d", r.StatusCode)
	}
	for i, c := range []*http.Client{fin, root} {
		if r := act(c, `{"note":"konfirmasi via telepon"}`); r.StatusCode != 200 || !verified() {
			t.Fatalf("verifikasi #%d: %d", i, r.StatusCode)
		}
	}
	var n int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='merchant.email_verified_manually' AND entity_id=$1`, mid).Scan(&n)
	if n != 1 {
		t.Fatalf("audit = %d, want 1 (idempoten)", n)
	}
}

// Tangguhkan: dari antrean atau approved -> suspended (alasan wajib, hanya merchant:approve);
// aktifkan kembali -> approved bila pernah disetujui, selain itu under_review.
func TestSuspendAndReactivateHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "tangguh@toko.test")
	verifyOwner(t, w, "tangguh@toko.test")
	fin := w.client(t, "finance@erapay.test")
	root := w.client(t, "root@erapay.test")
	act := func(c *http.Client, action, body string) *http.Response {
		return w.do(t, c, "POST", "/v1/admin/merchants/"+mid+"/"+action, body, nil)
	}
	status := func() string {
		var s string
		_ = w.db.QueryRow(t.Context(), `SELECT onboarding_status FROM merchants WHERE id = $1`, mid).Scan(&s)
		return s
	}
	if r := act(fin, "suspend", `{"note":"indikasi fraud"}`); r.StatusCode != 403 {
		t.Fatalf("finance suspend: %d", r.StatusCode)
	}
	if r := act(root, "suspend", `{"note":" "}`); r.StatusCode != 400 {
		t.Fatalf("tanpa alasan: %d", r.StatusCode)
	}
	if r := act(root, "reactivate", `{"note":"salah status"}`); r.StatusCode != 409 {
		t.Fatalf("reactivate bukan suspended: %d", r.StatusCode)
	}
	// Belum pernah disetujui: ditangguhkan dari antrean -> aktif kembali ke under_review.
	if r := act(root, "suspend", `{"note":"indikasi fraud"}`); r.StatusCode != 200 || status() != "suspended" {
		t.Fatalf("suspend antrean: %d %s", r.StatusCode, status())
	}
	if r := act(root, "reactivate", `{"note":"sudah klarifikasi"}`); r.StatusCode != 200 || status() != "under_review" {
		t.Fatalf("reactivate -> %s", status())
	}
	// Pernah disetujui: kembali approved dengan plan harga yang sama.
	if r := act(root, "approve", `{"mdr_bps":70,"platform_bps":100}`); r.StatusCode != 200 {
		t.Fatalf("approve: %d", r.StatusCode)
	}
	if r := act(root, "suspend", `{"note":"audit internal"}`); r.StatusCode != 200 || status() != "suspended" {
		t.Fatalf("suspend approved: %d %s", r.StatusCode, status())
	}
	if r := act(root, "suspend", `{"note":"dua kali"}`); r.StatusCode != 409 {
		t.Fatalf("suspend dua kali: %d", r.StatusCode)
	}
	if r := act(root, "reactivate", `{"note":"audit selesai"}`); r.StatusCode != 200 || status() != "approved" {
		t.Fatalf("reactivate -> %s", status())
	}
	var n int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE entity_id=$1 AND action IN ('merchant.suspended','merchant.reactivated')`, mid).Scan(&n)
	if n != 4 {
		t.Fatalf("audit = %d, want 4", n)
	}
}

// Tarif persetujuan: tiap komponen (MDR, platform) boleh persen, nominal rupiah, atau keduanya;
// tanpa nilai bawaan dan nominal dibatasi agar salah ketik tidak lolos.
func TestApproveFixedFeesHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "tarif@toko.test")
	verifyOwner(t, w, "tarif@toko.test")
	root := w.client(t, "root@erapay.test")
	approve := func(body string) *http.Response {
		return w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", body, nil)
	}
	for name, body := range map[string]string{
		"kosong":            `{}`,
		"platform hilang":   `{"mdr_bps":70}`,
		"mdr hilang":        `{"platform_fixed":500}`,
		"nominal negatif":   `{"mdr_bps":70,"platform_fixed":-1}`,
		"nominal kelewatan": `{"mdr_fixed":5000000,"platform_bps":100}`,
	} {
		if r := approve(body); r.StatusCode != 400 {
			t.Fatalf("%s: %d, want 400", name, r.StatusCode)
		}
	}
	// MDR dalam rupiah tetap, platform dalam persen.
	if r := approve(`{"mdr_fixed":500,"platform_bps":100}`); r.StatusCode != 200 {
		t.Fatalf("approve: %d", r.StatusCode)
	}
	var mdrBPS, mdrFixed, platBPS, platFixed int64
	if err := w.db.QueryRow(t.Context(), `SELECT mdr_bps, mdr_fixed, platform_bps, platform_fixed FROM pricing_plans WHERE code = 'qris-'||$1::text`,
		mid).Scan(&mdrBPS, &mdrFixed, &platBPS, &platFixed); err != nil {
		t.Fatal(err)
	}
	if mdrBPS != 0 || mdrFixed != 500 || platBPS != 100 || platFixed != 0 {
		t.Fatalf("plan = mdr %d bps + %d, platform %d bps + %d", mdrBPS, mdrFixed, platBPS, platFixed)
	}
}

// Aturan transaksi kecil di persetujuan: nilai batas dan flat harus berpasangan, dan tersimpan di plan.
func TestApproveFlatSmallTransactionHTTP(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "flat@toko.test")
	verifyOwner(t, w, "flat@toko.test")
	root := w.client(t, "root@erapay.test")
	approve := func(body string) *http.Response {
		return w.do(t, root, "POST", "/v1/admin/merchants/"+mid+"/approve", body, nil)
	}
	for name, body := range map[string]string{
		"tanpa pasangan":     `{"mdr_bps":70,"platform_bps":100,"mdr_flat_up_to":100000}`,
		"flat > batas":       `{"mdr_bps":70,"platform_bps":100,"mdr_flat_up_to":300,"mdr_flat_fee":500}`,
		"flat tanpa batas":   `{"mdr_bps":70,"platform_bps":100,"mdr_flat_up_to":0,"mdr_flat_fee":500}`,
		"flat kelewat besar": `{"mdr_bps":70,"platform_bps":100,"mdr_flat_up_to":100000,"mdr_flat_fee":2000000}`,
	} {
		if r := approve(body); r.StatusCode != 400 {
			t.Fatalf("%s: %d, want 400", name, r.StatusCode)
		}
	}
	if r := approve(`{"mdr_bps":70,"platform_bps":100,"mdr_flat_up_to":100000,"mdr_flat_fee":500}`); r.StatusCode != 200 {
		t.Fatalf("approve: %d", r.StatusCode)
	}
	var upTo, fee int64
	if err := w.db.QueryRow(t.Context(), `SELECT mdr_flat_up_to, mdr_flat_fee FROM pricing_plans WHERE code = 'qris-'||$1::text`, mid).Scan(&upTo, &fee); err != nil {
		t.Fatal(err)
	}
	if upTo != 100000 || fee != 500 {
		t.Fatalf("plan flat = %d / %d", upTo, fee)
	}
}

// DEWA menetapkan kata sandi pemilik langsung (klien khusus tanpa email): email jadi terverifikasi,
// kata sandi baru langsung bisa dipakai login, sesi lama ditutup, password tidak masuk audit, dan
// permintaan konkuren/berulang tetap konsisten.
func TestSetOwnerPasswordDirect(t *testing.T) {
	w := setup(t)
	mid := register(t, w, "khusus@toko.test")
	svc := onboarding.NewService(w.db, w.mailNotifier(), onboarding.Config{Box: testBox(t), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	var actor string
	if err := w.db.QueryRow(t.Context(), `SELECT id::text FROM users WHERE email = 'root@erapay.test'`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	in := onboarding.SetOwnerPasswordInput{MerchantID: mid, ActorID: actor, Password: "kata-sandi-khusus-123", Note: "klien khusus, tanpa email"}
	if _, err := svc.SetOwnerPassword(t.Context(), onboarding.SetOwnerPasswordInput{MerchantID: mid, Password: "pendek", Note: "x yz"}); err == nil {
		t.Fatal("password pendek harus ditolak")
	}
	if _, err := svc.SetOwnerPassword(t.Context(), onboarding.SetOwnerPasswordInput{MerchantID: mid, Password: in.Password, Note: " "}); err == nil {
		t.Fatal("tanpa alasan harus ditolak")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.SetOwnerPassword(t.Context(), in); err != nil {
				t.Errorf("set password: %v", err)
			}
		}()
	}
	wg.Wait()
	r := w.do(t, &http.Client{}, "POST", "/v1/auth/login", `{"email":"khusus@toko.test","password":"`+in.Password+`"}`, nil)
	if r.StatusCode != 200 {
		t.Fatalf("login dengan password baru: %d", r.StatusCode)
	}
	var verified bool
	_ = w.db.QueryRow(t.Context(), `SELECT email_verified_at IS NOT NULL FROM users WHERE email = 'khusus@toko.test'`).Scan(&verified)
	if !verified {
		t.Fatal("email harus terverifikasi")
	}
	var leaked int
	_ = w.db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='merchant.owner_password_set' AND metadata::text LIKE '%'||$1||'%'`, in.Password).Scan(&leaked)
	if leaked != 0 {
		t.Fatal("password bocor ke audit")
	}
}
