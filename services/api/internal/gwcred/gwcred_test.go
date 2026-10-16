package gwcred

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

const testPEM = "-----BEGIN PRIVATE KEY-----\nMIIBVQIBADANBgkqhkiG9w0BAQEFAASCAT8wggE7AgEAAkEA\n-----END PRIVATE KEY-----"

type fakeAd struct {
	gateway.Adapter
	ok   bool
	user string
}

func (f *fakeAd) Code() string { return "doku" }
func (f *fakeAd) TestConnection(context.Context, string, bool) []gateway.ConnectionStep {
	return []gateway.ConnectionStep{{Name: "credentials", OK: f.ok}}
}

type liveReg struct {
	mu  sync.Mutex
	cur gateway.Adapter
	n   int
}

func (l *liveReg) Replace(a gateway.Adapter) { l.mu.Lock(); l.cur = a; l.n++; l.mu.Unlock() }

func newSvc(t *testing.T, testOK bool) (*Service, *liveReg) {
	t.Helper()
	db := testdb.New(t)
	key := make([]byte, 32)
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	live := &liveReg{cur: &fakeAd{ok: true, user: "old"}}
	fac := func(c map[string]string, env string) (gateway.Adapter, error) {
		return &fakeAd{ok: testOK, user: c["client_id"]}, nil
	}
	return NewService(db, box, "sandbox", live, map[string]Factory{"doku": fac}), live
}

func full() map[string]string {
	return map[string]string{"client_id": "BRN-1", "secret_key": "SK-supersecret-12345", "private_key": testPEM,
		"terminal_id": "T12345", "postal_code": "12345"}
}

func setIn(v map[string]string, ver int) SetInput {
	return SetInput{Gateway: "doku", Environment: "sandbox", Values: v, ExpectVersion: ver, Note: "uji"}
}

