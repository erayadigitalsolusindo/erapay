package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/outbound"
)

func (d Deps) actor(r *http.Request) outbound.Actor {
	return outbound.Actor{UserID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context())}
}

// outboundErr writes the error response and returns false, or returns true when err is nil.
func (d Deps) outboundErr(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, outbound.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
	case errors.Is(err, outbound.ErrBadURL), errors.Is(err, outbound.ErrBlockedAddress):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "url must be a public https endpoint", false)
	case errors.Is(err, outbound.ErrTooManyEndpts):
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "endpoint limit reached", false)
	case errors.Is(err, outbound.ErrNotRedeliver):
		writeError(w, r, http.StatusConflict, "VALIDATION_ERROR", "delivery is still pending", false)
	default:
		d.Log.Error("webhook endpoints", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

func (d Deps) createWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "url is required", false)
		return
	}
	e, err := d.Outbound.CreateEndpoint(r.Context(), PrincipalFrom(r.Context()).MerchantID, body.URL, d.actor(r))
	if !d.outboundErr(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store") // the signing secret is shown exactly once
	writeJSON(w, http.StatusCreated, e)
}

func (d Deps) listWebhookEndpoints(w http.ResponseWriter, r *http.Request) {
	es, err := d.Outbound.ListEndpoints(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if !d.outboundErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": es})
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validUUID(s string) bool { return uuidRe.MatchString(s) }

func (d Deps) rotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
		return
	}
	e, err := d.Outbound.RotateSecret(r.Context(), PrincipalFrom(r.Context()).MerchantID, id, d.actor(r))
	if !d.outboundErr(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, e)
}

func (d Deps) disableWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
		return
	}
	err := d.Outbound.Disable(r.Context(), PrincipalFrom(r.Context()).MerchantID, id, d.actor(r))
	if !d.outboundErr(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := outbound.DeliveryFilter{EndpointID: q.Get("endpoint_id"), Status: q.Get("status")}
	if f.EndpointID != "" && !validUUID(f.EndpointID) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "endpoint_id must be a uuid", false)
		return
	}
	switch f.Status {
	case "", "pending", "succeeded", "dead", "cancelled":
	default:
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "unknown status", false)
		return
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be 1..100", false)
			return
		}
		f.Limit = n
	}
	if v := q.Get("cursor"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid cursor", false)
			return
		}
		f.Before = &t
	}
	ds, err := d.Outbound.ListDeliveries(r.Context(), PrincipalFrom(r.Context()).MerchantID, f)
	if !d.outboundErr(w, r, err) {
		return
	}
	resp := map[string]any{"data": ds}
	if len(ds) > 0 && (f.Limit == 0 && len(ds) == 50 || f.Limit > 0 && len(ds) == f.Limit) {
		resp["next_cursor"] = ds[len(ds)-1].CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (d Deps) redeliverWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
		return
	}
	nd, err := d.Outbound.Redeliver(r.Context(), PrincipalFrom(r.Context()).MerchantID, id, d.actor(r))
	if !d.outboundErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusAccepted, nd)
}
