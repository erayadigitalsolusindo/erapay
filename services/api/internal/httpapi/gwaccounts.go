package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gwaccount"
)

// gwAccountErr maps gwaccount errors to responses; returns true when err was nil.
func (d Deps) gwAccountErr(w http.ResponseWriter, r *http.Request, err error) bool {
	var ve *gwaccount.ValidationError
	switch {
	case err == nil:
		return true
	case errors.As(err, &ve):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", ve.Msg, false)
	case errors.Is(err, gwaccount.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "merchant or gateway account not found", false)
	case errors.Is(err, gwaccount.ErrWrongEnv):
		writeError(w, r, http.StatusConflict, "ENVIRONMENT_MISMATCH", "this server runs in a different environment; test that environment from its own server", false)
	case errors.Is(err, gwaccount.ErrUnsupported):
		writeError(w, r, http.StatusUnprocessableEntity, "TEST_UNSUPPORTED", "this gateway has no connection test", false)
	default:
		d.Log.Error("gwaccount", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

// platformGatewayAccounts: GET /v1/platform/merchants/{id}/gateway-accounts
func (d Deps) platformGatewayAccounts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ov, err := d.GWAccounts.List(r.Context(), id)
	if !d.gwAccountErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

// platformSetGatewayAccount: PUT /v1/platform/merchants/{id}/gateway-accounts
// Create/update the account for (merchant, gateway, environment). A reason is mandatory (audited).
func (d Deps) platformSetGatewayAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Gateway     string `json:"gateway"`
		Environment string `json:"environment"`
		GatewayRef  string `json:"gateway_ref"`
		Status      string `json:"status"`
		Note        string `json:"note"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	a, err := d.GWAccounts.Upsert(r.Context(), gwaccount.UpsertInput{
		MerchantID: id, Gateway: body.Gateway, Environment: body.Environment, GatewayRef: body.GatewayRef,
		Status: body.Status, Note: body.Note, ActorID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.gwAccountErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// platformTestGatewayAccount: POST /v1/platform/merchants/{id}/gateway-accounts/test
// deep=false checks only the server's gateway credentials; deep=true also exercises the merchant's gateway_ref.
func (d Deps) platformTestGatewayAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Gateway     string `json:"gateway"`
		Environment string `json:"environment"`
		Deep        bool   `json:"deep"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	res, err := d.GWAccounts.Test(r.Context(), gwaccount.TestInput{
		MerchantID: id, Gateway: body.Gateway, Environment: body.Environment, Deep: body.Deep,
		ActorID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.gwAccountErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}
