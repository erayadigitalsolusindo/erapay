package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
)

// requireAPIKey authenticates "Authorization: Bearer epk_..." and stores a
// Principal whose tenant and permissions come from the key row only.
func (d Deps) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		tok, ok := strings.CutPrefix(h, "Bearer ")
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "api key required", false)
			return
		}
		id, err := d.APIKeys.Verify(r.Context(), strings.TrimSpace(tok))
		if errors.Is(err, apikey.ErrInvalidKey) {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid api key", false)
			return
		}
		if err != nil {
			d.Log.Error("verify api key", "err", err)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
			return
		}
		p := &auth.Principal{KeyID: id.KeyID, MerchantID: id.MerchantID, Permissions: map[string]struct{}{}}
		for _, s := range id.Scopes {
			p.Permissions[s] = struct{}{}
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, p)
		ctx = context.WithValue(ctx, keyIdentityCtx{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type keyIdentityCtx struct{}

func (d Deps) apiKeySelf(w http.ResponseWriter, r *http.Request) {
	id, _ := r.Context().Value(keyIdentityCtx{}).(*apikey.Identity)
	writeJSON(w, http.StatusOK, map[string]any{
		"key_id": id.KeyID, "prefix": id.Prefix, "merchant_id": id.MerchantID,
		"environment": id.Environment, "scopes": id.Scopes,
	})
}

func (d Deps) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		len(strings.TrimSpace(body.Name)) == 0 || len(body.Name) > 80 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "name (1-80 chars) and scopes are required", false)
		return
	}
	p := PrincipalFrom(r.Context())
	if err := apikey.ValidateScopes(body.Scopes, p.Has); err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "scopes must be allowed and held by the creator", false)
		return
	}
	if body.ExpiresAt != nil && !body.ExpiresAt.After(time.Now()) {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "expires_at must be in the future", false)
		return
	}
	k, err := d.APIKeys.Create(r.Context(), apikey.CreateInput{
		MerchantID: p.MerchantID, CreatedBy: p.UserID, Name: strings.TrimSpace(body.Name),
		Scopes: body.Scopes, ExpiresAt: body.ExpiresAt, RequestID: middleware.GetReqID(r.Context())})
	if err != nil {
		d.Log.Error("create api key", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, k)
}

func (d Deps) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := d.APIKeys.List(r.Context(), PrincipalFrom(r.Context()).MerchantID)
	if err != nil {
		d.Log.Error("list api keys", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": keys})
}

func (d Deps) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	err := d.APIKeys.Revoke(r.Context(), p.MerchantID, chi.URLParam(r, "id"), p.UserID, "revoked by user", middleware.GetReqID(r.Context()))
	if !d.keyErr(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GraceSeconds int `json:"grace_seconds"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); (err != nil && !errors.Is(err, io.EOF)) || body.GraceSeconds < 0 ||
		time.Duration(body.GraceSeconds)*time.Second > apikey.MaxRotateGrace {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "grace_seconds must be 0..86400", false)
		return
	}
	p := PrincipalFrom(r.Context())
	k, err := d.APIKeys.Rotate(r.Context(), p.MerchantID, chi.URLParam(r, "id"), p.UserID,
		time.Duration(body.GraceSeconds)*time.Second, middleware.GetReqID(r.Context()))
	if !d.keyErr(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, k)
}

// keyErr writes an error response and returns false, or returns true when err is nil.
func (d Deps) keyErr(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, apikey.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", false)
	default:
		d.Log.Error("api key", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
	}
	return false
}
