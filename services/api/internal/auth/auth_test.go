package auth

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

const pw = "correct horse battery"

func TestPasswordHashVerify(t *testing.T) {
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$") || strings.Contains(h, pw) {
		t.Fatalf("unexpected hash format: %q", h)
	}
	if !VerifyPassword(pw, h) || VerifyPassword(pw+"x", h) {
		t.Fatal("verify mismatch")
	}
	h2, _ := HashPassword(pw)
	if h == h2 {
		t.Fatal("salt not random")
	}
	if _, err := HashPassword("short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("want ErrWeakPassword, got %v", err)
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=999999999,t=1,p=1$AA$AA", "$argon2id$v=19$m=65536,t=3,p=2$!!$!!"} {
		if VerifyPassword(pw, bad) {
			t.Fatalf("malformed hash accepted: %q", bad)
		}
	}
}

func login(s *Service, email, pass, mid string) (string, *Principal, error) {
	return s.Login(context.Background(), LoginInput{Email: email, Password: pass, MerchantID: mid, IP: "10.0.0.1"})
}

func TestLoginSessionLifecycle(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	ctx := context.Background()
	mid, _ := testdb.SeedMerchant(t, db, "A")
	uid, err := s.CreateUser(ctx, "Owner@A.test", pw)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AddMember(t, db, mid, uid, "merchant_owner")

	if _, _, err := login(s, "owner@a.test", "wrong-password!!", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong pw: %v", err)
	}
	if _, _, err := login(s, "nobody@a.test", pw, ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown user: %v", err)
	}
	tok, p, err := login(s, "OWNER@a.test", pw, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.MerchantID != mid || !p.Has("payment:create") || p.Has("gateway:manage") {
		t.Fatalf("bad principal: %+v", p)
	}

	// Only the hash of the token is stored.
	var n int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash = $1`, []byte(tok)).Scan(&n)
	if n != 0 {
		t.Fatal("raw token stored")
	}

	// Logout revokes immediately.
	if err := s.Logout(ctx, p, "req"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked session still valid: %v", err)
	}

	// Audit trail has outcomes but never the password or the token.
	var bad int
	_ = db.QueryRow(ctx,
		`SELECT count(*) FROM audit_logs WHERE position($1 in metadata::text) > 0 OR position($2 in metadata::text) > 0`,
		pw, tok).Scan(&bad)
	if bad != 0 {
		t.Fatal("secret leaked into audit_logs")
	}
	for _, a := range []string{"auth.login.failed", "auth.login.success", "auth.logout"} {
		_ = db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = $1`, a).Scan(&n)
		if n == 0 {
			t.Fatalf("missing audit action %s", a)
		}
	}
}

func TestSessionExpiryIdleDisableAndMembershipRemoval(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	ctx := context.Background()
	mid, _ := testdb.SeedMerchant(t, db, "A")
	uid, _ := s.CreateUser(ctx, "u@a.test", pw)
	testdb.AddMember(t, db, mid, uid, "cashier")

	newSession := func() string {
		s.now = time.Now
		tok, _, err := login(s, "u@a.test", pw, "")
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	at := func(d time.Duration) { s.now = func() time.Time { return time.Now().Add(d) } }

	tok := newSession()
	at(DefaultConfig().IdleTTL + time.Minute)
	if _, err := s.Authenticate(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("idle session accepted: %v", err)
	}
	tok = newSession()
	at(DefaultConfig().SessionTTL + time.Minute)
	if _, err := s.Authenticate(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired session accepted: %v", err)
	}

	tok = newSession()
	if _, err := db.Exec(ctx, `UPDATE users SET status='disabled' WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("disabled user accepted: %v", err)
	}
	if _, _, err := login(s, "u@a.test", pw, ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled user logged in: %v", err)
	}
	_, _ = db.Exec(ctx, `UPDATE users SET status='active' WHERE id=$1`, uid)

	tok = newSession()
	if _, err := db.Exec(ctx, `DELETE FROM merchant_users WHERE user_id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("removed member accepted: %v", err)
	}
}

func TestRevokeUserSessions(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	ctx := context.Background()
	mid, _ := testdb.SeedMerchant(t, db, "A")
	uid, _ := s.CreateUser(ctx, "u@a.test", pw)
	testdb.AddMember(t, db, mid, uid, "cashier")
	t1, _, _ := login(s, "u@a.test", pw, "")
	t2, _, _ := login(s, "u@a.test", pw, "")
	if err := s.RevokeUserSessions(ctx, uid, uid, "password_reset", "req"); err != nil {
		t.Fatal(err)
	}
	for _, tk := range []string{t1, t2} {
		if _, err := s.Authenticate(ctx, tk); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("session survived revoke-all: %v", err)
		}
	}
}

