// Package httpapi wires the public HTTP API.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Pinger is satisfied by pgx pools and redis clients' wrappers.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	Log   *slog.Logger
	DB    Pinger
	Redis Pinger
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID) // correlation ID on every request
	r.Use(middleware.Recoverer)
	r.Use(accessLog(d.Log))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		checks := map[string]string{"postgres": "ok", "redis": "ok"}
		code := http.StatusOK
		if err := d.DB.Ping(ctx); err != nil {
			checks["postgres"], code = "down", http.StatusServiceUnavailable
		}
		if err := d.Redis.Ping(ctx); err != nil {
			checks["redis"], code = "down", http.StatusServiceUnavailable
		}
		writeJSON(w, code, checks)
	})
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
