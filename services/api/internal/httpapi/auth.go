package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
)

const sessionCookie = "erapay_session"

type ctxKey struct{}

// PrincipalFrom returns the authenticated principal set by requireAuth.
func PrincipalFrom(ctx context.Context) *auth.Principal {
	p, _ := ctx.Value(ctxKey{}).(*auth.Principal)
	return p
}

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Retryable bool   `json:"retryable"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, msg string, retryable bool) {
	writeJSON(w, status, map[string]apiError{"error": {code, msg, middleware.GetReqID(r.Context()), retryable}})
}

func clientIP(r *http.Request) string {
	// RemoteAddr only: X-Forwarded-For is client-controlled unless a trusted proxy is configured.
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func tokenFrom(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// requireAuth authenticates the session and stores the Principal. Tenant identity
// comes only from here; headers/query/body merchant IDs are never read.
func (d Deps) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := d.Auth.Authenticate(r.Context(), tokenFrom(r))
		if errors.Is(err, auth.ErrUnauthenticated) {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required", false)
			return
		}
		if err != nil {
			d.Log.Error("authenticate", "err", err)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
	})
}

func requirePerm(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p := PrincipalFrom(r.Context()); p == nil || !p.Has(perm) {
				writeError(w, r, http.StatusForbidden, "FORBIDDEN", "permission denied", false)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireTenant rejects platform sessions on tenant-scoped routes.
func requireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := PrincipalFrom(r.Context()); p == nil || p.MerchantID == "" {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "merchant session required", false)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d Deps) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		MerchantID string `json:"merchant_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" || body.Password == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "email and password are required", false)
		return
	}
	token, p, err := d.Auth.Login(r.Context(), auth.LoginInput{
		Email: body.Email, Password: body.Password, MerchantID: body.MerchantID,
		IP: clientIP(r), UserAgent: r.UserAgent(), RequestID: middleware.GetReqID(r.Context()),
	})
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		w.Header().Set("Retry-After", "900")
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts, try again later", true)
		return
	case errors.Is(err, auth.ErrMerchantRequired):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "merchant_id is required for this account", false)
		return
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials", false)
		return
	case err != nil:
		d.Log.Error("login", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: int(d.Auth.SessionTTL().Seconds()),
	})
	writeJSON(w, http.StatusOK, meResponse(p))
}

func (d Deps) logout(w http.ResponseWriter, r *http.Request) {
	if err := d.Auth.Logout(r.Context(), PrincipalFrom(r.Context()), middleware.GetReqID(r.Context())); err != nil {
		d.Log.Error("logout", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func meResponse(p *auth.Principal) map[string]any {
	perms := make([]string, 0, len(p.Permissions))
	for c := range p.Permissions {
		perms = append(perms, c)
	}
	return map[string]any{"user_id": p.UserID, "merchant_id": p.MerchantID, "permissions": perms}
}

func (d Deps) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, meResponse(PrincipalFrom(r.Context())))
}

// merchantProfile and merchantOutlets read the tenant ONLY from the Principal.
func (d Deps) merchantProfile(w http.ResponseWriter, r *http.Request) {
	prof, err := d.Merchants.Profile(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if errors.Is(err, merchant.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
		return
	}
	if err != nil {
		d.Log.Error("profile", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, prof)
}

func (d Deps) merchantOutlets(w http.ResponseWriter, r *http.Request) {
	o, err := d.Merchants.Outlets(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if err != nil {
		d.Log.Error("outlets", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": o})
}

// requireJSON blocks cross-site form posts (defense in depth beside SameSite=Strict).
func requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, r, http.StatusUnsupportedMediaType, "VALIDATION_ERROR", "Content-Type must be application/json", false)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d Deps) mountV1(r chi.Router) {
	r.Route("/v1", func(r chi.Router) {
		r.Use(requireJSON)
		r.Post("/auth/login", d.login)
		if d.APIKeys != nil {
			r.With(d.requireAPIKey).Get("/api-keys/self", d.apiKeySelf)
		}
		r.Group(func(r chi.Router) {
			r.Use(d.requireAuth)
			r.Post("/auth/logout", d.logout)
			r.Get("/auth/me", d.me)
			r.Group(func(r chi.Router) {
				r.Use(requireTenant)
				r.With(requirePerm("merchant:read")).Get("/merchant/profile", d.merchantProfile)
				r.With(requirePerm("outlet:read")).Get("/merchant/outlets", d.merchantOutlets)
				if d.APIKeys != nil {
					r.Group(func(r chi.Router) {
						r.Use(requirePerm("api_key:manage"))
						r.Post("/api-keys", d.createAPIKey)
						r.Get("/api-keys", d.listAPIKeys)
						r.Post("/api-keys/{id}/revoke", d.revokeAPIKey)
						r.Post("/api-keys/{id}/rotate", d.rotateAPIKey)
					})
				}
			})
		})
	})
}
