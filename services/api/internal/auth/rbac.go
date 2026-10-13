package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

type RBACRole struct {
	ID    string `json:"id"`
	Code  string `json:"code"`
	Scope string `json:"scope"`
}

type RBACPermission struct {
	Code string `json:"code"`
}

type RBACUser struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	Status        string   `json:"status"`
	PlatformRoles []string `json:"platform_roles"`
	MerchantID    string   `json:"merchant_id,omitempty"`
	MerchantRole  string   `json:"merchant_role,omitempty"`
}

type MenuItem struct {
	ID             string  `json:"id"`
	Href           string  `json:"href"`
	Label          string  `json:"label"`
	PermissionCode *string `json:"permission_code"`
	PlatformOnly   bool    `json:"platform_only"`
	SortOrder      int     `json:"sort_order"`
}

type MenuAccess struct {
	MenuID   string `json:"menu_id"`
	RoleID   string `json:"role_id"`
	RoleCode string `json:"role_code"`
	Allowed  bool   `json:"allowed"`
}

func (s *Service) ListRoles(ctx context.Context) ([]RBACRole, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, code, scope FROM roles ORDER BY scope, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RBACRole{}
	for rows.Next() {
		var r RBACRole
		if err := rows.Scan(&r.ID, &r.Code, &r.Scope); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) CreateRole(ctx context.Context, code, scope, actor, requestID string) (RBACRole, error) {
	code = strings.TrimSpace(code)
	if code == "" || (scope != "platform" && scope != "merchant") {
		return RBACRole{}, errors.New("invalid role")
	}
	var id string
	err := s.db.QueryRow(ctx, `INSERT INTO roles (code, scope) VALUES ($1,$2) RETURNING id::text`, code, scope).Scan(&id)
	if err != nil {
		return RBACRole{}, err
	}
	_ = audit.Record(ctx, s.db, audit.Entry{ActorID: actor, Action: "rbac.role.created", EntityType: "role",
		EntityID: id, RequestID: requestID, Metadata: map[string]any{"code": code, "scope": scope}})
	return RBACRole{ID: id, Code: code, Scope: scope}, nil
}

func (s *Service) UpdateRole(ctx context.Context, id string, code *string, scope *string, actor, requestID string) (RBACRole, error) {
	// fetch existing
	var curCode, curScope string
	err := s.db.QueryRow(ctx, `SELECT code, scope FROM roles WHERE id = $1`, id).Scan(&curCode, &curScope)
	if err != nil {
		return RBACRole{}, err
	}
	if code != nil && strings.TrimSpace(*code) != "" {
		curCode = strings.TrimSpace(*code)
	}
	if scope != nil && (*scope == "platform" || *scope == "merchant") {
		curScope = *scope
	}
	_, err = s.db.Exec(ctx, `UPDATE roles SET code = $2, scope = $3 WHERE id = $1`, id, curCode, curScope)
	if err != nil {
		return RBACRole{}, err
	}
	_ = audit.Record(ctx, s.db, audit.Entry{ActorID: actor, Action: "rbac.role.updated", EntityType: "role",
		EntityID: id, RequestID: requestID, Metadata: map[string]any{"code": curCode, "scope": curScope}})
	return RBACRole{ID: id, Code: curCode, Scope: curScope}, nil
}

func (s *Service) DeleteRole(ctx context.Context, id, actor, requestID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// get code for audit
	var code string
	_ = tx.QueryRow(ctx, `SELECT code FROM roles WHERE id = $1`, id).Scan(&code)
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE role_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM merchant_users WHERE role_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM menu_role_access WHERE role_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actor, Action: "rbac.role.deleted", EntityType: "role",
		EntityID: id, RequestID: requestID, Metadata: map[string]any{"code": code}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ListPermissions(ctx context.Context) ([]RBACPermission, error) {
	rows, err := s.db.Query(ctx, `SELECT code FROM permissions ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RBACPermission{}
	for rows.Next() {
		var p RBACPermission
		if err := rows.Scan(&p.Code); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) GrantRolePermission(ctx context.Context, roleID, perm, actor, requestID string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_code) VALUES ($1,$2) ON CONFLICT DO NOTHING`, roleID, perm)
	if err != nil {
		return err
	}
	_ = audit.Record(ctx, s.db, audit.Entry{ActorID: actor, Action: "rbac.role_permission.granted", EntityType: "role",
		EntityID: roleID, RequestID: requestID, Metadata: map[string]any{"permission_code": perm}})
	return nil
}

func (s *Service) RevokeRolePermission(ctx context.Context, roleID, perm, actor, requestID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1 AND permission_code = $2`, roleID, perm)
	if err != nil {
		return err
	}
	_ = audit.Record(ctx, s.db, audit.Entry{ActorID: actor, Action: "rbac.role_permission.revoked", EntityType: "role",
		EntityID: roleID, RequestID: requestID, Metadata: map[string]any{"permission_code": perm}})
	return nil
}

func (s *Service) SetUserRoles(ctx context.Context, userID, merchantID string, roleIDs []string, actor, requestID string) error {
	if merchantID == "" {
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
			return err
		}
		for _, rid := range roleIDs {
			if strings.TrimSpace(rid) == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, rid); err != nil {
				return err
			}
		}
		if err := audit.Record(ctx, tx, audit.Entry{ActorID: actor, Action: "rbac.user_roles.set", EntityType: "user",
			EntityID: userID, RequestID: requestID, Metadata: map[string]any{"role_ids": roleIDs, "merchant_id": nil}}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	// merchant scoped: upsert single role (first) or delete
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if len(roleIDs) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM merchant_users WHERE merchant_id = $1 AND user_id = $2`, merchantID, userID); err != nil {
			return err
		}
	} else {
		rid := roleIDs[0]
		_, err = tx.Exec(ctx, `
			INSERT INTO merchant_users (merchant_id, user_id, role_id) VALUES ($1,$2,$3)
			ON CONFLICT (merchant_id, user_id) DO UPDATE SET role_id = EXCLUDED.role_id`,
			merchantID, userID, rid)
		if err != nil {
			return err
		}
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actor, Action: "rbac.user_roles.set", EntityType: "user",
		EntityID: userID, RequestID: requestID, Metadata: map[string]any{"role_ids": roleIDs, "merchant_id": merchantID}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ListUsers(ctx context.Context, q string) ([]RBACUser, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id::text, u.email, u.status,
			COALESCE((SELECT string_agg(r.code, ',' ORDER BY r.code) FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id), '') AS platform_roles,
			COALESCE((SELECT mu.merchant_id::text FROM merchant_users mu WHERE mu.user_id = u.id ORDER BY mu.merchant_id LIMIT 1), '') AS merchant_id,
			COALESCE((SELECT r.code FROM merchant_users mu JOIN roles r ON r.id = mu.role_id WHERE mu.user_id = u.id ORDER BY mu.merchant_id LIMIT 1), '') AS merchant_role
		FROM users u
		WHERE ($1 = '' OR u.email ILIKE '%' || $1 || '%')
		ORDER BY u.email LIMIT 200`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RBACUser{}
	for rows.Next() {
		var u RBACUser
		var platform string
		var merchantID, merchantRole string
		if err := rows.Scan(&u.ID, &u.Email, &u.Status, &platform, &merchantID, &merchantRole); err != nil {
			return nil, err
		}
		if platform != "" {
			u.PlatformRoles = strings.Split(platform, ",")
		} else {
			u.PlatformRoles = []string{}
		}
		u.MerchantID = merchantID
		u.MerchantRole = merchantRole
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) ListMenuItems(ctx context.Context) ([]MenuItem, error) {
	rows, err := s.db.Query(ctx, `SELECT id, href, label_key, permission_code, platform_only, sort_order FROM menu_items ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MenuItem{}
	for rows.Next() {
		var m MenuItem
		var pc *string
		if err := rows.Scan(&m.ID, &m.Href, &m.Label, &pc, &m.PlatformOnly, &m.SortOrder); err != nil {
			return nil, err
		}
		m.PermissionCode = pc
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) ListMenuAccess(ctx context.Context) ([]MenuAccess, error) {
	rows, err := s.db.Query(ctx, `
		SELECT mi.id, r.id::text, r.code, COALESCE(mra.allowed, false)
		FROM menu_items mi CROSS JOIN roles r
		LEFT JOIN menu_role_access mra ON mra.menu_id = mi.id AND mra.role_id = r.id
		ORDER BY mi.sort_order, mi.id, r.scope, r.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MenuAccess{}
	for rows.Next() {
		var ma MenuAccess
		if err := rows.Scan(&ma.MenuID, &ma.RoleID, &ma.RoleCode, &ma.Allowed); err != nil {
			return nil, err
		}
		out = append(out, ma)
	}
	return out, rows.Err()
}

func (s *Service) SetMenuAccess(ctx context.Context, menuID, roleID string, allowed bool, actor, requestID string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO menu_role_access (menu_id, role_id, allowed) VALUES ($1,$2,$3)
		ON CONFLICT (menu_id, role_id) DO UPDATE SET allowed = EXCLUDED.allowed`,
		menuID, roleID, allowed)
	if err != nil {
		return err
	}
	_ = audit.Record(ctx, s.db, audit.Entry{ActorID: actor, Action: "rbac.menu_access.set", EntityType: "menu",
		EntityID: menuID, RequestID: requestID, Metadata: map[string]any{"role_id": roleID, "allowed": allowed}})
	return nil
}

// expose pool type for callers if needed
var _ = (*pgxpool.Pool)(nil)
var _ = pgx.ErrNoRows
