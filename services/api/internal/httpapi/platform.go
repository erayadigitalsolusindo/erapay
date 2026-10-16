package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
)

// Panel platform khusus DEWA (aciraba-style: /platform). DEWA = Platform Admin EraPay:
// cookie & jalur auth terpisah (erapay_platform_session, Path=/platform) supaya sesi
// panel tidak bertukar dengan dashboard merchant/staf. RAIN popularity:
// - Rute /v1/platform/* hanya bisa dipakai akun ber-permission rbac:manage (DEWA), tanpa merchant.
// - Bootstrap pertama via ERAPAY_PLATFORM_SETUP_TOKEN (lihat prd/platform-panel.md).
const (
	platformCookie     = "erapay_platform_session"
	platformCookiePath = "/platform"
)

func platformTokenFrom(r *http.Request) string {
	if c, err := r.Cookie(platformCookie); err == nil {
		return c.Value
	}
	return ""
}

// requirePlatformAdmin authenticates the platform session and stores the
// Principal. Only a DEWA (rbac:manage, no merchant) passes; merchant sessions and
// other platform staff (super_admin/finance/ops) are rejected here.
func (d Deps) requirePlatformAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := d.Auth.Authenticate(r.Context(), platformTokenFrom(r))
		if errors.Is(err, auth.ErrUnauthenticated) || p == nil {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "platform authentication required", false)
			return
		}
		if err != nil {
			d.Log.Error("platform.authenticate", "err", err)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
			return
		}
		if p.MerchantID != "" || !p.Has("rbac:manage") {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "platform admin (DEWA) required", false)
			return
		}
		next.ServeHTTP(w, r.WithContext(contextWithPrincipal(r, p)))
	})
}

// setPlatformSession writes the platform cookie (panel-scoped and httpOnly).
func (d Deps) setPlatformSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: platformCookie, Value: token, Path: platformCookiePath, HttpOnly: true,
		Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: int(d.Auth.SessionTTL().Seconds()),
	})
}

// rejectPlatformLogin undoes the just-created session when the account is not a DEWA,
// then returns 401 (same answer for a non-dewa platform user or a merchant account).
func (d Deps) rejectPlatformLogin(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	// Only the fresh session is revoked (p.SessionID); an existing dashboard session stays.
	_ = d.Auth.Logout(r.Context(), p, requestIDOf(r))
	writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials", false)
}

