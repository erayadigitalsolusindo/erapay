// Package config loads runtime configuration from the environment.
// Secrets come only from env vars (never from code, fixtures, or logs).
package config

import (
	"errors"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
)

type Config struct {
	Env            string // development | sandbox | production
	HTTPAddr       string
	DatabaseURL    string
	RedisURL       string
	TrustedProxies []netip.Prefix // ERAPAY_TRUSTED_PROXIES: comma-separated CIDRs
	WebhookCapture bool           // ERAPAY_WEBHOOK_CAPTURE=1: store raw inbound gateway notifications (off by default)
	// EncryptionKey (ERAPAY_ENCRYPTION_KEY: 32 bytes, hex or base64) seals secrets at rest, e.g. merchant
	// webhook signing secrets. Empty disables outbound webhooks.
	EncryptionKey []byte

	// Payout policy: PRD payout.md leaves these OPEN, so they are configuration with conservative defaults.
	PayoutLive       bool          // ERAPAY_PAYOUT_LIVE=1 allows real payouts in production (go/no-go gate)
	PayoutMin        int64         // ERAPAY_PAYOUT_MIN, minor units
	PayoutMax        int64         // ERAPAY_PAYOUT_MAX
	PayoutDailyLimit int64         // ERAPAY_PAYOUT_DAILY_LIMIT, rolling 24h per merchant
	PayoutCooling    time.Duration // ERAPAY_PAYOUT_COOLING, e.g. 24h: wait after an account is verified
}

func Load() (Config, error) {
	c := Config{
		Env:         get("ERAPAY_ENV", "development"),
		HTTPAddr:    get("ERAPAY_HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("ERAPAY_DATABASE_URL"),
		RedisURL:    os.Getenv("ERAPAY_REDIS_URL"),

		WebhookCapture: os.Getenv("ERAPAY_WEBHOOK_CAPTURE") == "1",
	}
	if c.DatabaseURL == "" {
		return c, errors.New("ERAPAY_DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return c, errors.New("ERAPAY_REDIS_URL is required")
	}
	c.PayoutLive = os.Getenv("ERAPAY_PAYOUT_LIVE") == "1"
	for _, p := range []struct {
		env string
		dst *int64
		def int64
	}{{"ERAPAY_PAYOUT_MIN", &c.PayoutMin, 10_000}, {"ERAPAY_PAYOUT_MAX", &c.PayoutMax, 50_000_000}, {"ERAPAY_PAYOUT_DAILY_LIMIT", &c.PayoutDailyLimit, 200_000_000}} {
		*p.dst = p.def
		if v := os.Getenv(p.env); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n <= 0 {
				return c, errors.New(p.env + " must be a positive integer (minor units)")
			}
			*p.dst = n
		}
	}
	c.PayoutCooling = 24 * time.Hour
	if v := os.Getenv("ERAPAY_PAYOUT_COOLING"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return c, errors.New("ERAPAY_PAYOUT_COOLING must be a duration such as 24h")
		}
		c.PayoutCooling = d
	}
	if k := os.Getenv("ERAPAY_ENCRYPTION_KEY"); k != "" {
		key, err := secretbox.ParseKey(k)
		if err != nil {
			return c, errors.New("ERAPAY_ENCRYPTION_KEY: " + err.Error())
		}
		c.EncryptionKey = key
	}
	for _, s := range strings.Split(os.Getenv("ERAPAY_TRUSTED_PROXIES"), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return c, errors.New("ERAPAY_TRUSTED_PROXIES: invalid CIDR " + s)
		}
		c.TrustedProxies = append(c.TrustedProxies, p)
	}
	return c, nil
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
