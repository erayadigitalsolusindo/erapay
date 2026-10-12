package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/account"
)

func (d Deps) accountErr(w http.ResponseWriter, r *http.Request, err error) bool {
	var ve *account.ValidationError
	switch {
	case err == nil:
		return true
	case errors.As(err, &ve):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", ve.Msg, false)
	case errors.Is(err, account.ErrInvalidToken):
		writeError(w, r, http.StatusBadRequest, "INVALID_TOKEN", "the link is invalid, expired or already used", false)
	case errors.Is(err, account.ErrRateLimited):
		w.Header().Set("Retry-After", "3600")
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, try again later", true)
	default:
		d.Log.Error("account", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

func (d Deps) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	if !d.accountErr(w, r, d.Account.Verify(r.Context(), strings.TrimSpace(body.Token), middleware.GetReqID(r.Context()))) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

// emailRequest is the shared handler of resend-verification and forgot-password. Whatever the address,
// the answer is the same 202, so neither endpoint can be used to learn which emails have accounts.
func (d Deps) emailRequest(do func(r *http.Request, ip, email string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email string `json:"email"`
		}
		if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
			return
		}
		if e := strings.TrimSpace(body.Email); e == "" || len(e) > 254 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "email is required", false)
			return
		}
		if !d.accountErr(w, r, do(r, d.clientIP(r), body.Email)) {
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "ok"})
	}
}

func (d Deps) resendVerification(w http.ResponseWriter, r *http.Request) {
	d.emailRequest(func(r *http.Request, ip, email string) error {
		return d.Account.ResendVerification(r.Context(), ip, email)
	})(w, r)
}

func (d Deps) forgotPassword(w http.ResponseWriter, r *http.Request) {
	d.emailRequest(func(r *http.Request, ip, email string) error {
		return d.Account.ForgotPassword(r.Context(), ip, email)
	})(w, r)
}

func (d Deps) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	if !d.accountErr(w, r, d.Account.ResetPassword(r.Context(), strings.TrimSpace(body.Token), body.Password, middleware.GetReqID(r.Context()))) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
