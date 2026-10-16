package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/account"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/config"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gwaccount"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gwcred"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/httpapi"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/kyc"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/mailer"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/observability"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/onboarding"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/outbound"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payout"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/refund"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/settlement"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/webhook"
)

type redisPinger struct{ c *redis.Client }

func (r redisPinger) Ping(ctx context.Context) error { return r.c.Ping(ctx).Err() }

// kycReviewerBridge menghubungkan paket KYC ke mekanisme review onboarding yang
// sudah ada (transisi merchants.onboarding_status, verifikasi, penetapan fee).
type kycReviewerBridge struct{ onb *onboarding.Service }

func (b kycReviewerBridge) StartReview(ctx context.Context, in kyc.ReviewInput) error {
	_, err := b.onb.StartReview(ctx, onboarding.ReviewInput{
		MerchantID: in.MerchantID, ActorID: in.ActorID, RequestID: in.RequestID})
	return err
}
func (b kycReviewerBridge) RequestChanges(ctx context.Context, in kyc.ReviewInput) error {
	_, err := b.onb.RequestChanges(ctx, onboarding.ReviewInput{
		MerchantID: in.MerchantID, ActorID: in.ActorID, Note: in.Note, RequestID: in.RequestID})
	return err
}
func (b kycReviewerBridge) Reject(ctx context.Context, in kyc.RejectInput) error {
	_, err := b.onb.Reject(ctx, onboarding.RejectInput{
		MerchantID: in.MerchantID, ActorID: in.ActorID, Note: in.Note, RequestID: in.RequestID})
	return err
}
func (b kycReviewerBridge) Approve(ctx context.Context, in kyc.ApproveInput) error {
	_, err := b.onb.Approve(ctx, onboarding.ApproveInput{
		MerchantID: in.MerchantID, ActorID: in.ActorID,
		MDRBPS: in.MDRBPS, PlatformBPS: in.PlatformBPS,
		MDRFixed: in.MDRFixed, PlatformFixed: in.PlatformFixed, RequestID: in.RequestID})
	return err
}

