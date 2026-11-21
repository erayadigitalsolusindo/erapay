package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/onboarding"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/qrisgate"
)

func (d Deps) requireARUSProvisionToken(next http.Handler) http.Handler {
	return requireBearer(d.ARUSProvisionToken, next)
}

// requireARUSKeyToken: token TERPISAH khusus penerbitan API key, agar bocornya token provisioning tidak memberi hak itu.
func (d Deps) requireARUSKeyToken(next http.Handler) http.Handler {
	return requireBearer(d.ARUSKeyToken, next)
}

func requireBearer(want string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == r.Header.Get("Authorization") || subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "integration authentication required", false)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// arusKeyName menandai key yang diterbitkan lewat integrasi ini; hanya satu yang boleh aktif per merchant.
const arusKeyName = "ARUS (otomatis)"

// arusIssueKey: POST /v1/integrations/arus/tenants/{tenant_id}/api-key — server-to-server.
// Menerbitkan API key pembayaran (payment:create + payment:read saja, tanpa payout) untuk merchant yang sudah
// tertaut DAN disetujui. Secret hanya ada di respons ini. Bila key ARUS aktif sudah ada: 409 (rotasi lewat dashboard ERAPAY).
func (d Deps) arusIssueKey(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil || d.Onboarding == nil || d.APIKeys == nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid tenant_id or key issuing disabled", false)
		return
	}
	link, err := d.Onboarding.ARUSTenantStatus(r.Context(), tenantID)
	if errors.Is(err, onboarding.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "tenant is not linked", false)
		return
	}
	if err == nil && link.MerchantStatus != "approved" {
		writeError(w, r, http.StatusConflict, "MERCHANT_NOT_APPROVED", "merchant must be approved before an API key can be issued", false)
		return
	}
	var keys []apikey.Key
	if err == nil {
		keys, err = d.APIKeys.List(r.Context(), link.MerchantID.String())
	}
	if err != nil {
		d.Log.Error("arus issue key", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	now := time.Now()
	for _, k := range keys {
		if k.Name == arusKeyName && k.RevokedAt == nil && (k.ExpiresAt == nil || k.ExpiresAt.After(now)) {
			writeError(w, r, http.StatusConflict, "API_KEY_EXISTS", "an active ARUS API key already exists; rotate it from the ERAPAY dashboard", false)
			return
		}
	}
	k, err := d.APIKeys.Create(r.Context(), apikey.CreateInput{
		MerchantID: link.MerchantID.String(), Name: arusKeyName, Scopes: []string{"payment:create", "payment:read"},
		RequestID: middleware.GetReqID(r.Context())})
	if err != nil {
		d.Log.Error("arus issue key", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, k)
}

// arusApprove: POST /v1/integrations/arus/tenants/{tenant_id}/approve — server-to-server (token key).
// Persetujuan merchant tertaut oleh Platform Admin ARUS (sudah diverifikasi ulang password + TOTP di ARUS).
// Cek email/KYC dilewati (onboarding.ApproveInput.Via); tarif wajib dinyatakan eksplisit.
func (d Deps) arusApprove(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil || d.Onboarding == nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid tenant_id or approval disabled", false)
		return
	}
	var body struct {
		MDRBPS        int    `json:"mdr_bps"`
		PlatformBPS   int    `json:"platform_bps"`
		MDRFixed      int64  `json:"mdr_fixed"`
		PlatformFixed int64  `json:"platform_fixed"`
		MDRFlatUpTo   int64  `json:"mdr_flat_up_to"`
		MDRFlatFee    int64  `json:"mdr_flat_fee"`
		Admin         string `json:"admin"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	link, err := d.Onboarding.ARUSTenantStatus(r.Context(), tenantID)
	if errors.Is(err, onboarding.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "tenant is not linked", false)
		return
	}
	if err != nil {
		d.Log.Error("arus approve", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	m, err := d.Onboarding.Approve(r.Context(), onboarding.ApproveInput{
		MerchantID: link.MerchantID.String(), RequestID: middleware.GetReqID(r.Context()), Via: "arus", Admin: body.Admin,
		MDRBPS: body.MDRBPS, PlatformBPS: body.PlatformBPS, MDRFixed: body.MDRFixed, PlatformFixed: body.PlatformFixed,
		MDRFlatUpTo: body.MDRFlatUpTo, MDRFlatFee: body.MDRFlatFee})
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, m)
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

// arusListenerSetup: POST /v1/integrations/arus/tenants/{tenant_id}/listener-setup — server-to-server (token key).
// Mengembalikan tautan erapaylistener://setup (URL webhook, rahasia, paket) agar app ARUS bisa mengisi app Listener
// tanpa ketik ulang. Rahasia listener dibuat bila belum ada; bila sudah ada dipakai ulang (HP lama tetap jalan).
func (d Deps) arusListenerSetup(w http.ResponseWriter, r *http.Request) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenant_id"))
	if err != nil || d.Onboarding == nil || d.QRISGate == nil || d.PublicAPIURL == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid tenant_id or listener setup disabled", false)
		return
	}
	link, err := d.Onboarding.ARUSTenantStatus(r.Context(), tenantID)
	if errors.Is(err, onboarding.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "tenant is not linked", false)
		return
	}
	var acc qrisgate.Account
	var secret string
	if err == nil {
		mid := link.MerchantID.String()
		if acc, err = d.QRISGate.Account(r.Context(), mid); err == nil {
			if acc.HasSecret {
				secret, err = d.QRISGate.ListenerSecret(r.Context(), mid)
			} else {
				secret, err = d.QRISGate.RotateListenerSecret(r.Context(), mid, "", middleware.GetReqID(r.Context()))
			}
		}
	}
	if errors.Is(err, qrisgate.ErrUnavailable) {
		writeError(w, r, http.StatusUnprocessableEntity, "QRISGATE_UNAVAILABLE", "QRISGate cannot be set up for this account", false)
		return
	}
	if err != nil {
		d.Log.Error("arus listener setup", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	q := url.Values{"url": {d.PublicAPIURL + acc.WebhookPath}, "secret": {secret}}
	if len(acc.Packages) > 0 { // kosong: app Listener memakai paket bawaannya
		q.Set("packages", strings.Join(acc.Packages, "\n"))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"link": "erapaylistener://setup?" + q.Encode(), "ready": acc.Ready})
}
