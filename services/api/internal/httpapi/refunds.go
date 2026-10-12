package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/refund"
)

func (d Deps) refundActor(r *http.Request) refund.Actor {
	return refund.Actor{UserID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context())}
}

// refundErr writes the response for err and returns false; nil returns true.
func (d Deps) refundErr(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, refund.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), false)
	case errors.Is(err, refund.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
	case errors.Is(err, refund.ErrNotRefundable):
		writeError(w, r, http.StatusUnprocessableEntity, "PAYMENT_NOT_REFUNDABLE", "only a succeeded payment can be refunded", false)
	case errors.Is(err, refund.ErrTooMuch):
		writeError(w, r, http.StatusUnprocessableEntity, "REFUND_EXCEEDS_PAYMENT", "amount exceeds what is left to refund on this payment", false)
	case errors.Is(err, refund.ErrInsufficient):
		writeError(w, r, http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE", "available (settled) balance cannot cover this refund", false)
	case errors.Is(err, refund.ErrSelfApproval):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "the requester cannot approve their own refund", false)
	case errors.Is(err, refund.ErrState), errors.Is(err, refund.ErrRefConflict):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", err.Error(), false)
	case errors.Is(err, idempotency.ErrConflict):
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was used with a different request", false)
	case errors.Is(err, idempotency.ErrInProgress):
		w.Header().Set("Retry-After", "2")
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "a request with this Idempotency-Key is still in progress", true)
	default:
		d.Log.Error("refund", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

// requestRefund implements POST /v1/payments/{id}/refund (dashboard session; never an API key).
func (d Deps) requestRefund(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !idempotency.ValidKey(key) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key header is required (8-128 chars: letters, digits, . _ : -)", false)
		return
	}
	var body struct {
		Amount int64  `json:"amount"`
		Reason string `json:"reason"`
	}
	raw, ok := decodeStrict(w, r, 4<<10, &body)
	if !ok {
		return
	}
	hash, err := idempotency.HashJSON(raw, r.Method, "/v1/payments/"+id+"/refund")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body", false)
		return
	}
	p := PrincipalFrom(r.Context())
	out, replayed, err := d.Refunds.Request(r.Context(), refund.RequestInput{MerchantID: p.MerchantID, RequestedBy: p.UserID,
		PaymentID: id, Amount: body.Amount, Reason: body.Reason, IdempotencyKey: key, RequestHash: hash, RequestID: middleware.GetReqID(r.Context())})
	if !d.refundErr(w, r, err) {
		return
	}
	code := http.StatusCreated
	if replayed {
		code = http.StatusOK
	}
	writeJSON(w, code, out)
}

func (d Deps) listRefunds(w http.ResponseWriter, r *http.Request) {
	limit, ok := d.intQuery(w, r, "limit", 200)
	if !ok {
		return
	}
	q := r.URL.Query()
	if q.Get("payment_id") != "" && !validUUID(q.Get("payment_id")) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "payment_id must be a uuid", false)
		return
	}
	rs, err := d.Refunds.List(r.Context(), PrincipalFrom(r.Context()).MerchantID, q.Get("status"), q.Get("payment_id"), limit)
	if !d.refundErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rs})
}

func (d Deps) getRefund(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rf, err := d.Refunds.Get(r.Context(), PrincipalFrom(r.Context()).MerchantID, id)
	if !d.refundErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, rf)
}

func (d Deps) cancelRefund(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rf, err := d.Refunds.Cancel(r.Context(), PrincipalFrom(r.Context()).MerchantID, id, d.refundActor(r))
	if !d.refundErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, rf)
}

func (d Deps) adminListRefunds(w http.ResponseWriter, r *http.Request) {
	limit, ok := d.intQuery(w, r, "limit", 200)
	if !ok {
		return
	}
	q := r.URL.Query()
	for _, k := range []string{"merchant_id", "payment_id"} {
		if q.Get(k) != "" && !validUUID(q.Get(k)) {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", k+" must be a uuid", false)
			return
		}
	}
	rs, err := d.Refunds.List(r.Context(), q.Get("merchant_id"), q.Get("status"), q.Get("payment_id"), limit)
	if !d.refundErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rs})
}

func (d Deps) adminGetRefund(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rf, err := d.Refunds.GetAny(r.Context(), id)
	if !d.refundErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, rf)
}

func (d Deps) adminRefundAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct {
			Note          string `json:"note"`
			Reason        string `json:"reason"`
			BankReference string `json:"bank_reference"`
		}
		if action != "approve" {
			if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
				return
			}
		}
		var rf refund.Refund
		var err error
		a := d.refundActor(r)
		switch action {
		case "approve":
			rf, err = d.Refunds.Approve(r.Context(), id, a)
		case "reject":
			rf, err = d.Refunds.Reject(r.Context(), id, body.Note, a)
		case "mark-refunded":
			rf, err = d.Refunds.MarkRefunded(r.Context(), id, body.BankReference, a)
		case "mark-failed":
			rf, err = d.Refunds.MarkFailed(r.Context(), id, body.Reason, a)
		}
		if !d.refundErr(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, rf)
	}
}
