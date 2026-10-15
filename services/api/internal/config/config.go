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

	// Payout executor (Kirim DOKU, doku.md §6). The sender is the platform entity that
	// remits the funds; the gateway requires its identity on every transfer.
	PayoutFee             int64  // ERAPAY_PAYOUT_FEE: fallback transfer fee when the inquiry returns none
	PayoutCashAccount     string // ERAPAY_PAYOUT_CASH_ACCOUNT: cash_bank_pt (default) | cash_doku_clearing; [KD] source of funds
	PayoutChannelCode     string // ERAPAY_PAYOUT_CHANNEL_CODE: 07 bank deposit | 11 BI-FAST; "" = gateway default
	PayoutServiceCode     string // ERAPAY_PAYOUT_SERVICE_CODE: [?] transfer service code for check status
	PayoutSenderPhone     string // ERAPAY_PAYOUT_SENDER_PHONE, 62/0 prefix
	PayoutSenderFirstName string // ERAPAY_PAYOUT_SENDER_FIRST_NAME
	PayoutSenderLastName  string // ERAPAY_PAYOUT_SENDER_LAST_NAME
	PayoutSenderID        string // ERAPAY_PAYOUT_SENDER_ID; NPWP for a business remitter ([?])
	PayoutSenderIDType    string // ERAPAY_PAYOUT_SENDER_ID_TYPE; the spec lists KTP | PASSPORT

	// Outgoing email (sign-up verification, password reset). Without SMTP_HOST the email-sending endpoints
	// stay disabled outside development, so a link can never be "sent" into the void.
	SMTPHost, SMTPPort, SMTPUser, SMTPPass, SMTPFrom string
	WebURL                                           string // ERAPAY_WEB_URL: public base URL of the dashboard, used in email links
	RegisterPerIPHour                                int    // ERAPAY_REGISTER_PER_IP_HOUR: sign-up attempts per IP per hour
	// Optional MDR cap for plans assigned at merchant approval, e.g. the BI QRIS rule
	// (Rp500 for Rp0-Rp100.000, effective 1 Nov 2026). Both default 0 = uncapped, so fee
	// computation is unchanged until the operator opts in (docs/integrations/doku.md §6).
	MDRCap             int64  // ERAPAY_MDR_CAP, minor units
	MDRCapUpTo         int64  // ERAPAY_MDR_CAP_UPTO, minor units
	ARUSProvisionToken string // ERAPAY_ARUS_PROVISION_TOKEN: server-to-server tenant provisioning secret
	PlatformSetupToken string // ERAPAY_PLATFORM_SETUP_TOKEN (>= 32 chars): bootstrap first DEWA at /platform/setup

	// KYC wizard (data diri + dokumen) — ERAPAY_KYC_STORAGE_DIR wajib untuk mengaktifkan
	// seluruh route /v1/kyc/* (draft & edit). Tanpa penyimpanan, wizard mati total.
	KYCStorageDir  string // ERAPAY_KYC_STORAGE_DIR: direktori lokal (LocalFS) untuk dokumen & bukti
	KYCRecheckPolicy string // ERAPAY_KYC_RECHECK_POLICY: open (default) | closed — akses produksi saat re-check
	KYCDocUploadPerIP int // ERAPAY_KYC_UPLOAD_PER_IP per jam (default 60)
	KYCSubmitPerIP    int // ERAPAY_KYC_SUBMIT_PER_IP per jam (default 10)
}

