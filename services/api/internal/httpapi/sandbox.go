package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
)

// Sandbox twin of the signed-in merchant (docs/prd/sandbox.md). The dashboard session stays on the real
// merchant; the twin is managed through these routes, so nobody lands in sandbox by accident.

func (d Deps) sandboxStatus(w http.ResponseWriter, r *http.Request) {
	id, err := d.Merchants.SandboxOf(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if err != nil {
		d.Log.Error("sandbox status", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": d.APIKeys.HostsSandbox(), "enabled": id != "", "merchant_id": id,
		"active": id != "" && id == PrincipalFrom(r.Context()).MerchantID, "simulator": d.SandboxGateways != nil && d.hasSimulator()})
}

// hasSimulator: some sandbox adapter can simulate payments, so the dashboard can mention the simulator.
func (d Deps) hasSimulator() bool {
	lister, ok := d.SandboxGateways.(interface{ Codes() []string }) // *gateway.Registry; test doubles may not list
	if !ok {
		return false
	}
	for _, code := range lister.Codes() {
		if ad, ok := d.SandboxGateways.Get(code); ok {
			if _, sim := ad.(gateway.Simulator); sim {
				return true
			}
		}
	}
	return false
}

// switchSandbox implements POST /v1/sandbox/switch {"mode":"sandbox"|"live"}: the dashboard session works on the
// sandbox twin or on the real merchant from now on. The tenant is still decided by the server from the session.
func (d Deps) switchSandbox(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Mode != "sandbox" && body.Mode != "live") {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "mode must be sandbox or live", false)
		return
	}
	id, err := d.Auth.SwitchSandbox(r.Context(), PrincipalFrom(r.Context()), body.Mode == "sandbox", middleware.GetReqID(r.Context()))
	switch {
	case errors.Is(err, auth.ErrNoSandbox):
		writeError(w, r, http.StatusConflict, "SANDBOX_NOT_ENABLED", "sandbox is not enabled", false)
		return
	case errors.Is(err, auth.ErrSwitchNotAllowed):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "this session cannot switch", false)
		return
	case err != nil:
		d.Log.Error("switch sandbox", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": body.Mode, "merchant_id": id})
}

func (d Deps) enableSandbox(w http.ResponseWriter, r *http.Request) {
	if !d.APIKeys.HostsSandbox() {
		writeError(w, r, http.StatusConflict, "SANDBOX_UNAVAILABLE", "this server is already a sandbox environment", false)
		return
	}
	p := PrincipalFrom(r.Context())
	id, created, err := d.Merchants.EnableSandbox(r.Context(), p.MerchantID, p.UserID, middleware.GetReqID(r.Context()))
	switch {
	case errors.Is(err, merchant.ErrNotApproved):
		writeError(w, r, http.StatusConflict, "MERCHANT_NOT_APPROVED", "merchant must be approved before enabling sandbox", false)
		return
	case err != nil:
		d.Log.Error("enable sandbox", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, map[string]any{"enabled": true, "merchant_id": id})
}

// sandboxMerchant resolves the twin for the key routes; 404 when sandbox was never enabled.
func (d Deps) sandboxMerchant(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := d.Merchants.SandboxOf(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if err != nil {
		d.Log.Error("sandbox lookup", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return "", false
	}
	if id == "" {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "sandbox is not enabled", false)
		return "", false
	}
	return id, true
}

func (d Deps) createSandboxAPIKey(w http.ResponseWriter, r *http.Request) {
	if id, ok := d.sandboxMerchant(w, r); ok {
		d.issueAPIKey(w, r, id)
	}
}

func (d Deps) listSandboxAPIKeys(w http.ResponseWriter, r *http.Request) {
	if id, ok := d.sandboxMerchant(w, r); ok {
		d.listKeysOf(w, r, id)
	}
}

func (d Deps) revokeSandboxAPIKey(w http.ResponseWriter, r *http.Request) {
	if id, ok := d.sandboxMerchant(w, r); ok {
		d.revokeKeyOf(w, r, id)
	}
}

// simulatePayment implements POST /v1/sandbox/payments/{id}/simulate {"outcome":"paid|expired|failed"}.
// Sandbox API keys only. It applies the outcome through the same path as a verified webhook (ledger, outbound
// webhooks, idempotency), so integrators exercise their whole flow without scanning a QR.
func (d Deps) simulatePayment(w http.ResponseWriter, r *http.Request) {
	if id, _ := r.Context().Value(keyIdentityCtx{}).(*apikey.Identity); id == nil || id.Environment != "sandbox" {
		writeError(w, r, http.StatusForbidden, "SANDBOX_KEY_REQUIRED", "a sandbox API key (epk_sbx_) is required", false)
		return
	}
	var body struct {
		Outcome string `json:"outcome"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "outcome must be paid, expired or failed", false)
		return
	}
	merchantID := PrincipalFrom(r.Context()).MerchantID
	p, err := d.Payments.Get(r.Context(), merchantID, chi.URLParam(r, "id"))
	if errors.Is(err, payment.ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment not found", false)
		return
	}
	if err != nil {
		d.Log.Error("simulate: get payment", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	var sim gateway.Simulator
	if d.SandboxGateways != nil {
		if ad, ok := d.SandboxGateways.Get(p.Gateway); ok {
			sim, _ = ad.(gateway.Simulator)
		}
	}
	if sim == nil {
		writeError(w, r, http.StatusConflict, "NOT_SIMULATABLE", "this payment's gateway cannot be simulated", false)
		return
	}
	ev, err := sim.SimulateEvent(p.ID, p.Amount, p.Currency, body.Outcome)
	if errors.Is(err, gateway.ErrUnknownOutcome) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "outcome must be paid, expired or failed", false)
		return
	}
	res, err := d.PaymentFlow.ApplyWebhookSandbox(r.Context(), p.Gateway, ev)
	if err != nil {
		d.Log.Error("simulate: apply", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	cur, err := d.Payments.Get(r.Context(), merchantID, p.ID)
	if err != nil {
		d.Log.Error("simulate: reload payment", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payment": cur, "applied": res.Applied})
}