func TestMerchantSelection(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	ctx := context.Background()
	a, _ := testdb.SeedMerchant(t, db, "A")
	b, _ := testdb.SeedMerchant(t, db, "B")
	c, _ := testdb.SeedMerchant(t, db, "C")
	uid, _ := s.CreateUser(ctx, "multi@x.test", pw)
	testdb.AddMember(t, db, a, uid, "merchant_owner")
	testdb.AddMember(t, db, b, uid, "cashier")

	if _, _, err := login(s, "multi@x.test", pw, ""); !errors.Is(err, ErrMerchantRequired) {
		t.Fatalf("want ErrMerchantRequired, got %v", err)
	}
	if _, _, err := login(s, "multi@x.test", pw, c); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("non-member merchant accepted: %v", err)
	}
	_, p, err := login(s, "multi@x.test", pw, b)
	if err != nil || p.MerchantID != b || p.Has("api_key:manage") {
		t.Fatalf("want cashier of B, got %+v err=%v", p, err)
	}
	_, p, err = login(s, "multi@x.test", pw, a)
	if err != nil || p.MerchantID != a || !p.Has("api_key:manage") {
		t.Fatalf("want owner of A, got %+v err=%v", p, err)
	}
}

func TestPlatformSessionHasNoTenant(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	uid, _ := s.CreateUser(context.Background(), "root@erapay.test", pw)
	testdb.AddPlatformRole(t, db, uid, "super_admin")
	_, p, err := login(s, "root@erapay.test", pw, "")
	if err != nil || p.MerchantID != "" || !p.Has("gateway:manage") {
		t.Fatalf("bad platform principal: %+v err=%v", p, err)
	}
}

// Parallel guesses must never exceed the limit, and a locked account rejects even the right password.
func TestLoginRateLimitConcurrent(t *testing.T) {
	db := testdb.New(t)
	cfg := DefaultConfig()
	s := NewService(db, cfg)
	ctx := context.Background()
	mid, _ := testdb.SeedMerchant(t, db, "A")
	uid, _ := s.CreateUser(ctx, "u@a.test", pw)
	testdb.AddMember(t, db, mid, uid, "cashier")

	const workers = 25
	var wg sync.WaitGroup
	var mu sync.Mutex
	invalid, limited := 0, 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := login(s, "u@a.test", "wrong-password-guess", "")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errors.Is(err, ErrInvalidCredentials):
				invalid++
			case errors.Is(err, ErrRateLimited):
				limited++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if invalid != cfg.MaxFailEmail || limited != workers-cfg.MaxFailEmail {
		t.Fatalf("invalid=%d limited=%d, want %d/%d", invalid, limited, cfg.MaxFailEmail, workers-cfg.MaxFailEmail)
	}
	if _, _, err := login(s, "u@a.test", pw, ""); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("correct password must not bypass lockout: %v", err)
	}
	// Window passes -> allowed again.
	s.now = func() time.Time { return time.Now().Add(cfg.AttemptWin + time.Minute) }
	if _, _, err := login(s, "u@a.test", pw, ""); err != nil {
		t.Fatalf("login after window: %v", err)
	}
}

func TestLoginRateLimitPerIP(t *testing.T) {
	db := testdb.New(t)
	cfg := DefaultConfig()
	cfg.MaxFailIP = 3
	s := NewService(db, cfg)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _, _ = s.Login(ctx, LoginInput{Email: "x" + strconv.Itoa(i) + "@x.test", Password: "wrong-password-guess", IP: "9.9.9.9"})
	}
	_, _, err := s.Login(ctx, LoginInput{Email: "fresh@x.test", Password: "wrong-password-guess", IP: "9.9.9.9"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("IP not limited: %v", err)
	}
	_, _, err = s.Login(ctx, LoginInput{Email: "fresh@x.test", Password: "wrong-password-guess", IP: "8.8.8.8"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("other IP wrongly limited: %v", err)
	}
}
