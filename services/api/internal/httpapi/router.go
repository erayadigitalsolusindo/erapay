// Package httpapi wires the public HTTP API.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/account"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/observability"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/kyc"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/onboarding"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/outbound"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payout"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/refund"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/settlement"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/webhook"
)

// Pinger is satisfied by pgx pools and redis clients' wrappers.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	Auth               *auth.Service // nil disables /v1 routes (health-only tests)
	Merchants          *merchant.Repo
	APIKeys            *apikey.Service
	Payments           *payment.Service
	PaymentFlow        *payment.Orchestrator
	Outbound           *outbound.Service   // nil disables /v1/webhook-endpoints and /v1/webhook-deliveries
	Settlement         *settlement.Service // nil disables /v1/balance and /v1/admin/settlement routes
	Payouts            *payout.Service     // nil disables bank accounts and payouts
	Refunds            *refund.Service     // nil disables refunds
	Onboarding         *onboarding.Service // nil disables /v1/admin/merchants; sign-up also needs its notifier
	KYC                *kyc.Service        // nil disables /v1/kyc/* and /v1/admin/merchants/{id}/kyc
	Account            *account.Service    // nil disables email verification and password reset
	ARUSProvisionToken string              // server-to-server token for ARUS tenant provisioning
	PlatformSetupToken string              // ERAPAY_PLATFORM_SETUP_TOKEN (>= 32 chars) enables /v1/platform/setup until the first DEWA exists
	Webhooks           *webhook.Store      // optional raw capture (ERAPAY_WEBHOOK_CAPTURE); see webhooks.go
	Gateways           payment.Gateways    // adapters implementing gateway.WebhookVerifier enable verified webhooks
	SecureCookies      bool
	TrustedProxies     []netip.Prefix // peers allowed to set X-Forwarded-For
	Log                *slog.Logger
	DB                 Pinger
	Redis              Pinger
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID) // correlation ID on every request
	r.Use(middleware.Recoverer)
	r.Use(accessLog(d.Log))
	r.Use(instrument(d.Log))

	// Liveness: process is running (no dependency checks, never 503).
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Readiness: hard dependencies (postgres, redis). Used by k8s/aaPanel health check.
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		checks := map[string]string{"postgres": "ok", "redis": "ok"}
		code := http.StatusOK
		if d.DB != nil {
			if err := d.DB.Ping(ctx); err != nil {
				checks["postgres"], code = "down", http.StatusServiceUnavailable
			}
		} else {
			checks["postgres"], code = "down", http.StatusServiceUnavailable
		}
		if d.Redis != nil {
			if err := d.Redis.Ping(ctx); err != nil {
				checks["redis"], code = "down", http.StatusServiceUnavailable
			}
		} else {
			checks["redis"], code = "down", http.StatusServiceUnavailable
		}
		observability.SetReadyz(code == http.StatusOK)
		writeJSON(w, code, checks)
	})
	// Prometheus scrape endpoint (no auth; expose only inside trusted network in production).
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(observability.Gather()))
	})
	if d.Auth != nil {
		d.mountV1(r)
	}
	if d.KYC != nil {
		// KYC dipasang pada rute /v1 terpisah: endpoint unggah (dokumen/logo/bukti)
		// memakai multipart/form-data sehingga tidak boleh berada di bawah requireJSON.
		r.Route("/v1", func(r chi.Router) { d.mountKYC(r) })
	}
	if d.ARUSProvisionToken != "" {
		r.Route("/v1/integrations/arus", func(ir chi.Router) {
			ir.Use(d.requireARUSProvisionToken)
			ir.Get("/tenants/{tenant_id}", d.arusTenantStatus)
			ir.Post("/tenants", d.provisionARUSTenant)
		})
	}
	if d.Webhooks != nil || (d.Gateways != nil && d.PaymentFlow != nil) {
		d.mountWebhooks(r)
	}
	return r
}

func accessLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			log.Info("http",
				"request_id", middleware.GetReqID(r.Context()),
				"method", r.Method, "path", r.URL.Path,
				"status", ww.Status(), "ms", time.Since(start).Milliseconds())
		})
	}
}

// instrument records Prometheus-style metrics for every request except /healthz, /readyz, /metrics.
func instrument(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			code := strconv.Itoa(ww.Status())
			observability.ObserveHTTP(r.Method, routePattern(r), code, time.Since(start))
			if ww.Status() >= 500 {
				log.Error("http 5xx", "method", r.Method, "route", routePattern(r), "status", ww.Status(), "request_id", middleware.GetReqID(r.Context()))
			}
		})
	}
}

func routePattern(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
		return rc.RoutePattern()
	}
	return r.URL.Path
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
