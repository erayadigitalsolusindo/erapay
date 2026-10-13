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
	case errors.Is(err, onboarding.ErrRateLimited):
		w.Header().Set("Retry-After", "3600")
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many sign-ups, try again later", true)
	case errors.Is(err, onboarding.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
	case errors.Is(err, onboarding.ErrEmailUnverified):
		writeError(w, r, http.StatusConflict, "EMAIL_NOT_VERIFIED", "the owner has not verified their email yet", false)
	case errors.Is(err, onboarding.ErrEmailDeliveryUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "EMAIL_DELIVERY_UNAVAILABLE", "owner email delivery is not configured", false)
	case errors.Is(err, onboarding.ErrARUSMerchantAmbiguous):
		writeError(w, r, http.StatusConflict, "MERCHANT_LINK_AMBIGUOUS", "multiple existing merchants match this ARUS tenant", false)
	case errors.Is(err, onboarding.ErrState):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", "merchant is not in a state that allows this action", false)
	default:
		d.Log.Error("onboarding", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

// register is public (no signup session). It never logs the caller in: a new merchant is
// 'submitted' and cannot take payments until its owner verifies their email and a platform admin approves it.
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
	// 202 with the same body whether the address is new or already registered (see Registered).
	writeJSON(w, http.StatusAccepted, res)
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

func (d Deps) adminReviewAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct {
			Note string `json:"note"`
		}
		if action == "request-changes" {
			if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
				return
			}
		}
		in := onboarding.ReviewInput{MerchantID: id, ActorID: PrincipalFrom(r.Context()).UserID, Note: body.Note,
			RequestID: middleware.GetReqID(r.Context())}
		var m onboarding.Merchant
		var err error
		if action == "start-review" {
			m, err = d.Onboarding.StartReview(r.Context(), in)
		} else {
			m, err = d.Onboarding.RequestChanges(r.Context(), in)
		}
		if !d.onboardingErr(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, m)
	}
}

// merchantResubmit answers a change request. The merchant comes from the session only.
func (d Deps) merchantResubmit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LegalName    *string `json:"legal_name"`
		ContactPhone *string `json:"contact_phone"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	m, err := d.Onboarding.Resubmit(r.Context(), onboarding.ResubmitInput{MerchantID: p.MerchantID, ActorID: p.UserID,
		LegalName: body.LegalName, ContactPhone: body.ContactPhone, RequestID: middleware.GetReqID(r.Context())})
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, m)
}
