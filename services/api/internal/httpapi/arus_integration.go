package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/onboarding"
)

func (d Deps) requireARUSProvisionToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == r.Header.Get("Authorization") || subtle.ConstantTimeCompare([]byte(token), []byte(d.ARUSProvisionToken)) != 1 {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "integration authentication required", false)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d Deps) arusTenantStatus(w http.ResponseWriter, r *http.Request) {
	if d.Onboarding == nil {
		writeError(w, r, http.StatusServiceUnavailable, "INTEGRATION_DISABLED", "tenant provisioning is unavailable", false)
		return
	}
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid tenant_id", false)
		return
	}
	link, err := d.Onboarding.ARUSTenantStatus(r.Context(), tenantID)
	if errors.Is(err, onboarding.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "tenant is not linked", false)
		return
	}
	if err != nil {
		d.Log.Error("arus tenant status", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (d Deps) provisionARUSTenant(w http.ResponseWriter, r *http.Request) {
	if d.Onboarding == nil {
		writeError(w, r, http.StatusServiceUnavailable, "INTEGRATION_DISABLED", "tenant provisioning is unavailable", false)
		return
	}
	var body struct {
		TenantID      uuid.UUID `json:"tenant_id"`
		BusinessName  string    `json:"business_name"`
		OwnerName     string    `json:"owner_name"`
		OwnerEmail    string    `json:"owner_email"`
		OwnerPhone    string    `json:"owner_phone"`
		AcceptedTerms bool      `json:"accepted_terms"`
	}
	if _, ok := decodeStrict(w, r, 8<<10, &body); !ok {
		return
	}
	link, err := d.Onboarding.ProvisionARUSTenant(r.Context(), onboarding.ARUSTenantInput{
		TenantID: body.TenantID, BusinessName: body.BusinessName, OwnerName: body.OwnerName,
		OwnerEmail: body.OwnerEmail, OwnerPhone: body.OwnerPhone, AcceptedTerms: body.AcceptedTerms,
		RequestID: middleware.GetReqID(r.Context()),
	})
	if err != nil {
		if !d.onboardingErr(w, r, err) {
			return
		}
		writeError(w, r, http.StatusServiceUnavailable, "INTEGRATION_DISABLED", "owner email delivery is unavailable", false)
		return
	}
	writeJSON(w, http.StatusOK, link)
}

// arusTenantSSO: POST /v1/integrations/arus/tenants/{tenant_id}/sso — server-to-server.
// Returns the dashboard URL carrying a single-use code that signs the tenant's Owner in.
func (d Deps) arusTenantSSO(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil || d.Auth == nil || d.WebURL == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid tenant_id or sso disabled", false)
		return
	}
	code, ttl, err := d.Auth.IssueARUSSSOCode(r.Context(), tenantID, middleware.GetReqID(r.Context()))
	if errors.Is(err, auth.ErrSSOUnavailable) {
		writeError(w, r, http.StatusNotFound, "SSO_UNAVAILABLE", "tenant is not linked", false)
		return
	}
	if err != nil {
		d.Log.Error("arus sso", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": d.WebURL + "/sso?code=" + code, "expires_in": int(ttl.Seconds())})
}

// ssoExchange: POST /v1/auth/sso/exchange {code} — public; the single-use code is the credential.
func (d Deps) ssoExchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Code == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "code is required", false)
		return
	}
	token, p, err := d.Auth.ExchangeSSOCode(r.Context(), body.Code, auth.LoginInput{
		IP: d.clientIP(r), UserAgent: r.UserAgent(), RequestID: middleware.GetReqID(r.Context())})
	if errors.Is(err, auth.ErrSSOInvalidCode) {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "code invalid or expired", false)
		return
	}
	if err != nil {
		d.Log.Error("sso exchange", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: int(d.Auth.SessionTTL().Seconds()),
	})
	writeJSON(w, http.StatusOK, meResponse(p))
}
