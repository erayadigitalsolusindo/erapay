package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/config"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/httpapi"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
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

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	ropts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(ropts)

	reg, err := buildRegistry(cfg, log)
	if err != nil {
		return err
	}
	paySvc := payment.NewService(pool)
	flow := payment.NewOrchestrator(paySvc, pool, idempotency.NewStore(pool), reg, apikey.EnvFromApp(cfg.Env), log)
	go flow.RunPoller(ctx, 15*time.Second)
	defer rdb.Close()

	var out *outbound.Service
	var payouts *payout.Service
	if len(cfg.EncryptionKey) > 0 {
		box, err := secretbox.New(cfg.EncryptionKey)
		if err != nil {
			return err
		}
		out = outbound.NewService(pool, box, log)
		go out.RunDispatcher(ctx, 5*time.Second)

		dryRun := cfg.Env != "production"
		payouts = payout.NewService(pool, box, payout.Config{DryRun: dryRun, LiveEnabled: cfg.PayoutLive, MinAmount: cfg.PayoutMin,
			MaxAmount: cfg.PayoutMax, DailyLimit: cfg.PayoutDailyLimit, Cooling: cfg.PayoutCooling})
		if !dryRun && !cfg.PayoutLive {
			log.Warn("payouts are disabled in production until ERAPAY_PAYOUT_LIVE=1 (go/no-go)")
		}
	} else {
		log.Warn("ERAPAY_ENCRYPTION_KEY not set: outbound merchant webhooks, bank accounts and payouts are disabled")
	}

	var hooks *webhook.Store
	if cfg.WebhookCapture {
		hooks = webhook.NewStore(pool)
		log.Warn("webhook capture enabled: raw gateway notifications are stored (contain payer PII)")
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Webhooks: hooks, Gateways: reg, Outbound: out, Settlement: settlement.NewService(pool), Payouts: payouts, Refunds: refund.NewService(pool),
			Log: log, DB: pool, Redis: redisPinger{rdb},
			Auth: auth.NewService(pool, auth.DefaultConfig()), Merchants: merchant.NewRepo(pool),
			APIKeys: apikey.NewService(pool, apikey.EnvFromApp(cfg.Env)), Payments: paySvc, PaymentFlow: flow,
			SecureCookies: cfg.Env != "development", TrustedProxies: cfg.TrustedProxies,
		}),
		ReadHeaderTimeout: 5 * time.Second,
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