func Load() (Config, error) {
	c := Config{
		Env:         get("ERAPAY_ENV", "development"),
		HTTPAddr:    get("ERAPAY_HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("ERAPAY_DATABASE_URL"),
		RedisURL:    os.Getenv("ERAPAY_REDIS_URL"),

		WebhookCapture:     os.Getenv("ERAPAY_WEBHOOK_CAPTURE") == "1",
		ARUSProvisionToken: strings.TrimSpace(os.Getenv("ERAPAY_ARUS_PROVISION_TOKEN")),
		PlatformSetupToken: strings.TrimSpace(os.Getenv("ERAPAY_PLATFORM_SETUP_TOKEN")),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("ERAPAY_DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return c, errors.New("ERAPAY_REDIS_URL is required")
	}
	if c.ARUSProvisionToken != "" && len(c.ARUSProvisionToken) < 32 {
		return c, errors.New("ERAPAY_ARUS_PROVISION_TOKEN must be at least 32 characters")
	}
	if c.PlatformSetupToken != "" && len(c.PlatformSetupToken) < 32 {
		return c, errors.New("ERAPAY_PLATFORM_SETUP_TOKEN must be at least 32 characters")
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
	c.PayoutFee = 0
	if v := os.Getenv("ERAPAY_PAYOUT_FEE"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return c, errors.New("ERAPAY_PAYOUT_FEE must be a non-negative integer (minor units)")
		}
		c.PayoutFee = n
	}
	c.PayoutCashAccount = os.Getenv("ERAPAY_PAYOUT_CASH_ACCOUNT")
	switch c.PayoutCashAccount {
	case "", "cash_bank_pt", "cash_doku_clearing":
	default:
		return c, errors.New("ERAPAY_PAYOUT_CASH_ACCOUNT must be cash_bank_pt or cash_doku_clearing")
	}
	c.PayoutChannelCode = os.Getenv("ERAPAY_PAYOUT_CHANNEL_CODE")
	switch c.PayoutChannelCode {
	case "", "07", "11":
	default:
		return c, errors.New("ERAPAY_PAYOUT_CHANNEL_CODE must be 07 or 11")
	}
	c.PayoutServiceCode = os.Getenv("ERAPAY_PAYOUT_SERVICE_CODE")
	c.PayoutSenderPhone = os.Getenv("ERAPAY_PAYOUT_SENDER_PHONE")
	c.PayoutSenderFirstName = os.Getenv("ERAPAY_PAYOUT_SENDER_FIRST_NAME")
	c.PayoutSenderLastName = os.Getenv("ERAPAY_PAYOUT_SENDER_LAST_NAME")
	c.PayoutSenderID = os.Getenv("ERAPAY_PAYOUT_SENDER_ID")
	c.PayoutSenderIDType = os.Getenv("ERAPAY_PAYOUT_SENDER_ID_TYPE")
	for _, e := range []struct {
		env string
		dst *int64
	}{
		{"ERAPAY_MDR_CAP", &c.MDRCap},
		{"ERAPAY_MDR_CAP_UPTO", &c.MDRCapUpTo},
	} {
		if v := os.Getenv(e.env); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				return c, errors.New(e.env + " must be a non-negative integer (minor units)")
			}
			*e.dst = n
		}
	}
	if c.MDRCapUpTo > 0 && c.MDRCap == 0 {
		return c, errors.New("ERAPAY_MDR_CAP_UPTO requires ERAPAY_MDR_CAP > 0")
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
	c.KYCStorageDir = strings.TrimSpace(os.Getenv("ERAPAY_KYC_STORAGE_DIR"))
	c.KYCRecheckPolicy = strings.ToLower(strings.TrimSpace(os.Getenv("ERAPAY_KYC_RECHECK_POLICY")))
	switch c.KYCRecheckPolicy {
	case "", "open":
		c.KYCRecheckPolicy = "open"
	case "closed":
	default:
		return c, errors.New("ERAPAY_KYC_RECHECK_POLICY must be open or closed")
	}
	if v := os.Getenv("ERAPAY_KYC_UPLOAD_PER_IP"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, errors.New("ERAPAY_KYC_UPLOAD_PER_IP must be a positive integer")
		}
		c.KYCDocUploadPerIP = n
	}
	if v := os.Getenv("ERAPAY_KYC_SUBMIT_PER_IP"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, errors.New("ERAPAY_KYC_SUBMIT_PER_IP must be a positive integer")
		}
		c.KYCSubmitPerIP = n
	}
	return c, nil
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
