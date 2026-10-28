package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/routing"
)

// routingErr maps routing errors to responses; returns true when err was nil.
func (d Deps) routingErr(w http.ResponseWriter, r *http.Request, err error) bool {
	var ve *routing.ValidationError
	switch {
	case err == nil:
		return true
	case errors.As(err, &ve):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", ve.Msg, false)
	case errors.Is(err, routing.ErrConflict):
		writeError(w, r, http.StatusConflict, "VERSION_CONFLICT", "jalur sudah diubah orang lain; muat ulang halaman lalu coba lagi", false)
	case errors.Is(err, routing.ErrWrongEnv):
		writeError(w, r, http.StatusConflict, "ENVIRONMENT_MISMATCH", "this server runs in a different environment; change it from that environment's server", false)
	default:
		d.Log.Error("routing", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

// platformPaymentRouting: GET /v1/platform/payment-routing
// The gateway chosen for new payments (or none) and which gateways can be chosen right now.
func (d Deps) platformPaymentRouting(w http.ResponseWriter, r *http.Request) {
	ov, err := d.Routing.Overview(r.Context())
	if !d.routingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

// platformSetPaymentRouting: PUT /v1/platform/payment-routing
// Takes effect for the next payment; payments already created keep their gateway.
func (d Deps) platformSetPaymentRouting(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Environment string `json:"environment"`
		Gateway     string `json:"gateway"`
		Version     int    `json:"version"`
		Note        string `json:"note"`
	}
	if _, ok := decodeStrict(w, r, 16<<10, &body); !ok {
		return
	}
	ov, err := d.Routing.Set(r.Context(), routing.SetInput{Environment: body.Environment, Gateway: body.Gateway,
		ExpectVersion: body.Version, Note: body.Note, ActorID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context())})
	if !d.routingErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, ov)
}