func TestSetStoresEncryptedAndMasksSecrets(t *testing.T) {
	s, _ := newSvc(t, true)
	ctx := context.Background()
	st, err := s.Set(ctx, setIn(full(), 0))
	if err != nil {
		t.Fatal(err)
	}
	if !st.Complete || st.Version != 1 {
		t.Fatalf("%+v", st)
	}
	for _, f := range st.Fields {
		if f.Secret && strings.Contains(f.Preview, "supersecret") || f.Key == "private_key" && f.Preview != "••••" {
			t.Fatalf("secret leaked in preview: %+v", f)
		}
	}
	var raw []byte
	if err := s.db.QueryRow(ctx, `SELECT sealed FROM gateway_credentials WHERE gateway='doku' AND environment='sandbox'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "supersecret") || strings.Contains(string(raw), "BRN-1") {
		t.Fatal("credentials stored in plaintext")
	}
	// Row copied to another environment must not open (AAD binds gateway/env).
	if _, err := s.box.Open(raw, aad("doku", "production")); err == nil {
		t.Fatal("sealed value opened under a different AAD")
	}
	// Audit has field names, never values.
	var md string
	if err := s.db.QueryRow(ctx, `SELECT metadata::text FROM audit_logs WHERE action='gateway_credentials.set'`).Scan(&md); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(md, "supersecret") || strings.Contains(md, "BRN-1") || !strings.Contains(md, "secret_key") {
		t.Fatalf("audit metadata: %s", md)
	}
}

func TestSetMergeKeepsUnsentAndClears(t *testing.T) {
	s, _ := newSvc(t, true)
	ctx := context.Background()
	v := full()
	v["base_url"] = "https://api-sandbox.doku.com"
	if _, err := s.Set(ctx, setIn(v, 0)); err != nil {
		t.Fatal(err)
	}
	// Only rotate the secret; blank fields keep their stored value.
	st, err := s.Set(ctx, setIn(map[string]string{"secret_key": "NEW-secret-98765", "client_id": ""}, 1))
	if err != nil || st.Version != 2 || !st.Complete {
		t.Fatalf("%+v %v", st, err)
	}
	vals, ok, err := s.Load(ctx, "doku", "sandbox")
	if err != nil || !ok || vals["client_id"] != "BRN-1" || vals["secret_key"] != "NEW-secret-98765" {
		t.Fatalf("%v %v %v", vals, ok, err)
	}
	in := setIn(nil, 2)
	in.Clear = []string{"base_url"}
	if st, err = s.Set(ctx, in); err != nil {
		t.Fatal(err)
	}
	if vals, _, _ = s.Load(ctx, "doku", "sandbox"); vals["base_url"] != "" {
		t.Fatal("base_url not cleared")
	}
	in = setIn(nil, 3)
	in.Clear = []string{"client_id"}
	var ve *ValidationError
	if _, err := s.Set(ctx, in); !errors.As(err, &ve) {
		t.Fatalf("clearing a required field: %v", err)
	}
}

func TestSetValidation(t *testing.T) {
	s, _ := newSvc(t, true)
	ctx := context.Background()
	cases := map[string]map[string]string{
		"unknown key (cannot become an env editor)": {"DATABASE_URL": "postgres://x"},
		"bad client id":                             {"client_id": "a b;c"},
		"base url off the allowlist":                {"base_url": "https://evil.example.com"},
		"private key not pem":                       {"private_key": "hello"},
		"short terminal":                            {"terminal_id": "x"},
		"bad postal":                                {"postal_code": "ABCDE"},
	}
	for name, vals := range cases {
		var ve *ValidationError
		if _, err := s.Set(ctx, setIn(vals, 0)); !errors.As(err, &ve) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	in := setIn(full(), 0)
	in.Note = "x"
	var ve *ValidationError
	if _, err := s.Set(ctx, in); !errors.As(err, &ve) {
		t.Errorf("short note: %v", err)
	}
	if _, err := s.Set(ctx, SetInput{Gateway: "midtrans", Environment: "sandbox", Note: "uji"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown gateway: %v", err)
	}
	if _, err := s.Set(ctx, SetInput{Gateway: "doku", Environment: "staging", Note: "uji"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown env: %v", err)
	}
}

// Two admins save from the same loaded form: exactly one wins, the other gets a conflict.
func TestSetConcurrentSameVersion(t *testing.T) {
	s, _ := newSvc(t, true)
	ctx := context.Background()
	var okN, conflictN atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Set(ctx, setIn(full(), 0))
			switch {
			case err == nil:
				okN.Add(1)
			case errors.Is(err, ErrConflict):
				conflictN.Add(1)
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if okN.Load() != 1 || conflictN.Load() != 7 {
		t.Fatalf("ok=%d conflict=%d", okN.Load(), conflictN.Load())
	}
	if st, _ := s.Status(ctx, "doku", "sandbox"); st.Version != 1 {
		t.Fatalf("version = %d", st.Version)
	}
}

// Replaying a request (same expected version) never applies twice.
func TestSetReplayIsRejected(t *testing.T) {
	s, _ := newSvc(t, true)
	ctx := context.Background()
	if _, err := s.Set(ctx, setIn(full(), 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set(ctx, setIn(full(), 0)); !errors.Is(err, ErrConflict) {
		t.Fatalf("replay: %v", err)
	}
}

func TestApplySwapsOnlyWhenTestPasses(t *testing.T) {
	ctx := context.Background()
	act := ActInput{Gateway: "doku", Environment: "sandbox"}

	s, live := newSvc(t, true)
	if _, _, err := s.Apply(ctx, act); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("apply before saving: %v", err)
	}
	if _, err := s.Set(ctx, setIn(full(), 0)); err != nil {
		t.Fatal(err)
	}
	res, applied, err := s.Apply(ctx, act)
	if err != nil || !applied || !res.OK || live.cur.(*fakeAd).user != "BRN-1" {
		t.Fatalf("%+v %v %v %+v", res, applied, err, live.cur)
	}
	if _, _, err := s.Apply(ctx, ActInput{Gateway: "doku", Environment: "production"}); !errors.Is(err, ErrWrongEnv) {
		t.Fatalf("wrong env: %v", err)
	}

	// A candidate that fails the connection test must leave the working adapter untouched.
	s2, live2 := newSvc(t, false)
	if _, err := s2.Set(ctx, setIn(full(), 0)); err != nil {
		t.Fatal(err)
	}
	res, applied, err = s2.Apply(ctx, act)
	if err != nil || applied || res.OK || live2.n != 0 || live2.cur.(*fakeAd).user != "old" {
		t.Fatalf("%+v %v %v n=%d", res, applied, err, live2.n)
	}
}
