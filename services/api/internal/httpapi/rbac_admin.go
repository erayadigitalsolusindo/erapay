package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
)

type strPtr *string

func (d Deps) adminListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := d.Auth.ListRoles(r.Context())
	if err != nil {
		d.Log.Error("rbac.roles.list", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": roles})
}

func (d Deps) adminCreateRole(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code  string `json:"code"`
		Scope string `json:"scope"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	role, err := d.Auth.CreateRole(r.Context(), body.Code, body.Scope, p.UserID, middleware.GetReqID(r.Context()))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), false)
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (d Deps) adminUpdateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Code  *string `json:"code"`
		Scope *string `json:"scope"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	role, err := d.Auth.UpdateRole(r.Context(), id, body.Code, body.Scope, p.UserID, middleware.GetReqID(r.Context()))
	if err != nil {
		nf := strings.Contains(strings.ToLower(err.Error()), "no rows")
		status := http.StatusBadRequest
		if nf {
			status = http.StatusNotFound
		}
		writeError(w, r, status, "VALIDATION_ERROR", err.Error(), false)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (d Deps) adminDeleteRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if err := d.Auth.DeleteRole(r.Context(), id, p.UserID, middleware.GetReqID(r.Context())); err != nil {
		d.Log.Error("rbac.roles.delete", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) adminListPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := d.Auth.ListPermissions(r.Context())
	if err != nil {
		d.Log.Error("rbac.permissions.list", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": perms})
}

func (d Deps) adminGrantRolePermission(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RoleID         string `json:"role_id"`
		PermissionCode string `json:"permission_code"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	if body.RoleID == "" || body.PermissionCode == "" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "role_id and permission_code required", false)
		return
	}
	p := PrincipalFrom(r.Context())
	if err := d.Auth.GrantRolePermission(r.Context(), body.RoleID, body.PermissionCode, p.UserID, middleware.GetReqID(r.Context())); err != nil {
		d.Log.Error("rbac.role_permissions.grant", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	// support revoke via granted=false? handled by separate endpoint
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) adminRevokeRolePermission(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RoleID         string `json:"role_id"`
		PermissionCode string `json:"permission_code"`
	}
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if err := d.Auth.RevokeRolePermission(r.Context(), body.RoleID, body.PermissionCode, p.UserID, middleware.GetReqID(r.Context())); err != nil {
		d.Log.Error("rbac.role_permissions.revoke", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) adminSetUserRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		MerchantID string   `json:"merchant_id"`
		RoleIDs    []string `json:"role_ids"`
	}
	// allow DisallowUnknownFields? body may use role_ids. keep strict.
	if _, ok := decodeStrict(w, r, 4<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	if err := d.Auth.SetUserRoles(r.Context(), userID, body.MerchantID, body.RoleIDs, p.UserID, middleware.GetReqID(r.Context())); err != nil {
		d.Log.Error("rbac.user_roles.set", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) adminListRBACUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	users, err := d.Auth.ListUsers(r.Context(), q)
	if err != nil {
		d.Log.Error("rbac.users.list", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": users})
}

func (d Deps) adminListMenuItems(w http.ResponseWriter, r *http.Request) {
	items, err := d.Auth.ListMenuItems(r.Context())
	if err != nil {
		d.Log.Error("rbac.menus.list", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	access, err := d.Auth.ListMenuAccess(r.Context())
	if err != nil {
		d.Log.Error("rbac.menus.access", "err", err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
		return
	}
	roles, _ := d.Auth.ListRoles(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"data":   items,
		"roles":  roles,
		"access": access,
	})
}

func (d Deps) adminSetMenuPermissions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			MenuID  string `json:"menu_id"`
			RoleID  string `json:"role_id"`
			Allowed bool   `json:"allowed"`
		} `json:"items"`
	}
	if _, ok := decodeStrict(w, r, 64<<10, &body); !ok {
		return
	}
	p := PrincipalFrom(r.Context())
	for _, it := range body.Items {
		if it.MenuID == "" || it.RoleID == "" {
			continue
		}
		if err := d.Auth.SetMenuAccess(r.Context(), it.MenuID, it.RoleID, it.Allowed, p.UserID, middleware.GetReqID(r.Context())); err != nil {
			d.Log.Error("rbac.menu_access.set", "err", err)
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error", true)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// keep auth import used
var _ = auth.RBACRole{}
var _ = json.NewEncoder
var _ = chi.URLParam
