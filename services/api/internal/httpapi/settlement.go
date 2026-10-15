package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/settlement"
)

func (d Deps) settleActor(r *http.Request) settlement.Actor {
	return settlement.Actor{UserID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context())}
}

func (d Deps) intQuery(w http.ResponseWriter, r *http.Request, name string, max int) (int, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > max {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", name+" must be 1.."+strconv.Itoa(max), false)
		return 0, false
	}
	return n, true
}

// getBalance: the merchant's own split, tenant from the principal only.
func (d Deps) getBalance(w http.ResponseWriter, r *http.Request) {
	cur := r.URL.Query().Get("currency")
	if cur == "" {
		cur = "IDR"
	}
	if len(cur) != 3 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "currency must be a 3-letter code", false)
		return
	}
	b, err := d.Settlement.Balance(r.Context(), PrincipalFrom(r.Context()).MerchantID, cur)
	if err != nil {
		d.Log.Error("balance", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (d Deps) recordSettlementBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Gateway        string   `json:"gateway"`
		BatchRef       string   `json:"batch_ref"`
		Currency       string   `json:"currency"`
		SettlementDate string   `json:"settlement_date"`
		ReportedNet    int64    `json:"reported_net"`
		PaymentIDs     []string `json:"payment_ids"`
		Note           string   `json:"note"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body", false)
		return
	}
	date, err := time.Parse("2006-01-02", body.SettlementDate)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "settlement_date must be YYYY-MM-DD", false)
		return
	}
	for _, id := range body.PaymentIDs {
		if !validUUID(id) {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "payment_ids must be uuids", false)
			return
		}
	}
	b, err := d.Settlement.RecordBatch(r.Context(), settlement.BatchInput{
		Gateway: body.Gateway, BatchRef: body.BatchRef, Currency: body.Currency, SettlementDate: date,
		ReportedNet: body.ReportedNet, PaymentIDs: body.PaymentIDs, Note: body.Note}, d.settleActor(r))
	var mm *settlement.Mismatch
	switch {
	case err == nil:
		code := http.StatusCreated
		if b.Replayed {
			code = http.StatusOK
		}
		writeJSON(w, code, b)
	case errors.As(err, &mm):
		// Nothing was booked; an exception is open for review.
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": apiError{"SETTLEMENT_MISMATCH",
			"reported net differs from the expected net; an exception was opened", middleware.GetReqID(r.Context()), false},
			"reported_net": mm.Reported, "expected_net": mm.Expected, "difference": mm.Reported - mm.Expected})
	case errors.Is(err, settlement.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid settlement batch", false)
	case errors.Is(err, settlement.ErrIneligible):
		writeError(w, r, http.StatusUnprocessableEntity, "SETTLEMENT_INELIGIBLE",
			"some payments are unknown, not succeeded, of another gateway/currency, or already settled", false)
	case errors.Is(err, settlement.ErrBatchConflict):
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "batch_ref already recorded with different content", false)
	default:
		d.Log.Error("record settlement batch", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
}

func (d Deps) internalErr(w http.ResponseWriter, r *http.Request, what string, err error) {
	d.Log.Error(what, "err", err)
	writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
}

func (d Deps) listUnsettled(w http.ResponseWriter, r *http.Request) {
	limit, ok := d.intQuery(w, r, "limit", settlement.MaxBatchPayments)
	if !ok {
		return
	}
	q := r.URL.Query()
	rows, err := d.Settlement.ListUnsettled(r.Context(), q.Get("gateway"), q.Get("currency"), limit)
	if err != nil {
		d.internalErr(w, r, "list unsettled", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// listMyUnsettled: tampilan tenant; tenant hanya dari principal, tidak pernah dari query.
func (d Deps) listMyUnsettled(w http.ResponseWriter, r *http.Request) {
	limit, ok := d.intQuery(w, r, "limit", settlement.MaxBatchPayments)
	if !ok {
		return
	}
	rows, err := d.Settlement.ListUnsettledFor(r.Context(), PrincipalFrom(r.Context()).MerchantID, r.URL.Query().Get("currency"), limit)
	if err != nil {
		d.internalErr(w, r, "list my unsettled", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (d Deps) listSettlementBatches(w http.ResponseWriter, r *http.Request) {
	limit, ok := d.intQuery(w, r, "limit", 200)
	if !ok {
		return
	}
	rows, err := d.Settlement.ListBatches(r.Context(), r.URL.Query().Get("gateway"), limit)
	if err != nil {
		d.internalErr(w, r, "list batches", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (d Deps) listExceptions(w http.ResponseWriter, r *http.Request) {
	limit, ok := d.intQuery(w, r, "limit", 200)
	if !ok {
		return
	}
	st := r.URL.Query().Get("status")
	if st != "" && st != "open" && st != "resolved" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "status must be open or resolved", false)
		return
	}
	rows, err := d.Settlement.ListExceptions(r.Context(), st, limit)
	if err != nil {
		d.internalErr(w, r, "list exceptions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (d Deps) resolveException(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Note string `json:"note"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Note) < 3 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "note (3-1000 chars) is required", false)
		return
	}
	if !validUUID(id) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
		return
	}
	switch err := d.Settlement.ResolveException(r.Context(), id, body.Note, d.settleActor(r)); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, settlement.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found or already resolved", false)
	case errors.Is(err, settlement.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "note (3-1000 chars) is required", false)
	default:
		d.internalErr(w, r, "resolve exception", err)
	}
}
