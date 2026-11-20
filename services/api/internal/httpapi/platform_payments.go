package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
)

// platformPayments: GET /v1/platform/payments?status=&q=&limit= — cross-tenant list for the platform panel.
func (d Deps) platformPayments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, err := d.Payments.PlatformList(r.Context(), payment.Status(q.Get("status")), q.Get("q"), limit)
	if errors.Is(err, payment.ErrValidation) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "status tidak dikenal", false)
		return
	}
	if err != nil {
		d.Log.Error("platform.payments", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// platformDeletePayment: DELETE /v1/platform/payments/{id} {reason}. Only a payment that never moved money
// can go; the deletion and who/why are written to the audit log (action payment.purge).
func (d Deps) platformDeletePayment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	if _, ok := decodeStrict(w, r, 16<<10, &body); !ok {
		return
	}
	err := d.Payments.Purge(r.Context(), chi.URLParam(r, "id"), PrincipalFrom(r.Context()).UserID, body.Reason, middleware.GetReqID(r.Context()))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	case errors.Is(err, payment.ErrValidation):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "alasan wajib diisi (3-500 karakter)", false)
	case errors.Is(err, payment.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "PAYMENT_NOT_FOUND", "transaksi tidak ditemukan", false)
	case errors.Is(err, payment.ErrNotPurgeable):
		writeError(w, r, http.StatusConflict, "PAYMENT_NOT_DELETABLE", "transaksi ini pernah menggerakkan uang atau masih bisa dibayar, jadi tidak boleh dihapus", false)
	default:
		d.Log.Error("platform.delete payment", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
}
