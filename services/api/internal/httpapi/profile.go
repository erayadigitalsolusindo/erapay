package httpapi

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
)

// mfaStepUp membaca {password, code} dari body dan memverifikasinya (kata sandi + kode TOTP/pemulihan).
// Mengembalikan false setelah menulis respons bila gagal. Dipakai oleh aksi sensitif MFA.
func (d Deps) mfaStepUp(w http.ResponseWriter, r *http.Request, p *auth.Principal) bool {
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return false
	}
	if body.Password == "" || body.Code == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "password and code are required", false)
		return false
	}
	err := d.Auth.StepUpMFA(r.Context(), p.UserID, body.Password, body.Code)
	switch {
	case err == nil:
		return true
	case errors.Is(err, auth.ErrMFARateLimited):
		secs := 900
		var rl *auth.MFARateLimitedError
		if errors.As(err, &rl) {
			secs = int(math.Ceil(rl.RetryAfter.Seconds()))
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts, try again later", true)
	case errors.Is(err, auth.ErrMFANotEnrolled):
		writeError(w, r, http.StatusBadRequest, "MFA_NOT_ENROLLED", "MFA not enrolled", false)
	case errors.Is(err, auth.ErrMFAStepUpFailed):
		writeError(w, r, http.StatusUnauthorized, "MFA_INVALID_CODE", "password or code is incorrect", false)
	default:
		d.Log.Error("mfa.stepup", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

func (d Deps) getProfile(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	pr, err := d.Auth.GetProfile(r.Context(), p.UserID)
	if err != nil {
		d.Log.Error("profile.get", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, pr)
}

func (d Deps) updateProfile(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var body struct {
		FullName string `json:"full_name"`
		Phone    string `json:"phone"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	err := d.Auth.UpdateProfile(r.Context(), p.UserID, body.FullName, body.Phone, middleware.GetReqID(r.Context()))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidProfile) {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "nama maksimal 100 karakter dan telepon harus berupa nomor yang valid", false)
			return
		}
		d.Log.Error("profile.update", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	d.getProfile(w, r)
}

func (d Deps) changePassword(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	var body struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	if body.Current == "" || body.New == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "current_password and new_password are required", false)
		return
	}
	err := d.Auth.ChangePassword(r.Context(), p.UserID, p.SessionID, body.Current, body.New, d.clientIP(r), middleware.GetReqID(r.Context()))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, auth.ErrRateLimited):
		secs := 900
		var rl *auth.RateLimitedError
		if errors.As(err, &rl) {
			secs = int(math.Ceil(rl.RetryAfter.Seconds()))
		}
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts, try again later", true)
	case errors.Is(err, auth.ErrWrongPassword):
		writeError(w, r, http.StatusUnauthorized, "WRONG_PASSWORD", "current password is incorrect", false)
	case errors.Is(err, auth.ErrWeakPassword):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), false)
	default:
		d.Log.Error("profile.password", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
}
