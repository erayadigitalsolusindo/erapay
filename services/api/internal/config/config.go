// Package config loads runtime configuration from the environment.
// Secrets come only from env vars (never from code, fixtures, or logs).
package config

import (
	"errors"
	"net/netip"
	"net/url"
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

	// Outgoing email (sign-up verification, password reset). Without SMTP_HOST the email-sending endpoints
	// stay disabled outside development, so a link can never be "sent" into the void.
	SMTPHost, SMTPPort, SMTPUser, SMTPPass, SMTPFrom string
	WebURL                                           string // ERAPAY_WEB_URL: public base URL of the dashboard, used in email links
	RegisterPerIPHour                                int    // ERAPAY_REGISTER_PER_IP_HOUR: sign-up attempts per IP per hour
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
	c.SMTPHost, c.SMTPPort, c.SMTPUser, c.SMTPPass, c.SMTPFrom =
		os.Getenv("ERAPAY_SMTP_HOST"), get("ERAPAY_SMTP_PORT", "587"), os.Getenv("ERAPAY_SMTP_USER"), os.Getenv("ERAPAY_SMTP_PASSWORD"), os.Getenv("ERAPAY_SMTP_FROM")
	c.WebURL = strings.TrimRight(strings.TrimSpace(os.Getenv("ERAPAY_WEB_URL")), "/")
	if c.WebURL != "" {
		u, err := url.Parse(c.WebURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || (c.Env == "production" && u.Scheme != "https") {
			return c, errors.New("ERAPAY_WEB_URL must be an absolute URL (https in production)")
		}
	}
	if c.SMTPHost != "" && (c.SMTPFrom == "" || c.WebURL == "") {
		return c, errors.New("ERAPAY_SMTP_FROM and ERAPAY_WEB_URL are required when ERAPAY_SMTP_HOST is set")
	}
	c.RegisterPerIPHour = 5
	if v := os.Getenv("ERAPAY_REGISTER_PER_IP_HOUR"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, errors.New("ERAPAY_REGISTER_PER_IP_HOUR must be a positive integer")
		}
		c.RegisterPerIPHour = n
	}
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
