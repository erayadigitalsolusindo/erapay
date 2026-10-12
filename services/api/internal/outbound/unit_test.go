package outbound

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
)

func TestSignVerify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	body := []byte(`{"id":"e1"}`)
	h := Sign("whsec_x", now, body)
	if err := Verify("whsec_x", h, body, now.Add(2*time.Minute), 5*time.Minute); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	for name, c := range map[string]struct {
		secret string
		body   []byte
		at     time.Time
	}{
		"wrong secret":  {"whsec_y", body, now},
		"tampered body": {"whsec_x", []byte(`{"id":"e2"}`), now},
		"replay (old)":  {"whsec_x", body, now.Add(10 * time.Minute)},
		"future":        {"whsec_x", body, now.Add(-10 * time.Minute)},
	} {
		if err := Verify(c.secret, h, c.body, c.at, 5*time.Minute); !errors.Is(err, ErrBadSignature) {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
	for _, bad := range []string{"", "t=1", "v1=abc", "t=abc,v1=abc"} {
		if Verify("whsec_x", bad, body, now, time.Hour) == nil {
			t.Errorf("malformed header %q accepted", bad)
		}
	}
}

func TestBackoffGrowsCapsAndJitters(t *testing.T) {
	prev := time.Duration(0)
	for n := 1; n <= 6; n++ {
		d := Backoff(n, 0.5) // u=0.5 -> factor 1.0
		if d <= prev {
			t.Fatalf("attempt %d: %v not greater than %v", n, d, prev)
		}
		prev = d
	}
	if got := Backoff(1, 0.5); got != 30*time.Second {
		t.Fatalf("first backoff = %v", got)
	}
	if got := Backoff(100, 1); got > time.Duration(1.2*float64(maxBackoff)) {
		t.Fatalf("not capped: %v", got)
	}
	if lo, hi := Backoff(3, 0), Backoff(3, 0.999); lo >= hi || lo < 96*time.Second || hi > 144*time.Second {
		t.Fatalf("jitter range lo=%v hi=%v", lo, hi)
	}
}

func TestBlockedIP(t *testing.T) {
	for _, s := range []string{
		"127.0.0.1", "::1", "10.0.0.5", "172.16.3.4", "192.168.1.1", "169.254.169.254", "fe80::1", "fd00::1",
		"0.0.0.0", "::", "100.64.0.1", "224.0.0.1", "ff02::1", "198.18.0.1", "240.0.0.1", "::ffff:127.0.0.1", "::ffff:10.1.2.3", "64:ff9b::7f00:1",
	} {
		if !blockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s must be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "203.0.114.9", "2606:4700:4700::1111"} {
		if blockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s must be allowed", s)
		}
	}
}

type fakeResolver map[string][]string

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, s := range f[host] {
		out = append(out, netip.MustParseAddr(s))
	}
	if out == nil {
		return nil, errors.New("no such host")
	}
	return out, nil
}

func TestValidateURL(t *testing.T) {
	res := fakeResolver{
		"merchant.example": {"93.184.216.34"},
		"internal.evil":    {"10.0.0.8"},
		"mixed.evil":       {"93.184.216.34", "169.254.169.254"},
	}
	ctx := context.Background()
	ok := []string{"https://merchant.example/hooks/erapay", "https://merchant.example:8443/x", "https://93.184.216.34/x"}
	for _, u := range ok {
		if err := ValidateURL(ctx, u, res, false); err != nil {
			t.Errorf("%s rejected: %v", u, err)
		}
	}
	bad := []string{
		"http://merchant.example/x",          // not https
		"https://user:pw@merchant.example/x", // credentials
		"https://merchant.example/x#frag",    // fragment
		"https://localhost/x", "https://127.0.0.1/x", "https://[::1]/x", "https://10.1.1.1/x",
		"https://169.254.169.254/latest/meta-data", // cloud metadata
		"https://internal.evil/x",                  // resolves private
		"https://mixed.evil/x",                     // ONE private answer is enough to refuse
		"https://nxdomain.example/x",               // does not resolve
		"https://printer.local/x", "ftp://merchant.example/x", "https://", "not a url", "https://merchant.example:99999/x",
	}
	for _, u := range bad {
		if err := ValidateURL(ctx, u, res, false); err == nil {
			t.Errorf("%s accepted", u)
		}
	}
}

// The dialer re-checks the address actually connected to, independent of ValidateURL (DNS rebinding),
// and redirects are never followed.
func TestSafeClientRefusesLoopbackAndRedirects(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hit = true }))
	defer srv.Close()
	if _, err := safeClient(2*time.Second, false).Get(srv.URL); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("loopback dial not blocked: %v", err)
	}
	if hit {
		t.Fatal("request reached a blocked address")
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hit = true }))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redir.Close()
	resp, err := safeClient(2*time.Second, true).Get(redir.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || hit {
		t.Fatalf("redirect followed: status=%d hit=%v", resp.StatusCode, hit)
	}
}

func TestSecretboxRoundTripAndBinding(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	b, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	sealed, _ := b.Seal([]byte("whsec_abc"), []byte("ep-1"))
	if bytes.Contains(sealed, []byte("whsec_abc")) {
		t.Fatal("plaintext visible in ciphertext")
	}
	if p, err := b.Open(sealed, []byte("ep-1")); err != nil || string(p) != "whsec_abc" {
		t.Fatalf("open: %q %v", p, err)
	}
	if _, err := b.Open(sealed, []byte("ep-2")); err == nil {
		t.Fatal("ciphertext opened under a different endpoint id")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := b.Open(sealed, []byte("ep-1")); err == nil {
		t.Fatal("tampered ciphertext opened")
	}
	if _, err := secretbox.ParseKey("short"); err == nil {
		t.Fatal("short key accepted")
	}
}