// tunePool memberi batas koneksi yang eksplisit. Nilai bisa ditimpa lewat query string DSN
// (mis. ?pool_max_conns=40) karena ParseConfig dijalankan lebih dulu.
func tunePool(c *pgxpool.Config) {
	q := c.ConnString()
	if !strings.Contains(q, "pool_max_conns") {
		c.MaxConns = int32(max(10, 4*runtime.GOMAXPROCS(0)))
	}
	if !strings.Contains(q, "pool_min_conns") {
		c.MinConns = 2
	}
	if !strings.Contains(q, "pool_max_conn_lifetime") {
		c.MaxConnLifetime = 30 * time.Minute
	}
	if !strings.Contains(q, "pool_max_conn_idle_time") {
		c.MaxConnIdleTime = 5 * time.Minute
	}
	if !strings.Contains(q, "pool_health_check_period") {
		c.HealthCheckPeriod = time.Minute
	}
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pcfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	tunePool(pcfg)
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Start metrics collector (samples queue depths, etc.)
	go observability.StartCollector(ctx, pool, 30*time.Second)
	observability.SetReadyz(false) // will be set true by /readyz handler once deps are up

	ropts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(ropts)

	var box *secretbox.Box
	if len(cfg.EncryptionKey) > 0 {
		box, err = secretbox.New(cfg.EncryptionKey)
		if err != nil {
			return err
		}
	}
	// Gateway credentials saved in the panel need the encryption key; without it the server keeps
	// using DOKU_* from the environment, exactly as before.
	var credSvc *gwcred.Service
	if box != nil {
		credSvc = gwcred.NewService(pool, box, apikey.EnvFromApp(cfg.Env), nil, gatewayFactories())
	}
	reg, err := buildRegistry(cfg, log, credSvc)
	if err != nil {
		return err
	}
	if credSvc != nil {
		credSvc.SetLive(reg)
	}
	paySvc := payment.NewService(pool)
	flow := payment.NewOrchestrator(paySvc, pool, idempotency.NewStore(pool), reg, apikey.EnvFromApp(cfg.Env), log)
	go flow.RunPoller(ctx, 15*time.Second)
	defer rdb.Close()

	var out *outbound.Service
	var payouts *payout.Service
	if box != nil {
		out = outbound.NewService(pool, box, log)
		go out.RunDispatcher(ctx, 5*time.Second)

		dryRun := cfg.Env != "production"
		payouts = payout.NewService(pool, box, payout.Config{DryRun: dryRun, LiveEnabled: cfg.PayoutLive, MinAmount: cfg.PayoutMin,
			MaxAmount: cfg.PayoutMax, DailyLimit: cfg.PayoutDailyLimit, Cooling: cfg.PayoutCooling,
			FeeFlat: cfg.PayoutFee, CashAccount: cfg.PayoutCashAccount, ChannelCode: cfg.PayoutChannelCode, ServiceCode: cfg.PayoutServiceCode,
			Sender: payout.Sender{CustomerNumber: cfg.PayoutSenderPhone, FirstName: cfg.PayoutSenderFirstName, LastName: cfg.PayoutSenderLastName,
				PersonalID: cfg.PayoutSenderID, IDType: cfg.PayoutSenderIDType}})
		if d, ok := disburserOf(reg); ok {
			payouts.SetDisburser(d)
			log.Info("payout executor ready", "gateway", d.Code())
		}
		if !dryRun && !cfg.PayoutLive {
			log.Warn("payouts are disabled in production until ERAPAY_PAYOUT_LIVE=1 (go/no-go)")
		}
	} else {
		log.Warn("ERAPAY_ENCRYPTION_KEY not set: outbound merchant webhooks, bank accounts, payouts and self sign-up are disabled")
	}

	var hooks *webhook.Store
	if cfg.WebhookCapture {
		hooks = webhook.NewStore(pool)
		log.Warn("webhook capture enabled: raw gateway notifications are stored (contain payer PII)")
	}

	// Email is needed to prove address ownership. Without a mailer, sign-up, verification and password
	// reset stay off rather than "sending" links nowhere; development falls back to logging them.
	var mail mailer.Mailer
	webURL := cfg.WebURL
	switch {
	case cfg.SMTPHost != "":
		mail = mailer.NewSMTP(mailer.SMTPConfig{Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser, Pass: cfg.SMTPPass, From: cfg.SMTPFrom})
	case cfg.Env == "development":
		mail = mailer.Log{L: log}
		if webURL == "" {
			webURL = "http://localhost:5173"
		}
		log.Warn("ERAPAY_SMTP_HOST not set: emails (verification, password reset) are only logged in development")
	default:
		log.Warn("ERAPAY_SMTP_HOST not set: self sign-up, email verification and password reset are disabled")
	}
	var acct *account.Service
	var notifier onboarding.Notifier
	if mail != nil {
		acct = account.NewService(pool, mail, webURL, log)
		defer acct.Wait() // let queued emails finish on shutdown
		notifier = acct
	}
	if notifier != nil && box == nil {
		log.Warn("ERAPAY_ENCRYPTION_KEY not set: self sign-up stays off because the identity data (NIK, NPWP) cannot be sealed at rest")
	}

	onb := onboarding.NewService(pool, notifier, onboarding.Config{RegisterPerIP: cfg.RegisterPerIPHour, MDRCap: cfg.MDRCap, MDRCapUpTo: cfg.MDRCapUpTo, Box: box, Log: log})
	var kycSvc *kyc.Service
	if cfg.KYCStorageDir != "" && box != nil {
		kycSvc = kyc.NewService(pool, kyc.Config{
			Box: box, Storage: &kyc.LocalFS{Dir: cfg.KYCStorageDir}, Log: log,
			RecheckPolicy: cfg.KYCRecheckPolicy,
			UploadPerIP:   cfg.KYCDocUploadPerIP, SubmitPerIP: cfg.KYCSubmitPerIP,
		})
		kycSvc.SetReviewer(kycReviewerBridge{onb})
	} else {
		log.Warn("ERAPAY_KYC_STORAGE_DIR or ERAPAY_ENCRYPTION_KEY not set: /v1/kyc/* wizard is disabled")
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Webhooks: hooks, Gateways: reg, Outbound: out, Settlement: settlement.NewService(pool), Payouts: payouts, Refunds: refund.NewService(pool), Onboarding: onb, Account: acct,
			Log: log, DB: pool, Redis: redisPinger{rdb},
			Auth: auth.NewService(pool, auth.DefaultConfig()), Merchants: merchant.NewRepo(pool),
			APIKeys: apikey.NewService(pool, apikey.EnvFromApp(cfg.Env)), Payments: paySvc, PaymentFlow: flow,
			KYC:           kycSvc,
			GWAccounts:    gwaccount.NewService(pool, reg, apikey.EnvFromApp(cfg.Env)),
			GWCreds:       credSvc,
			SecureCookies: cfg.Env != "development", TrustedProxies: cfg.TrustedProxies,
			ARUSProvisionToken: cfg.ARUSProvisionToken,
			PlatformSetupToken: cfg.PlatformSetupToken,
		}),
		// Tanpa batas ini koneksi lambat (slowloris) bisa menahan goroutine tanpa batas waktu.
		// WriteTimeout > timeout gateway pada Create (30s) supaya respons pembayaran tidak terpotong.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      75 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("api started", "addr", cfg.HTTPAddr, "env", cfg.Env)

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	}
	return nil
}