// platformLogin is the panel-specific entry point. Only DEWA accounts can use the
// panel; everyone else gets the same "invalid credentials" answer (no account/role
// enumeration), and any session created in the process is revoked immediately.
func (d Deps) platformLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.Email == "" || body.Password == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "email and password are required", false)
		return
	}
	token, p, err := d.Auth.Login(r.Context(), auth.LoginInput{
		Email: body.Email, Password: body.Password,
		IP: d.clientIP(r), UserAgent: r.UserAgent(), RequestID: requestIDOf(r),
	})
	switch {
	case errors.Is(err, auth.ErrRateLimited):
		var rl *auth.RateLimitedError
		secs := 900
		if errors.As(err, &rl) {
			secs = int(math.Ceil(rl.RetryAfter.Seconds()))
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts, try again later", true)
		return
	case errors.Is(err, auth.ErrMFARequired):
		// Do not reveal the role yet: the DEWA check happens after MFA completes.
		var mfaErr *auth.MFARequiredError
		if errors.As(err, &mfaErr) {
			writeJSON(w, http.StatusOK, map[string]any{
				"mfa_required": true,
				"challenge_id": mfaErr.ChallengeID,
				"expires_at":   mfaErr.ExpiresAt,
			})
			return
		}
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials", false)
		return
	case errors.Is(err, auth.ErrMerchantRequired):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "merchant_id is required for this account", false)
		return
	case err != nil:
		d.Log.Error("platform.login", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	if p == nil || p.MerchantID != "" || !p.Has("rbac:manage") {
		d.rejectPlatformLogin(w, r, p)
		return
	}
	d.setPlatformSession(w, token)
	writeJSON(w, http.StatusOK, meResponse(p))
}

// platformMFAVerify completes a platform login after MFA, then enforces the DEWA check.
func (d Deps) platformMFAVerify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		Code        string `json:"code"`
		ChallengeID string `json:"challenge_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.Email == "" || body.Password == "" || body.Code == "" || body.ChallengeID == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "email, password, code, and challenge_id are required", false)
		return
	}
	token, p, err := d.Auth.CompleteMFALogin(r.Context(), auth.LoginInput{
		Email: body.Email, Password: body.Password,
		IP: d.clientIP(r), UserAgent: r.UserAgent(), RequestID: requestIDOf(r),
	}, body.Code, body.ChallengeID)
	switch {
	case errors.Is(err, auth.ErrMFAInvalidCode):
		writeError(w, r, http.StatusUnauthorized, "MFA_INVALID_CODE", "invalid MFA code", false)
		return
	case errors.Is(err, auth.ErrMFAChallengeExpired):
		writeError(w, r, http.StatusUnauthorized, "MFA_CHALLENGE_EXPIRED", "MFA challenge expired", false)
		return
	case errors.Is(err, auth.ErrRateLimited):
		var rl *auth.RateLimitedError
		secs := 900
		if errors.As(err, &rl) {
			secs = int(math.Ceil(rl.RetryAfter.Seconds()))
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many MFA attempts, try again later", true)
		return
	case err != nil:
		d.Log.Error("platform.mfa.verify", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	if p == nil || p.MerchantID != "" || !p.Has("rbac:manage") {
		d.rejectPlatformLogin(w, r, p)
		return
	}
	d.setPlatformSession(w, token)
	writeJSON(w, http.StatusOK, meResponse(p))
}

func (d Deps) platformMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, meResponse(PrincipalFrom(r.Context())))
}

func (d Deps) platformLogout(w http.ResponseWriter, r *http.Request) {
	if err := d.Auth.Logout(r.Context(), PrincipalFrom(r.Context()), requestIDOf(r)); err != nil {
		d.Log.Error("platform.logout", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: platformCookie, Value: "", Path: platformCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

// platformSetupStatus tells the panel whether bootstrap is still possible.
// enabled = a setup token is configured AND no DEWA exists yet.
func (d Deps) platformSetupStatus(w http.ResponseWriter, r *http.Request) {
	done, err := d.Auth.PlatformAdminExists(r.Context())
	if err != nil {
		d.Log.Error("platform.setup-status", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"done":    done,
		"enabled": d.PlatformSetupToken != "" && !done,
	})
}

// platformSetup bootstraps the first DEWA. It only works while no DEWA exists and
// the caller presents ERAPAY_PLATFORM_SETUP_TOKEN (>= 32 chars, constant-time compare).
func (d Deps) platformSetup(w http.ResponseWriter, r *http.Request) {
	if d.PlatformSetupToken == "" {
		writeError(w, r, http.StatusNotFound, "NOT_CONFIGURED", "platform setup is not configured", false)
		return
	}
	var body struct {
		Token    string `json:"token"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || body.Token == "" || body.Email == "" || body.Password == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "token, email, and password are required", false)
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Token), []byte(d.PlatformSetupToken)) != 1 {
		writeError(w, r, http.StatusUnauthorized, "INVALID_SETUP_TOKEN", "invalid setup token", false)
		return
	}
	uid, err := d.Auth.CreatePlatformAdmin(r.Context(), body.Email, body.Password, requestIDOf(r))
	switch {
	case errors.Is(err, auth.ErrPlatformSetupDone):
		writeError(w, r, http.StatusConflict, "PLATFORM_SETUP_DONE", "platform setup has already been completed", false)
		return
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid email", false)
		return
	case err != nil:
		d.Log.Error("platform.setup", "err", err)
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), false)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id": uid,
		"message": "DEWA created. Log in at /platform/login and complete 2FA.",
	})
}

