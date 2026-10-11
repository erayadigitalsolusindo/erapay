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
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
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
	defer rdb.Close()

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			Log: log, DB: pool, Redis: redisPinger{rdb},
			Auth: auth.NewService(pool, auth.DefaultConfig()), Merchants: merchant.NewRepo(pool),
			APIKeys: apikey.NewService(pool, apikey.EnvFromApp(cfg.Env)),
			SecureCookies: cfg.Env != "development",
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
