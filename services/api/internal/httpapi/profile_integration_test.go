package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// Pengguna merchant: ubah profil, ganti kata sandi, aktifkan MFA (login lalu meminta kode), dan nonaktifkan MFA
// hanya dengan kata sandi + kode.
func TestMerchantProfileAndMFA(t *testing.T) {
	w := newPlatformWorld(t)
	ctx := t.Context()
	svc := auth.NewService(w.db, auth.DefaultConfig())
	uid, err := svc.CreateUser(ctx, "owner@toko.test", itPW)
	if err != nil {
		t.Fatal(err)
	}
	mid, _ := testdb.SeedMerchant(t, w.db, "Toko Uji")
	testdb.AddMember(t, w.db, mid, uid, "merchant_owner")

	login := func(pw string) *http.Response {
		return w.do(t, &http.Client{}, "POST", "/v1/auth/login", `{"email":"owner@toko.test","password":"`+pw+`"}`, nil)
	}
	lr := login(itPW)
	if lr.StatusCode != 200 {
		t.Fatalf("login: %d", lr.StatusCode)
	}
	hdr := map[string]string{"Cookie": sessionCookie + "=" + readCookie(lr, sessionCookie)}

	// Tanpa sesi ditolak.
	if r := w.do(t, &http.Client{}, "GET", "/v1/profile", "", nil); r.StatusCode != 401 {
		t.Fatalf("profile anon: %d", r.StatusCode)
	}
	// Profil: baca, ubah, validasi.
	if p := decode(t, w.do(t, &http.Client{}, "GET", "/v1/profile", "", hdr)); p["email"] != "owner@toko.test" || p["mfa_enabled"] != false {
		t.Fatalf("profile: %v", p)
	}
	if r := w.do(t, &http.Client{}, "PUT", "/v1/profile", `{"full_name":"Budi Santoso","phone":"+62 812-3456-7890"}`, hdr); r.StatusCode != 200 {
		t.Fatalf("update profile: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "PUT", "/v1/profile", `{"full_name":"x","phone":"abc"}`, hdr); r.StatusCode != 400 {
		t.Fatalf("bad phone: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "PUT", "/v1/profile", `{"full_name":"`+strings.Repeat("a", 101)+`","phone":""}`, hdr); r.StatusCode != 400 {
		t.Fatalf("long name: %d", r.StatusCode)
	}
	if me := decode(t, w.do(t, &http.Client{}, "GET", "/v1/auth/me", "", hdr)); me["full_name"] != "Budi Santoso" {
		t.Fatalf("me.full_name: %v", me["full_name"])
	}

	// Ganti kata sandi: yang lama harus benar; sesi lain dicabut, sesi ini tetap.
	other := login(itPW)
	otherHdr := map[string]string{"Cookie": sessionCookie + "=" + readCookie(other, sessionCookie)}
	if r := w.do(t, &http.Client{}, "POST", "/v1/profile/password", `{"current_password":"salah-salah-salah","new_password":"kata sandi baru panjang"}`, hdr); r.StatusCode != 401 {
		t.Fatalf("wrong current: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "POST", "/v1/profile/password", `{"current_password":"`+itPW+`","new_password":"pendek"}`, hdr); r.StatusCode != 400 {
		t.Fatalf("weak new: %d", r.StatusCode)
	}
	const newPW = "kata sandi baru panjang"
	if r := w.do(t, &http.Client{}, "POST", "/v1/profile/password", `{"current_password":"`+itPW+`","new_password":"`+newPW+`"}`, hdr); r.StatusCode != 204 {
		t.Fatalf("change password: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "GET", "/v1/profile", "", hdr); r.StatusCode != 200 {
		t.Fatalf("current session must survive: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "GET", "/v1/profile", "", otherHdr); r.StatusCode != 401 {
		t.Fatalf("other session must be revoked: %d", r.StatusCode)
	}
	if r := login(itPW); r.StatusCode != 401 {
		t.Fatalf("old password still works: %d", r.StatusCode)
	}

	// MFA: enroll -> confirm dengan kode TOTP.
	en := decode(t, w.do(t, &http.Client{}, "POST", "/v1/auth/mfa/enroll", `{}`, hdr))
	secret, _ := en["secret"].(string)
	if secret == "" || len(en["recovery_codes"].([]any)) == 0 {
		t.Fatalf("enroll: %v", en)
	}
	if r := w.do(t, &http.Client{}, "POST", "/v1/auth/mfa/confirm", `{"code":"000000"}`, hdr); r.StatusCode != 401 {
		t.Fatalf("bad confirm code: %d", r.StatusCode)
	}
	code, _ := totp.GenerateCode(secret, w.now())
	if r := w.do(t, &http.Client{}, "POST", "/v1/auth/mfa/confirm", `{"code":"`+code+`"}`, hdr); r.StatusCode != 200 {
		t.Fatalf("confirm: %d", r.StatusCode)
	}
	if p := decode(t, w.do(t, &http.Client{}, "GET", "/v1/profile", "", hdr)); p["mfa_enabled"] != true {
		t.Fatalf("mfa_enabled: %v", p)
	}

	// Login berikutnya (pengguna merchant) kini meminta tantangan MFA, bukan sesi.
	lr2 := w.do(t, &http.Client{}, "POST", "/v1/auth/login", `{"email":"owner@toko.test","password":"`+newPW+`"}`, nil)
	b := decode(t, lr2)
	if b["mfa_required"] != true || readCookie(lr2, sessionCookie) != "" {
		t.Fatalf("login must require MFA: %v", b)
	}

	// Nonaktifkan MFA wajib kata sandi + kode.
	if r := w.do(t, &http.Client{}, "POST", "/v1/auth/mfa/disable", `{}`, hdr); r.StatusCode != 400 {
		t.Fatalf("disable without proof: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "POST", "/v1/auth/mfa/disable", `{"password":"`+newPW+`","code":"000000"}`, hdr); r.StatusCode != 401 {
		t.Fatalf("disable wrong code: %d", r.StatusCode)
	}
	if r := w.do(t, &http.Client{}, "POST", "/v1/auth/mfa/disable", `{"password":"salah-salah-salah","code":"`+code+`"}`, hdr); r.StatusCode != 401 {
		t.Fatalf("disable wrong password: %d", r.StatusCode)
	}
	code, _ = totp.GenerateCode(secret, w.now())
	if r := w.do(t, &http.Client{}, "POST", "/v1/auth/mfa/disable", `{"password":"`+newPW+`","code":"`+code+`"}`, hdr); r.StatusCode != 200 {
		t.Fatalf("disable: %d", r.StatusCode)
	}
	if p := decode(t, w.do(t, &http.Client{}, "GET", "/v1/profile", "", hdr)); p["mfa_enabled"] != false {
		t.Fatalf("mfa still on: %v", p)
	}
}

func (w *platformWorld) now() time.Time { return time.Now() }
