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
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apprelease"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/config"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	qrisgategw "github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/qrisgate"
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
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/platformsetting"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/qrisgate"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/refund"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/routing"
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
	settingsSvc := platformsetting.NewService(pool)
	qgSvc := qrisgate.NewService(pool, settingsSvc, nil)
	if box != nil {
		// QRISGate keys are per tenant (encrypted in qrisgate_accounts), so it needs the encryption key. Registered
		// always; a merchant is only routed to it once DEWA activates their gateway account (the per-merchant flag).
		qgSvc.SetBox(box)
		qg, err := qrisgategw.New(qrisgategw.Config{Creds: qgSvc})
		if err != nil {
			return err
		}
		reg.Register(qg)
	}
	if credSvc != nil {
		credSvc.SetLive(reg)
	}
	// A production server also serves sandbox merchants, from their own adapters.
	var sbxReg *gateway.Registry
	if cfg.Env == "production" && (credSvc != nil || cfg.SandboxFake) {
		sbxReg = buildSandboxRegistry(log, credSvc, cfg.SandboxFake)
		if credSvc != nil {
			credSvc.SetSandbox(sbxReg)
		}
	}
	merchants := merchant.NewRepo(pool)
	merchants.SetFakeGateway(cfg.SandboxFake && sbxReg != nil)
	paySvc := payment.NewService(pool)
	payment.QRImageURL = func(sbx bool, code, ref string) string {
		r := reg
		if sbx {
			r = sbxReg
		}
		if r == nil {
			return ""
		}
		if q, ok := r.Get(code); ok {
			if u, ok := q.(interface{ QRImageURL(string) string }); ok {
				return u.QRImageURL(ref)
			}
		}
		return ""
	}
	flow := payment.NewOrchestrator(paySvc, pool, idempotency.NewStore(pool), reg, apikey.EnvFromApp(cfg.Env), log)
	if sbxReg != nil {
		flow.SetSandbox(sbxReg)
	}
	qgSvc.SetPayer(flow)
	var appReleases *apprelease.Service
	if cfg.APKStorageDir != "" {
		if appReleases, err = apprelease.NewService(pool, cfg.APKStorageDir); err != nil {
			return err
		}
	}
	routingSvc := routing.NewService(pool, reg, apikey.EnvFromApp(cfg.Env))
	flow.SetRouting(routingSvc)
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

	gwAccounts := gwaccount.NewService(pool, reg, apikey.EnvFromApp(cfg.Env))
	if sbxReg != nil {
		gwAccounts.SetSandbox(sbxReg)
	}

	var sbxGateways payment.Gateways // stays a true nil interface when there is no sandbox registry
	if sbxReg != nil {
		sbxGateways = sbxReg
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
			Webhooks: hooks, Gateways: reg, SandboxGateways: sbxGateways, Outbound: out, Settlement: settlement.NewService(pool), Payouts: payouts, Refunds: refund.NewService(pool), Onboarding: onb, Account: acct,
			Log: log, DB: pool, Redis: redisPinger{rdb},
			Auth: auth.NewService(pool, auth.DefaultConfig()), Merchants: merchants,
			APIKeys: apikey.NewService(pool, apikey.EnvFromApp(cfg.Env)), Payments: paySvc, PaymentFlow: flow,
			KYC:           kycSvc,
			GWAccounts:    gwAccounts,
			GWCreds:       credSvc,
			Routing:       routingSvc,
			Settings:      settingsSvc,
			QRISGate:      qgSvc,
			AppReleases:   appReleases,
			SecureCookies: cfg.Env != "development", TrustedProxies: cfg.TrustedProxies,
			ARUSProvisionToken: cfg.ARUSProvisionToken, ARUSKeyToken: cfg.ARUSKeyToken,
			WebURL:             cfg.WebURL,
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
