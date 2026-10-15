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
	case errors.Is(err, onboarding.ErrIdentityIncomplete):
		writeError(w, r, http.StatusConflict, "IDENTITY_INCOMPLETE", "the merchant has not completed its identity data yet", false)
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
		AccountType  string                    `json:"account_type"`
		Name         string                    `json:"name"`
		BusinessName string                    `json:"business_name"`
		Email        string                    `json:"email"`
		Phone        string                    `json:"phone"`
		Password     string                    `json:"password"`
		AcceptTerms  bool                      `json:"accept_terms"`
		Identity     *onboarding.IdentityInput `json:"identity"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	res, err := d.Onboarding.Register(r.Context(), onboarding.RegisterInput{
		AccountType: body.AccountType, OwnerName: body.Name, BusinessName: body.BusinessName,
		Email: body.Email, Phone: body.Phone, Password: body.Password, AcceptTerms: body.AcceptTerms,
		Identity: body.Identity,
		IP:       d.clientIP(r), RequestID: middleware.GetReqID(r.Context()),
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
	// The reviewer detail view opens the sealed NIK/NPWP so the data diri can actually be checked;
	// every other view (list, the merchant's own) sees masks only.
	m, err := d.Onboarding.GetForReview(r.Context(), id)
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
		MDRBPS        *int   `json:"mdr_bps"`
		PlatformBPS   *int   `json:"platform_bps"`
		MDRFixed      *int64 `json:"mdr_fixed"`
		PlatformFixed *int64 `json:"platform_fixed"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	// Fees are a business decision with no default: each component (MDR, platform) must be stated
	// explicitly, as a percentage (bps), a fixed rupiah amount, or both. The unstated half is 0.
	if (body.MDRBPS == nil && body.MDRFixed == nil) || (body.PlatformBPS == nil && body.PlatformFixed == nil) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "mdr and platform fee are required (mdr_bps/mdr_fixed and platform_bps/platform_fixed)", false)
		return
	}
	in := onboarding.ApproveInput{MerchantID: id, ActorID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context())}
	if body.MDRBPS != nil {
		in.MDRBPS = *body.MDRBPS
	}
	if body.PlatformBPS != nil {
		in.PlatformBPS = *body.PlatformBPS
	}
	if body.MDRFixed != nil {
		in.MDRFixed = *body.MDRFixed
	}
	if body.PlatformFixed != nil {
		in.PlatformFixed = *body.PlatformFixed
	}
	m, err := d.Onboarding.Approve(r.Context(), in)
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

// adminSuspendMerchant: tangguhkan merchant, atau aktifkan kembali (reactivate); alasan wajib.
func (d Deps) adminSuspendMerchant(reactivate bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		in := onboarding.SuspendInput{MerchantID: id, ActorID: PrincipalFrom(r.Context()).UserID, Note: body.Note,
			RequestID: middleware.GetReqID(r.Context())}
		var m onboarding.Merchant
		var err error
		if reactivate {
			m, err = d.Onboarding.Reactivate(r.Context(), in)
		} else {
			m, err = d.Onboarding.Suspend(r.Context(), in)
		}
		if !d.onboardingErr(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, m)
	}
}

// adminRevokeMerchant: cabut persetujuan (approved -> under_review | rejected), alasan wajib.
func (d Deps) adminRevokeMerchant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		To   string `json:"to"`
		Note string `json:"note"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	m, err := d.Onboarding.Revoke(r.Context(), onboarding.RevokeInput{
		MerchantID: id, ActorID: PrincipalFrom(r.Context()).UserID, To: body.To, Note: body.Note,
		RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// adminVerifyMerchantEmail: verifikasi email pemilik secara manual (alasan wajib, tercatat di audit).
func (d Deps) adminVerifyMerchantEmail(w http.ResponseWriter, r *http.Request) {
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
	m, err := d.Onboarding.VerifyOwnerEmail(r.Context(), onboarding.VerifyEmailInput{
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

// merchantIdentity is the merchant's own view of the data diri it submitted: masked NIK/NPWP,
// the other fields as entered, and whether the set is complete enough for approval.
func (d Deps) merchantIdentity(w http.ResponseWriter, r *http.Request) {
	m, err := d.Onboarding.Get(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, m.Identity)
}

// merchantSetIdentity completes or corrects the data diri during verification. The merchant comes
// from the session only; every rule lives in Go.
func (d Deps) merchantSetIdentity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Identity *onboarding.IdentityInput `json:"identity"`
	}
	if _, ok := decodeStrict(w, r, 8<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	m, err := d.Onboarding.SetIdentity(r.Context(), onboarding.SetIdentityInput{
		MerchantID: p.MerchantID, ActorID: p.UserID, Identity: body.Identity,
		RequestID: middleware.GetReqID(r.Context())})
	if !d.onboardingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, m.Identity)
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
