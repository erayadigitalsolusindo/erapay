package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gwcred"
)

// gwCredErr maps gwcred errors to responses; returns true when err was nil.
func (d Deps) gwCredErr(w http.ResponseWriter, r *http.Request, err error) bool {
	var ve *gwcred.ValidationError
	switch {
	case err == nil:
		return true
	case errors.As(err, &ve):
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", ve.Msg, false)
	case errors.Is(err, gwcred.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "gateway or environment not found", false)
	case errors.Is(err, gwcred.ErrConflict):
		writeError(w, r, http.StatusConflict, "VERSION_CONFLICT", "kredensial sudah diubah orang lain; muat ulang halaman lalu coba lagi", false)
	case errors.Is(err, gwcred.ErrWrongEnv):
		writeError(w, r, http.StatusConflict, "ENVIRONMENT_MISMATCH", "this server runs in a different environment; apply it from that environment's server", false)
	case errors.Is(err, gwcred.ErrIncomplete):
		writeError(w, r, http.StatusUnprocessableEntity, "CREDENTIALS_INCOMPLETE", "lengkapi semua field wajib dulu", false)
	case errors.Is(err, gwcred.ErrNoFactory):
		writeError(w, r, http.StatusUnprocessableEntity, "APPLY_UNSUPPORTED", "this gateway cannot be applied live on this server", false)
	default:
		d.Log.Error("gwcred", "err", err, "request_id", middleware.GetReqID(r.Context()))
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}

// platformGatewayCreds: GET /v1/platform/gateway-credentials
// Every gateway with its form schema and, per environment, the masked stored state. Secrets are never returned.
func (d Deps) platformGatewayCreds(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		gwcred.Schema
		Environments []gwcred.Status `json:"environments"`
	}
	out := []entry{}
	for _, sc := range gwcred.Schemas() {
		e := entry{Schema: sc}
		for _, env := range []string{"sandbox", "production"} {
			st, err := d.GWCreds.Status(r.Context(), sc.Gateway, env)
			if !d.gwCredErr(w, r, err) {
				return
			}
			e.Environments = append(e.Environments, st)
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"server_environment": d.GWCreds.Env(), "gateways": out})
}

// platformSetGatewayCreds: PUT /v1/platform/gateway-credentials/{gateway}/{env}
// Saves (merges) credentials; does not change what the server uses until /apply.
func (d Deps) platformSetGatewayCreds(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Values  map[string]string `json:"values"`
		Clear   []string          `json:"clear"`
		Version int               `json:"version"`
		Note    string            `json:"note"`
	}
	if _, ok := decodeStrict(w, r, 64<<10, &body); !ok {
		return
	}
	st, err := d.GWCreds.Set(r.Context(), gwcred.SetInput{
		Gateway: chi.URLParam(r, "gateway"), Environment: chi.URLParam(r, "env"), Values: body.Values, Clear: body.Clear,
		ExpectVersion: body.Version, Note: body.Note, ActorID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context()),
	})
	if !d.gwCredErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (d Deps) gwCredAct(r *http.Request) gwcred.ActInput {
	return gwcred.ActInput{Gateway: chi.URLParam(r, "gateway"), Environment: chi.URLParam(r, "env"),
		ActorID: PrincipalFrom(r.Context()).UserID, RequestID: middleware.GetReqID(r.Context())}
}

// platformTestGatewayCreds: POST /v1/platform/gateway-credentials/{gateway}/{env}/test
func (d Deps) platformTestGatewayCreds(w http.ResponseWriter, r *http.Request) {
	res, err := d.GWCreds.Test(r.Context(), d.gwCredAct(r))
	if !d.gwCredErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// platformApplyGatewayCreds: POST /v1/platform/gateway-credentials/{gateway}/{env}/apply
// Swaps the live adapter only when the connection test passes.
func (d Deps) platformApplyGatewayCreds(w http.ResponseWriter, r *http.Request) {
	res, applied, err := d.GWCreds.Apply(r.Context(), d.gwCredAct(r))
	if !d.gwCredErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"applied": applied, "ok": res.OK, "steps": res.Steps})
}
