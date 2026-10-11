package httpapi

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	trusted := Deps{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}
	cases := []struct {
		name, remote, xff, want string
		d                       Deps
	}{
		{"no proxies configured ignores XFF", "10.1.1.1:5000", "6.6.6.6", "10.1.1.1", Deps{}},
		{"untrusted peer cannot spoof", "10.1.1.1:5000", "6.6.6.6", "10.1.1.1", trusted},
		{"trusted proxy: use appended entry", "127.0.0.1:5000", "6.6.6.6, 203.0.113.9", "203.0.113.9", trusted},
		{"trusted proxy without XFF", "127.0.0.1:5000", "", "127.0.0.1", trusted},
		{"garbage XFF falls back to peer", "127.0.0.1:5000", "not-an-ip", "127.0.0.1", trusted},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := c.d.clientIP(r); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}
