-- +goose Up
-- Menu panel platform (/platform/*) ikut diatur di RBAC (menu_items + menu_role_access) seperti menu lain.
INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order) VALUES
    ('/platform','/platform','pf.nav.overview','rbac:manage',true,1),
    ('/platform/tenants','/platform/tenants','pf.nav.tenants','rbac:manage',true,2),
    ('/platform/transactions','/platform/transactions','pf.nav.trx','rbac:manage',true,3),
    ('/platform/gateways','/platform/gateways','pf.nav.gateways','rbac:manage',true,130),
    ('/platform/audit','/platform/audit','pf.nav.audit','rbac:manage',true,131),
    ('/platform/access','/platform/access','pf.nav.access','rbac:manage',true,132)
ON CONFLICT (id) DO NOTHING;

INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT mi.id, r.id, r.scope = 'platform' AND EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_code = mi.permission_code)
FROM menu_items mi CROSS JOIN roles r WHERE mi.id LIKE '/platform%'
ON CONFLICT (menu_id, role_id) DO NOTHING;

-- +goose Down
DELETE FROM menu_items WHERE id LIKE '/platform%';
