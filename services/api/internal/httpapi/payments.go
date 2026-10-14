package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
)

// requireAnyAuth accepts an API key (POS/integrator) or a dashboard session.
func (d Deps) requireAnyAuth(next http.Handler) http.Handler {
	byKey, bySession := d.requireAPIKey(next), d.requireAuth(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			byKey.ServeHTTP(w, r)
			return
		}
		bySession.ServeHTTP(w, r)
	})
}

func (d Deps) getPayment(w http.ResponseWriter, r *http.Request) {
	p, err := d.Payments.Get(r.Context(), PrincipalFrom(r.Context()).MerchantID, chi.URLParam(r, "id"))
	if errors.Is(err, payment.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment not found", false)
		return
	}
	if err != nil {
		d.Log.Error("get payment", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// paymentTimeline implements GET /v1/payments/{id}/timeline: the append-only log of
// every transition attempt, oldest first. Read-only - it reports what happened and
// never re-applies anything, so a conflicting gateway report (applied=false) is shown
// as evidence rather than acted upon (PRD §0 #2).
func (d Deps) paymentTimeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	events, err := d.Payments.Events(r.Context(), PrincipalFrom(r.Context()).MerchantID, id)
	switch {
	case errors.Is(err, payment.ErrNotFound):
		// Also covers another tenant's payment: the query is scoped by merchant id.
		writeError(w, r, http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment not found", false)
		return
	case err != nil:
		d.Log.Error("payment timeline", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	if events == nil { // never serialise a null where a list is expected
		events = []payment.Event{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": events})
}

var wib = func() *time.Location {
	l, err := time.LoadLocation(payment.ReportTimezone)
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return l
}()

// parseTimeParam accepts RFC 3339, or a date (YYYY-MM-DD) read as a day in WIB. endOfDay turns a bare
// date into the start of the NEXT day, so `to=2026-10-12` includes the whole of that day.
func parseTimeParam(v string, endOfDay bool) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	t, err := time.ParseInLocation("2006-01-02", v, wib)
	if err != nil {
		return time.Time{}, false
	}
	if endOfDay {
		t = t.AddDate(0, 0, 1)
	}
	return t, true
}

// listPayments implements GET /v1/payments (tenant from the principal only).
func (d Deps) listPayments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := payment.ListFilter{MerchantID: PrincipalFrom(r.Context()).MerchantID, Status: payment.Status(q.Get("status")),
		Reference: q.Get("merchant_reference"), Cursor: q.Get("cursor")}
	if len(f.Reference) > 64 || (f.Status != "" && !payment.ValidStatus(f.Status)) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid status or merchant_reference", false)
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
	for _, p := range []struct {
		name string
		dst  **time.Time
		end  bool
	}{{"from", &f.From, false}, {"to", &f.To, true}} {
		if v := q.Get(p.name); v != "" {
			t, ok := parseTimeParam(v, p.end)
			if !ok {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", p.name+" must be RFC 3339 or YYYY-MM-DD", false)
				return
			}
			*p.dst = &t
		}
	}
	items, next, err := d.Payments.List(r.Context(), f)
	switch {
	case errors.Is(err, payment.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), false)
		return
	case err != nil:
		d.Log.Error("list payments", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	resp := map[string]any{"data": items}
	if next != "" {
		resp["next_cursor"] = next
	}
	writeJSON(w, http.StatusOK, resp)
}

// paymentInquiry implements POST /v1/payments/{id}/inquiry: the manual "cek status"
// button on the transaction screen. It re-asks the gateway (DOKU) for the CURRENT
// status of a pending/unknown payment based on its reference number, and applies
// the result with source=inquiry — exactly what the background poller does, but on
// demand. Money rules (amount check, state machine, source=inquiry) live in Go.
func (d Deps) paymentInquiry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	principal := PrincipalFrom(r.Context())
	// Ownership first: same lookup as getPayment so another tenant's id is a 404,
	// not a signal. Inquire itself is scoped by payment id.
	if _, err := d.Payments.Get(r.Context(), principal.MerchantID, id); err != nil {
		if errors.Is(err, payment.ErrNotFound) {
			writeError(w, r, http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment not found", false)
			return
		}
		d.Log.Error("inquiry lookup", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	err := d.PaymentFlow.Inquire(r.Context(), id)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case errors.Is(err, payment.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment not found", false)
	case errors.Is(err, gateway.ErrNotSupported):
		writeError(w, r, http.StatusServiceUnavailable, "INQUIRY_NOT_SUPPORTED", "gateway does not support status inquiry", false)
	case errors.Is(err, payment.ErrAmountMismatch):
		writeError(w, r, http.StatusConflict, "AMOUNT_MISMATCH", "gateway reports success with a different amount; review manually", false)
	default:
		d.Log.Error("payment inquiry", "err", err)
		writeError(w, r, http.StatusBadGateway, "INQUIRY_FAILED", "gateway did not answer; try again later", true)
	}
}

// reportSummary implements GET /v1/reports/summary?from=&to=&currency= (dates in WIB, inclusive).
func (d Deps) reportSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cur := q.Get("currency")
	if cur == "" {
		cur = "IDR"
	}
	today := time.Now().In(wib)
	from, to := today.AddDate(0, 0, -29), today
	for _, p := range []struct {
		name string
		dst  *time.Time
	}{{"from", &from}, {"to", &to}} {
		if v := q.Get(p.name); v != "" {
			t, err := time.ParseInLocation("2006-01-02", v, wib)
			if err != nil {
				writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", p.name+" must be YYYY-MM-DD", false)
				return
			}
			*p.dst = t
		}
	}
	if len(cur) != 3 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "currency must be a 3-letter code", false)
		return
	}
	s, err := d.Payments.Summarize(r.Context(), PrincipalFrom(r.Context()).MerchantID, cur, from, to)
	switch {
	case errors.Is(err, payment.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), false)
	case err != nil:
		d.Log.Error("summary", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	default:
		writeJSON(w, http.StatusOK, s)
	}
}
