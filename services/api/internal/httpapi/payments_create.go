package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
)

// createPayment implements POST /v1/payments. The body may NOT contain fee, status,
// gateway or merchant fields: unknown fields are rejected (PRD §0 #7, #8).
func (d Deps) createPayment(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if !idempotency.ValidKey(key) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key header is required (8-128 chars: letters, digits, . _ : -)", false)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		writeError(w, r, http.StatusRequestEntityTooLarge, "VALIDATION_ERROR", "request body too large", false)
		return
	}
	var body struct {
		MerchantReference string     `json:"merchant_reference"`
		Amount            int64      `json:"amount"`
		Currency          string     `json:"currency"`
		PaymentMethod     string     `json:"payment_method"`
		Description       string     `json:"description"`
		OutletID          string     `json:"outlet_id"`
		ExpiresAt         *time.Time `json:"expires_at"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body: unknown or malformed field", false)
		return
	}
	hash, err := idempotency.HashJSON(raw, r.Method, "/v1/payments")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body", false)
		return
	}

	p := PrincipalFrom(r.Context()) // tenant ONLY from the authenticated identity
	out, err := d.PaymentFlow.Create(r.Context(), payment.CreateRequest{
		MerchantID: p.MerchantID, OutletID: body.OutletID, MerchantReference: body.MerchantReference,
		Description: body.Description, Amount: body.Amount, Currency: body.Currency, PaymentMethod: body.PaymentMethod,
		ExpiresAt: body.ExpiresAt, IdempotencyKey: key, RequestHash: hash,
	})
	switch {
	case err == nil:
	case errors.Is(err, payment.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), false)
		return
	case errors.Is(err, payment.ErrMethodUnavailable), errors.Is(err, payment.ErrNoPricing):
		writeError(w, r, http.StatusUnprocessableEntity, "PAYMENT_METHOD_UNAVAILABLE", "payment method is not available for this merchant", false)
		return
	case errors.Is(err, payment.ErrMerchantNotActive):
		writeError(w, r, http.StatusForbidden, "MERCHANT_NOT_ACTIVE", "merchant is not active", false)
		return
	case errors.Is(err, idempotency.ErrConflict):
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency-Key was used with a different request", false)
		return
	case errors.Is(err, idempotency.ErrInProgress):
		w.Header().Set("Retry-After", "2")
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "a request with this Idempotency-Key is still being processed", true)
		return
	default:
		d.Log.Error("create payment", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}

	pay := out.Payment
	switch pay.Status {
	case payment.Failed:
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": apiError{Code: "GATEWAY_REJECTED", Message: "the payment gateway rejected the payment",
				RequestID: middleware.GetReqID(r.Context()), Retryable: false},
			"payment": pay})
	case payment.Unknown:
		// Outcome ambiguous: tell the client to poll, never to blindly create again.
		writeJSON(w, http.StatusAccepted, pay)
	default:
		code := http.StatusCreated
		if out.Replayed {
			code = http.StatusOK
		}
		writeJSON(w, code, pay)
	}
}
