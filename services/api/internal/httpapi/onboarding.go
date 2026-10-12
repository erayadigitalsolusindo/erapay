package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/onboarding"
)

func (d Deps) onboardingErr(w http.ResponseWriter, r *http.Request, err error) bool {
	var ve *onboarding.ValidationError
	switch {
	case err == nil:
		return true
	case errors.As(err, &ve):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", ve.Msg, false)
	case errors.Is(err, onboarding.ErrEmailTaken):
		writeError(w, r, http.StatusConflict, "EMAIL_TAKEN", "email is already registered", false)
	case errors.Is(err, onboarding.ErrRateLimited):
		w.Header().Set("Retry-After", "3600")
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many sign-ups, try again later", true)
	case errors.Is(err, onboarding.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
	case errors.Is(err, onboarding.ErrState):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", "merchant is not awaiting review", false)
	default:
		d.Log.Error("onboarding", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

// register is public (no signup session). It never logs the caller in: a new merchant is
// 'submitted' and cannot take payments until a platform admin approves it.
func (d Deps) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountType  string `json:"account_type"`
		Name         string `json:"name"`
		BusinessName string `json:"business_name"`
		Email        string `json:"email"`
		Phone        string `json:"phone"`
		Password     string `json:"password"`
		AcceptTerms  bool   `json:"accept_terms"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	res, err := d.Onboarding.Register(r.Context(), onboarding.RegisterInput{
		AccountType: body.AccountType, OwnerName: body.Name, BusinessName: body.BusinessName,
		Email: body.Email, Phone: body.Phone, Password: body.Password, AcceptTerms: body.AcceptTerms,
		IP: d.clientIP(r), RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// requirePlatform rejects merchant sessions. Review endpoints are cross-tenant, so a permission
// alone is not enough: a merchant session must never reach them whatever its role holds.
func requirePlatform(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := PrincipalFrom(r.Context()); p == nil || p.MerchantID != "" {
			writeError(w, r, http.StatusForbidden, "FORBIDDEN", "platform session required", false)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (d Deps) adminListMerchants(w http.ResponseWriter, r *http.Request) {
	limit, ok := d.intQuery(w, r, "limit", 200)
	if !ok {
		return
	}
	if limit == 0 {
		limit = 50
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "submitted"
	}
	ms, err := d.Onboarding.List(r.Context(), status, limit)
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": ms})
}

func (d Deps) adminGetMerchant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, err := d.Onboarding.Get(r.Context(), id)
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (d Deps) adminApproveMerchant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		MDRBPS      *int `json:"mdr_bps"`
		PlatformBPS *int `json:"platform_bps"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	// Fees are a business decision with no default: both must be stated explicitly.
	if body.MDRBPS == nil || body.PlatformBPS == nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "mdr_bps and platform_bps are required", false)
		return
	}
	m, err := d.Onboarding.Approve(r.Context(), onboarding.ApproveInput{
		MerchantID: id, ActorID: PrincipalFrom(r.Context()).UserID,
		MDRBPS: *body.MDRBPS, PlatformBPS: *body.PlatformBPS, RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (d Deps) adminRejectMerchant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	m, err := d.Onboarding.Reject(r.Context(), onboarding.RejectInput{
		MerchantID: id, ActorID: PrincipalFrom(r.Context()).UserID, Note: body.Note,
		RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, m)
}
