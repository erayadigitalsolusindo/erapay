package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payout"
)

func (d Deps) payoutActor(r *http.Request) payout.Actor {
	return payout.Actor{UserID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context())}
}

// payoutErr writes the response for err and returns false; nil returns true.
func (d Deps) payoutErr(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, payout.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), false)
	case errors.Is(err, payout.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
	case errors.Is(err, payout.ErrDisabled):
		writeError(w, r, http.StatusServiceUnavailable, "PAYOUT_DISABLED", "payouts are not enabled in this environment", false)
	case errors.Is(err, payout.ErrMerchantState):
		writeError(w, r, http.StatusForbidden, "MERCHANT_NOT_ACTIVE", "merchant is not active", false)
	case errors.Is(err, payout.ErrAccountUnusable):
		writeError(w, r, http.StatusUnprocessableEntity, "BANK_ACCOUNT_UNUSABLE", "bank account is not verified, still cooling down, or unknown", false)
	case errors.Is(err, payout.ErrLimit):
		writeError(w, r, http.StatusUnprocessableEntity, "PAYOUT_LIMIT", "amount is outside the allowed payout limits", false)
	case errors.Is(err, payout.ErrInsufficient):
		writeError(w, r, http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE", "amount exceeds the available balance", false)
	case errors.Is(err, payout.ErrSelfApproval):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "the requester cannot approve their own payout", false)
	case errors.Is(err, payout.ErrState), errors.Is(err, payout.ErrAccountInUse), errors.Is(err, payout.ErrRefConflict):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", err.Error(), false)
	case errors.Is(err, idempotency.ErrConflict):
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was used with a different request", false)
	case errors.Is(err, idempotency.ErrInProgress):
		w.Header().Set("Retry-After", "2")
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "a request with this Idempotency-Key is still in progress", true)
	default:
		d.Log.Error("payout", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

func decodeStrict(w http.ResponseWriter, r *http.Request, max int64, dst any) ([]byte, bool) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, max))
	if err != nil {
		writeError(w, r, http.StatusRequestEntityTooLarge, "VALIDATION_ERROR", "request body too large", false)
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body: unknown or malformed field", false)
		return nil, false
	}
	return raw, true
}

func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
		return "", false
	}
	return id, true
}

// ---- merchant side (dashboard session only: money-out is never reachable with an API key) ----

func (d Deps) createBankAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BankCode      string `json:"bank_code"`
		AccountNumber string `json:"account_number"`
		AccountName   string `json:"account_name"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	b, err := d.Payouts.AddBankAccount(r.Context(), PrincipalFrom(r.Context()).MerchantID, body.BankCode, body.AccountNumber,
		strings.TrimSpace(body.AccountName), d.payoutActor(r))
	if !d.payoutErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (d Deps) listBankAccounts(w http.ResponseWriter, r *http.Request) {
	bs, err := d.Payouts.ListBankAccounts(r.Context(), PrincipalFrom(r.Context()).MerchantID, "")
	if !d.payoutErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": bs})
}

func (d Deps) disableBankAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !d.payoutErr(w, r, d.Payouts.DisableBankAccount(r.Context(), PrincipalFrom(r.Context()).MerchantID, id, d.payoutActor(r))) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) requestPayout(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if !idempotency.ValidKey(key) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key header is required (8-128 chars: letters, digits, . _ : -)", false)
		return
	}
	var body struct {
		BankAccountID string `json:"bank_account_id"`
		Amount        int64  `json:"amount"`
		Currency      string `json:"currency"`
	}
	raw, ok := decodeStrict(w, r, 4<<10, &body)
	if !ok {
		return
	}
	hash, err := idempotency.HashJSON(raw, r.Method, "/v1/payouts")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body", false)
		return
	}
	p := PrincipalFrom(r.Context())
	out, replayed, err := d.Payouts.Request(r.Context(), payout.RequestInput{MerchantID: p.MerchantID, RequestedBy: p.UserID,
		BankAccountID: body.BankAccountID, Amount: body.Amount, Currency: body.Currency, IdempotencyKey: key,
		RequestHash: hash, RequestID: middleware.GetReqID(r.Context())})
	if !d.payoutErr(w, r, err) {
		return
	}
	code := http.StatusCreated
	if replayed {
		code = http.StatusOK
	}
	writeJSON(w, code, out)
}

func (d Deps) listPayouts(w http.ResponseWriter, r *http.Request) {
	limit, ok := d.intQuery(w, r, "limit", 200)
	if !ok {
		return
	}
	ps, err := d.Payouts.List(r.Context(), PrincipalFrom(r.Context()).MerchantID, r.URL.Query().Get("status"), limit)
	if !d.payoutErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": ps})
}

func (d Deps) getPayout(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := d.Payouts.Get(r.Context(), PrincipalFrom(r.Context()).MerchantID, id)
	if !d.payoutErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (d Deps) cancelPayout(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := d.Payouts.Cancel(r.Context(), PrincipalFrom(r.Context()).MerchantID, id, d.payoutActor(r))
	if !d.payoutErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ---- platform side (Finance / Ops) ----

func (d Deps) adminListBankAccounts(w http.ResponseWriter, r *http.Request) {
	st := r.URL.Query().Get("status")
	if st != "" && st != "pending_verification" && st != "verified" && st != "disabled" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "unknown status", false)
		return
	}
	bs, err := d.Payouts.ListBankAccounts(r.Context(), "", st)
	if !d.payoutErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": bs})
}

func (d Deps) adminVerifyBankAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	b, err := d.Payouts.VerifyBankAccount(r.Context(), id, d.payoutActor(r))
	if !d.payoutErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (d Deps) adminListPayouts(w http.ResponseWriter, r *http.Request) {
	limit, ok := d.intQuery(w, r, "limit", 200)
	if !ok {
		return
	}
	q := r.URL.Query()
	if q.Get("merchant_id") != "" && !validUUID(q.Get("merchant_id")) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "merchant_id must be a uuid", false)
		return
	}
	ps, err := d.Payouts.List(r.Context(), q.Get("merchant_id"), q.Get("status"), limit)
	if !d.payoutErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": ps})
}

func (d Deps) adminGetPayout(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := d.Payouts.GetAny(r.Context(), id)
	if !d.payoutErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (d Deps) adminPayoutAction(action string) http.HandlerFunc {
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
		var p payout.Payout
		var err error
		a := d.payoutActor(r)
		switch action {
		case "approve":
			p, err = d.Payouts.Approve(r.Context(), id, a)
		case "reject":
			p, err = d.Payouts.Reject(r.Context(), id, body.Note, a)
		case "mark-paid":
			p, err = d.Payouts.MarkPaid(r.Context(), id, body.BankReference, a)
		case "mark-failed":
			p, err = d.Payouts.MarkFailed(r.Context(), id, body.Reason, a)
		}
		if !d.payoutErr(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func (d Deps) adminPayoutDestination(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	dest, err := d.Payouts.RevealDestination(r.Context(), id, d.payoutActor(r))
	if !d.payoutErr(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, dest)
}
