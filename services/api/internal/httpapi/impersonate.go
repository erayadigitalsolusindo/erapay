package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
)

// Impersonasi DEWA ("masuk sebagai tenant", pola ARUS). Aturan:
//   - Hanya DEWA di panel (/v1/platform/impersonate), alasan wajib (>= 10 karakter).
//   - Hasilnya sesi merchant biasa (cookie erapay_session) atas nama DEWA, TTL 60 menit,
//     izin = role merchant_owner. Aktor di semua audit tetap user DEWA yang sebenarnya.
//   - Setiap request pada sesi ini ditulis ke audit_logs (impersonation.request) SEBELUM
//     handler jalan; untuk method yang mengubah data, gagal menulis audit = request ditolak.
//   - Endpoint akun DEWA sendiri (MFA, profil, kata sandi) tertutup selama impersonasi.

// readOnlyQuiet are polled by the dashboard shell on every navigation; logging them
// would bury the real activity.
var impersonationQuiet = map[string]bool{"/v1/auth/me": true, "/v1/menus": true}

// guardImpersonation audits an impersonated request. It returns false after writing the
// response when the request must not proceed.
func (d Deps) guardImpersonation(w http.ResponseWriter, r *http.Request, p *auth.Principal) bool {
	path := r.URL.Path
	if strings.HasPrefix(path, "/v1/auth/mfa/") || strings.HasPrefix(path, "/v1/profile") {
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "not available while impersonating", false)
		return false
	}
	mutating := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
	if !mutating && impersonationQuiet[path] {
		return true
	}
	err := d.Auth.RecordImpersonated(r.Context(), p, requestIDOf(r), map[string]any{
		"method": r.Method, "path": path, "mutating": mutating,
	})
	if err != nil {
		d.Log.Error("impersonation.audit", "err", err)
		if mutating { // fail closed: no audit trail, no change
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
			return false
		}
	}
	return true
}

func (d Deps) setImpersonationSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: d.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: int(auth.ImpersonationTTL().Seconds()),
	})
}

// platformImpersonate: POST /v1/platform/impersonate {merchant_id, reason}.
// Returns the new session in a Set-Cookie (erapay_session); the SvelteKit server forwards it.
func (d Deps) platformImpersonate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MerchantID string `json:"merchant_id"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.MerchantID == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "merchant_id and reason are required", false)
		return
	}
	token, p, err := d.Auth.StartImpersonation(r.Context(), PrincipalFrom(r.Context()), auth.StartImpersonationInput{
		MerchantID: body.MerchantID, Reason: body.Reason,
		IP: d.clientIP(r), UserAgent: r.UserAgent(), RequestID: requestIDOf(r),
	})
	switch {
	case errors.Is(err, auth.ErrReasonRequired):
		writeError(w, r, http.StatusBadRequest, "REASON_REQUIRED", "alasan wajib diisi (10–500 karakter)", false)
		return
	case errors.Is(err, auth.ErrMerchantNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "merchant not found", false)
		return
	case errors.Is(err, auth.ErrNotPlatformAdmin):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "platform admin (DEWA) required", false)
		return
	case err != nil:
		// Malformed uuid surfaces here too; treat as bad input rather than a 500.
		if strings.Contains(err.Error(), "invalid input syntax for type uuid") {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid merchant_id", false)
			return
		}
		d.Log.Error("platform.impersonate", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	d.setImpersonationSession(w, token)
	writeJSON(w, http.StatusOK, meResponse(p))
}

// platformMerchants: GET /v1/platform/merchants?q=&limit= — tenant picker.
func (d Deps) platformMerchants(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	ms, err := d.Auth.SearchMerchants(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		d.Log.Error("platform.merchants", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": ms})
}

// platformAudit: GET /v1/platform/audit-logs?prefix=&merchant_id=&actor_id=&before=&limit=
// Read-only view of audit_logs for the DEWA (default: impersonation.* trail).
func (d Deps) platformAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, err := d.Auth.ListAudit(r.Context(), auth.AuditFilter{
		Prefix: q.Get("prefix"), MerchantID: q.Get("merchant_id"), ActorID: q.Get("actor_id"),
		BeforeID: q.Get("before"), Limit: limit,
	})
	if err != nil {
		if strings.Contains(err.Error(), "invalid input syntax for type uuid") {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid id filter", false)
			return
		}
		d.Log.Error("platform.audit", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}