// mountPlatform wires the platform panel under /v1/platform. It lives outside
// requireAuth because login and setup are public; every other route is behind
// requirePlatformAdmin (DEWA only). MFA and RBAC endpoints are re-mounted here so
// the panel uses its own cookie and never depends on dashboard sessions.
func (d Deps) mountPlatform(r chi.Router) {
	r.Route("/platform", func(r chi.Router) {
		r.Use(requireJSON) // semua endpoint platform JSON (setup, login, MFA, RBAC)
		r.Get("/setup-status", d.platformSetupStatus)
		r.Post("/setup", d.platformSetup)
		r.Post("/auth/login", d.platformLogin)
		r.Post("/auth/mfa/verify", d.platformMFAVerify)
		r.Group(func(r chi.Router) {
			r.Use(d.requirePlatformAdmin)
			r.Get("/auth/me", d.platformMe)
			r.Post("/auth/logout", d.platformLogout)
			// Impersonasi tenant + jejak audit (lihat impersonate.go).
			r.Get("/merchants", d.platformMerchants)
			r.Post("/impersonate", d.platformImpersonate)
			// Aktivasi manual klien khusus (tanpa email): verifikasi email pemilik / set kata sandi langsung.
			r.Post("/merchants/{id}/verify-email", d.adminVerifyMerchantEmail)
			r.Post("/merchants/{id}/owner-password", d.platformSetOwnerPassword)
			// Akun gateway per merchant (aktivasi + tes koneksi); tanpa ini merchant tidak bisa membuat QRIS.
			if d.GWAccounts != nil {
				r.Get("/merchants/{id}/gateway-accounts", d.platformGatewayAccounts)
				r.Put("/merchants/{id}/gateway-accounts", d.platformSetGatewayAccount)
				r.Post("/merchants/{id}/gateway-accounts/test", d.platformTestGatewayAccount)
			}
			r.Get("/audit-logs", d.platformAudit)
			r.Get("/stats", d.platformStats)
			// MFA for the panel (DEWA), using the platform session.
			r.Get("/auth/mfa/status", d.mfaStatus)
			r.Post("/auth/mfa/enroll", d.mfaEnroll)
			r.Post("/auth/mfa/confirm", d.mfaConfirm)
			r.Post("/auth/mfa/disable", d.mfaDisable)
			r.Post("/auth/mfa/regenerate-recovery-codes", d.mfaRegenerateRecoveryCodes)
			// RBAC management, platform-panel only (DEWA via rbac:manage).
			r.Route("/admin", func(r chi.Router) {
				r.With(requirePerm("rbac:manage")).Get("/roles", d.adminListRoles)
				r.With(requirePerm("rbac:manage")).Post("/roles", d.adminCreateRole)
				r.With(requirePerm("rbac:manage")).Put("/roles/{id}", d.adminUpdateRole)
				r.With(requirePerm("rbac:manage")).Delete("/roles/{id}", d.adminDeleteRole)
				r.With(requirePerm("rbac:manage")).Get("/permissions", d.adminListPermissions)
				r.With(requirePerm("rbac:manage")).Get("/role-permissions", d.adminListRolePermissions)
				r.With(requirePerm("rbac:manage")).Post("/role-permissions", d.adminGrantRolePermission)
				r.With(requirePerm("rbac:manage")).Delete("/role-permissions", d.adminRevokeRolePermission)
				r.With(requirePerm("rbac:manage")).Put("/users/{id}/roles", d.adminSetUserRole)
				r.With(requirePerm("rbac:manage")).Get("/users", d.adminListRBACUsers)
				r.With(requirePerm("rbac:manage")).Delete("/users/{id}", d.adminDeleteUser)
				r.With(requirePerm("rbac:manage")).Get("/menu-permissions", d.adminListMenuItems)
				r.With(requirePerm("rbac:manage")).Put("/menu-permissions", d.adminSetMenuPermissions)
			})
		})
	})
}

// contextWithPrincipal stores the principal in the request context, same key used
// by requireAuth so requirePerm/PrincipalFrom work unchanged.
func contextWithPrincipal(r *http.Request, p *auth.Principal) context.Context {
	return context.WithValue(r.Context(), ctxKey{}, p)
}

// requestIDOf returns the chi request id (or "" for public routes without it).
func requestIDOf(r *http.Request) string {
	return middleware.GetReqID(r.Context())
}
