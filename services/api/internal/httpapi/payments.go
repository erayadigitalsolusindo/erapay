package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

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
