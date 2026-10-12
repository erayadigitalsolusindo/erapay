package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

var (
	ErrBadURL         = errors.New("outbound: invalid webhook url")
	ErrBlockedAddress = errors.New("outbound: destination address is not allowed")
)

var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", // reserved / benchmarking / documentation
	"64:ff9b::/96", "2001:db8::/32", // NAT64, documentation
)

func mustPrefixes(s ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(s))
	for i, p := range s {
		out[i] = netip.MustParsePrefix(p)
	}
	return out
}

// blockedIP is true for anything that is not a public unicast address: loopback, private,
// link-local (incl. the 169.254.169.254 metadata endpoint), multicast, unspecified, CGNAT, reserved.
func blockedIP(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Resolver is net.Resolver's LookupNetIP; injectable for tests.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// ValidateURL checks a merchant-supplied endpoint: https, no credentials, a hostname that
// resolves ONLY to public addresses. It runs at registration and again before each send; the
// dialer in safeClient re-checks the address actually connected to (defeats DNS rebinding).
// allowPrivate relaxes only the address and scheme checks, and exists for tests.
func ValidateURL(ctx context.Context, raw string, res Resolver, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || len(raw) > 2048 {
		return fmt.Errorf("%w: malformed", ErrBadURL)
	}
	if u.Scheme != "https" && !(allowPrivate && u.Scheme == "http") {
		return fmt.Errorf("%w: https is required", ErrBadURL)
	}
	host := u.Hostname()
	if host == "" || strings.HasSuffix(strings.ToLower(host), ".local") || strings.EqualFold(host, "localhost") && !allowPrivate {
		return fmt.Errorf("%w: host not allowed", ErrBadURL)
	}
	if p := u.Port(); p != "" {
		var n int
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%w: bad port", ErrBadURL)
		}
	}
	if allowPrivate {
		return nil
	}
	if a, err := netip.ParseAddr(host); err == nil {
		if blockedIP(a) {
			return ErrBlockedAddress
		}
		return nil
	}
	if res == nil {
		res = net.DefaultResolver
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addrs, err := res.LookupNetIP(rctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return fmt.Errorf("%w: host does not resolve", ErrBadURL)
	}
	for _, a := range addrs {
		if blockedIP(a) {
			return ErrBlockedAddress
		}
	}
	return nil
}

// safeClient never follows redirects (a 3xx can point at an internal address) and refuses to
// connect to a blocked address even if DNS changed after ValidateURL. It ignores proxy env vars.
func safeClient(timeout time.Duration, allowPrivate bool) *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || blockedIP(ap.Addr()) {
				return ErrBlockedAddress
			}
			return nil
		}
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           d.DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: timeout,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
