package httpapi

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/webhook"
)

// captureWebhook stores the raw request and acknowledges it. It deliberately does NOT
// parse the body or change any payment: DOKU's QRIS notification format and signature
// token source are unconfirmed, so status stays driven by the inquiry poller.
// The path is a catch-all under /webhooks/{gateway} so whatever URL DOKU is given works.
// Bodies and headers are never logged (PII/secrets); only the capture id context is.
func (d Deps) captureWebhook(w http.ResponseWriter, r *http.Request) {
	gw := chi.URLParam(r, "gateway")
	body, err := io.ReadAll(io.LimitReader(r.Body, webhook.MaxBody+1))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "unreadable body", false)
		return
	}
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
	writeJSON(w, http.StatusOK, map[string]string{"responseCode": "2005600", "responseMessage": "Successful"})
}

func (d Deps) mountWebhooks(r chi.Router) {
	r.Handle("/webhooks/{gateway}", http.HandlerFunc(d.captureWebhook))
	r.Handle("/webhooks/{gateway}/*", http.HandlerFunc(d.captureWebhook))
}
