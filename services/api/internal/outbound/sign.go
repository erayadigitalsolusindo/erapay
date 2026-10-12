package outbound

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

// Signature format (documented for merchants):
//
//	X-EraPay-Signature: t=<unix seconds>,v1=<hex(HMAC-SHA256(secret, "<t>." + body))>
//
// Receivers must recompute over the raw body, compare in constant time, and reject a
// timestamp older than their tolerance (replay protection).
const SignatureHeader = "X-EraPay-Signature"

func Sign(secret string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	return "t=" + ts + ",v1=" + mac(secret, ts, body)
}

func mac(secret, ts string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(ts + "."))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

var ErrBadSignature = errors.New("outbound: signature invalid or expired")

// Verify is the reference receiver check (also used by our tests and docs).
func Verify(secret, header string, body []byte, now time.Time, tolerance time.Duration) error {
	var ts, v1 string
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts = v
		case "v1":
			v1 = v
		}
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || v1 == "" {
		return ErrBadSignature
	}
	if d := now.Sub(time.Unix(sec, 0)); d > tolerance || d < -tolerance {
		return ErrBadSignature
	}
	if !hmac.Equal([]byte(mac(secret, ts, body)), []byte(v1)) {
		return ErrBadSignature
	}
	return nil
}

// Retry policy: exponential backoff with jitter, then dead-letter.
const (
	MaxAttempts = 8
	baseBackoff = 30 * time.Second
	maxBackoff  = 6 * time.Hour
)

// Backoff is the wait after the n-th failed attempt (n >= 1); u in [0,1) adds ±20% jitter.
func Backoff(n int, u float64) time.Duration {
	if n < 1 {
		n = 1
	}
	d := maxBackoff
	if n <= 20 {
		if x := baseBackoff << (n - 1); x < maxBackoff {
			d = x
		}
	}
	return time.Duration(float64(d) * (0.8 + 0.4*u))
}

func jitter() float64 { return rand.Float64() }

// newSecret returns a signing secret shown to the merchant exactly once.
func newSecret() (string, error) {
	b, err := randBytes(32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("whsec_%x", b), nil
}
