package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/platformsetting"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/qrisgate"
)

// qrisgateFee: GET /v1/platform/qrisgate-fee (DEWA): the per-transaction deduction from the tenant's credit. 0 = none.
func (d Deps) qrisgateFee(w http.ResponseWriter, r *http.Request) {
	v, err := d.Settings.Int(r.Context(), platformsetting.QRISGateFee)
	if err != nil {
		d.internalErr(w, r, "qrisgate fee", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fee": v, "currency": "IDR", "max": platformsetting.MaxFee})
}

// platformSetQRISGateFee: PUT /v1/platform/qrisgate-fee {fee, reason}. Takes effect for later transactions.
func (d Deps) platformSetQRISGateFee(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Fee    int64  `json:"fee"`
		Reason string `json:"reason"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	err := d.Settings.SetFee(r.Context(), body.Fee, body.Reason, PrincipalFrom(r.Context()).UserID, middleware.GetReqID(r.Context()))
	switch {
	case errors.Is(err, platformsetting.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "fee must be 0..1000000 and reason at least 3 characters", false)
	case err != nil:
		d.internalErr(w, r, "set qrisgate fee", err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"fee": body.Fee, "currency": "IDR"})
	}
}

// qrisgateOverview: GET /v1/qrisgate (tenant): fee per transaction and the credit balance (may be negative).
func (d Deps) qrisgateOverview(w http.ResponseWriter, r *http.Request) {
	fee, err := d.Settings.Int(r.Context(), platformsetting.QRISGateFee)
	if err != nil {
		d.internalErr(w, r, "qrisgate fee", err)
		return
	}
	bal, err := d.QRISGate.Balance(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if err != nil {
		d.internalErr(w, r, "qrisgate balance", err)
		return
	}
	acc, err := d.QRISGate.Account(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if err != nil {
		d.internalErr(w, r, "qrisgate account", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fee": fee, "balance": bal, "currency": qrisgate.Currency, "max_topup": qrisgate.MaxTopup, "account": acc})
}

// qrisgateSaveAccount: PUT /v1/qrisgate/account {api_key, webhook_secret} (tenant session, merchant:write).
// Write-only: the key and secret are never returned. The gateway stays OFF until DEWA activates the account.
func (d Deps) qrisgateSaveAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey        string `json:"api_key"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if _, ok := decodeStrict(w, r, 8<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	err := d.QRISGate.SaveAccount(r.Context(), p.MerchantID, p.UserID, middleware.GetReqID(r.Context()), body.APIKey, body.WebhookSecret)
	switch {
	case errors.Is(err, qrisgate.ErrUnavailable):
		writeError(w, r, http.StatusUnprocessableEntity, "QRISGATE_UNAVAILABLE", "QRISGate credentials cannot be saved for this account", false)
	case d.qrisgateErr(w, r, err):
		acc, err := d.QRISGate.Account(r.Context(), p.MerchantID)
		if err != nil {
			d.internalErr(w, r, "qrisgate account", err)
			return
		}
		writeJSON(w, http.StatusOK, acc)
	}
}

func (d Deps) qrisgateErr(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, qrisgate.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid amount, reference, or note", false)
	case errors.Is(err, qrisgate.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "top-up not found", false)
	case errors.Is(err, qrisgate.ErrInvalidState):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", err.Error(), false)
	case errors.Is(err, qrisgate.ErrSelfDecision):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", err.Error(), false)
	case errors.Is(err, qrisgate.ErrConflict):
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was used with a different amount", false)
	default:
		d.internalErr(w, r, "qrisgate", err)
	}
	return false
}

// qrisgateRequestTopup: POST /v1/qrisgate/topups {amount} + Idempotency-Key (tenant session).
func (d Deps) qrisgateRequestTopup(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if !idempotency.ValidKey(key) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key header is required (8-128 chars: letters, digits, . _ : -)", false)
		return
	}
	var body struct {
		Amount int64 `json:"amount"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	t, err := d.QRISGate.RequestTopup(r.Context(), p.MerchantID, p.UserID, key, body.Amount)
	if d.qrisgateErr(w, r, err) {
		st := http.StatusCreated
		if t.Replayed {
			st = http.StatusOK
		}
		writeJSON(w, st, t)
	}
}

// qrisgateRequestQRISTopup: POST /v1/qrisgate/topups/qris {amount} + Idempotency-Key (tenant session). The response
// carries the QR to pay; the balance is credited by itself when the payment succeeds.
func (d Deps) qrisgateRequestQRISTopup(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if !idempotency.ValidKey(key) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key header is required (8-128 chars: letters, digits, . _ : -)", false)
		return
	}
	var body struct {
		Amount int64 `json:"amount"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	t, err := d.QRISGate.RequestQRISTopup(r.Context(), p.MerchantID, p.UserID, key, body.Amount)
	switch {
	case errors.Is(err, payment.ErrMethodUnavailable), errors.Is(err, payment.ErrNoPricing), errors.Is(err, payment.ErrMerchantNotActive):
		writeError(w, r, http.StatusUnprocessableEntity, "QRIS_UNAVAILABLE", "QRIS is not available for this merchant yet", false)
	case d.qrisgateErr(w, r, err):
		st := http.StatusCreated
		if t.Replayed {
			st = http.StatusOK
		}
		writeJSON(w, st, t)
	}
}

func (d Deps) qrisgateListTopups(w http.ResponseWriter, r *http.Request) {
	rows, err := d.QRISGate.ListTopups(r.Context(), PrincipalFrom(r.Context()).MerchantID, "", 50)
	if err != nil {
		d.internalErr(w, r, "qrisgate topups", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (d Deps) qrisgateCancelTopup(w http.ResponseWriter, r *http.Request) {
	if d.qrisgateErr(w, r, d.QRISGate.CancelTopup(r.Context(), PrincipalFrom(r.Context()).MerchantID, chi.URLParam(r, "id"))) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (d Deps) qrisgateHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := d.QRISGate.History(r.Context(), PrincipalFrom(r.Context()).MerchantID, 100)
	if err != nil {
		d.internalErr(w, r, "qrisgate history", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// adminListQRISGateTopups: GET /v1/admin/qrisgate-topups?status= (Finance).
func (d Deps) adminListQRISGateTopups(w http.ResponseWriter, r *http.Request) {
	st := r.URL.Query().Get("status")
	rows, err := d.QRISGate.ListTopups(r.Context(), "", st, 200)
	if err != nil {
		d.internalErr(w, r, "qrisgate admin topups", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// adminDecideQRISGateTopup: POST /v1/admin/qrisgate-topups/{id}/confirm|reject.
func (d Deps) adminDecideQRISGateTopup(confirm bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			BankReference string `json:"bank_reference"`
			Note          string `json:"note"`
		}
		if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
			return
		}
		t, err := d.QRISGate.Decide(r.Context(), chi.URLParam(r, "id"), PrincipalFrom(r.Context()).UserID,
			middleware.GetReqID(r.Context()), confirm, body.BankReference, body.Note)
		if d.qrisgateErr(w, r, err) {
			writeJSON(w, http.StatusOK, t)
		}
	}
}

// qrisgateTestWebhook: POST /v1/qrisgate/account/test (tenant session, merchant:write). Signs a test notification
// with the tenant's STORED secret and verifies it in-process; the secret never leaves the server.
func (d Deps) qrisgateTestWebhook(w http.ResponseWriter, r *http.Request) {
	type tester interface {
		TestWebhook(ctx context.Context, merchantID string) []gateway.ConnectionStep
	}
	var t tester
	if d.Gateways != nil {
		if ad, ok := d.Gateways.Get("qrisgate"); ok {
			t, _ = ad.(tester)
		}
	}
	if t == nil {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "QRISGate is not enabled on this server", false)
		return
	}
	steps := t.TestWebhook(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	all := true
	for _, s := range steps {
		all = all && s.OK
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": all, "steps": steps})
}
