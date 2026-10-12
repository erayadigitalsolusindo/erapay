package httpapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/webhook"
)

// captureWebhook optionally stores the raw request (capture), then, for gateways whose
// notification format is confirmed, verifies the signature and applies the status
// (PRD §0 #2: a validated webhook is an allowed source; inquiry remains the safety net).
// The path is a catch-all under /webhooks/{gateway} so whatever URL DOKU is given works.
// Bodies and headers are never logged (PII/secrets); only the capture id context is.
func (d Deps) captureWebhook(w http.ResponseWriter, r *http.Request) {
	gw := chi.URLParam(r, "gateway")
	body, err := io.ReadAll(io.LimitReader(r.Body, webhook.MaxBody+1))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "unreadable body", false)
		return
	}
	if d.Webhooks != nil {
		stored, err := d.Webhooks.Capture(r.Context(), gw, r, body)
		if err != nil {
			// 5xx makes the gateway retry, so the notification is not lost.
			d.Log.Error("webhook capture failed", "request_id", middleware.GetReqID(r.Context()), "gateway", gw, "err", err)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "capture failed", true)
			return
		}
		if !stored {
			d.Log.Warn("webhook capture cap reached", "gateway", gw)
		}
	}
	if code := d.applyWebhook(r, gw, body); code != http.StatusOK {
		writeError(w, r, code, "WEBHOOK_REJECTED", "notification rejected", code >= 500)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// applyWebhook verifies and applies a notification for gateways with a confirmed format.
// It returns the HTTP status to answer. Only a bad signature (400) or a transient failure
// (500, so the gateway retries) is not 200: an unknown payment, an unmapped status or an
// amount mismatch cannot be fixed by a retry, so they are logged and acknowledged.
func (d Deps) applyWebhook(r *http.Request, gw string, body []byte) int {
	if d.Gateways == nil || d.PaymentFlow == nil {
		return http.StatusOK // capture-only
	}
	ad, ok := d.Gateways.Get(gw)
	if !ok {
		return http.StatusOK
	}
	v, ok := ad.(gateway.WebhookVerifier)
	if !ok {
		return http.StatusOK
	}
	rid := middleware.GetReqID(r.Context())
	ev, err := v.VerifyWebhook(r.Header, r.URL.Path, body)
	switch {
	case errors.Is(err, gateway.ErrInvalidSignature):
		d.Log.Warn("webhook signature rejected", "request_id", rid, "gateway", gw)
		return http.StatusBadRequest
	case err != nil:
		d.Log.Warn("webhook authentic but unusable", "request_id", rid, "gateway", gw, "err", err)
		return http.StatusOK
	}
	res, err := d.PaymentFlow.ApplyWebhook(r.Context(), gw, ev)
	switch {
	case errors.Is(err, payment.ErrNotFound):
		d.Log.Warn("webhook for unknown payment", "request_id", rid, "gateway", gw)
	case errors.Is(err, payment.ErrAmountMismatch):
		// already logged with details by ApplyWebhook
	case err != nil:
		d.Log.Error("webhook apply failed", "request_id", rid, "gateway", gw, "err", err)
		return http.StatusInternalServerError
	default:
		d.Log.Info("webhook applied", "request_id", rid, "gateway", gw, "payment_id", ev.PaymentRef,
			"applied", res.Applied, "noop", res.NoOp, "duplicate", res.Duplicate, "conflict", res.Conflict)
	}
	return http.StatusOK
}

func (d Deps) mountWebhooks(r chi.Router) {
	r.Handle("/webhooks/{gateway}", http.HandlerFunc(d.captureWebhook))
	r.Handle("/webhooks/{gateway}/*", http.HandlerFunc(d.captureWebhook))
}
