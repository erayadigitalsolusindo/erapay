-- +goose Up
-- DEWA (platform super_admin++): role + permissions for dynamic RBAC & menu access.

INSERT INTO roles (code, scope) VALUES ('dewa','platform') ON CONFLICT (code) DO NOTHING;

INSERT INTO permissions (code) VALUES ('rbac:manage'), ('menu:manage') ON CONFLICT (code) DO NOTHING;

-- DEWA inherits every permission (including the two new ones).
INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r CROSS JOIN permissions p
WHERE r.code = 'dewa'
ON CONFLICT (role_id, permission_code) DO NOTHING;

-- Menu catalog mirrored from apps/web nav.ts so access is manageable at runtime.
CREATE TABLE menu_items (
    id              text PRIMARY KEY,
    href            text NOT NULL,
    label_key       text NOT NULL,
    permission_code text REFERENCES permissions(code) ON DELETE SET NULL,
    platform_only   boolean NOT NULL DEFAULT false,
    sort_order      integer NOT NULL DEFAULT 0
);

CREATE TABLE menu_role_access (
    menu_id  text NOT NULL REFERENCES menu_items(id) ON DELETE CASCADE,
    role_id  uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    allowed  boolean NOT NULL DEFAULT false,
    PRIMARY KEY (menu_id, role_id)
);

INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order) VALUES
    ('/dashboard','/dashboard','nav.overview',NULL,false,10),
    ('/dashboard/transactions','/dashboard/transactions','nav.transactions',NULL,false,20),
    ('/dashboard/api-keys','/dashboard/api-keys','nav.apiKeys','api_key:manage',false,30),
    ('/dashboard/payouts','/dashboard/payouts','nav.payouts','payout:read',false,40),
    ('/dashboard/refunds','/dashboard/refunds','nav.refunds','refund:read',false,50),
    ('/dashboard/webhooks','/dashboard/webhooks','nav.webhooks','webhook:manage',false,60),
    ('/dashboard/onboarding','/dashboard/onboarding','nav.account','merchant:write',false,70),
    ('/dashboard/finance/settlement','/dashboard/finance/settlement','nav.fSettlement','settlement:read',true,80),
    ('/dashboard/finance/payouts','/dashboard/finance/payouts','nav.fPayouts','payout:review',true,90),
    ('/dashboard/finance/refunds','/dashboard/finance/refunds','nav.fRefunds','refund:review',true,100),
    ('/dashboard/finance/merchants','/dashboard/finance/merchants','nav.fMerchants','merchant:review',true,110);

-- Default matrix: derived from role permissions + platform/merchant scope.
INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT mi.id, r.id,
       CASE
         WHEN mi.platform_only AND r.scope <> 'platform' THEN false
         WHEN NOT mi.platform_only AND r.scope <> 'merchant' THEN false
         WHEN mi.permission_code IS NULL THEN true
         ELSE EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_code = mi.permission_code)
       END
FROM menu_items mi CROSS JOIN roles r
ON CONFLICT (menu_id, role_id) DO NOTHING;

-- +goose Down
DROP TABLE menu_role_access;
DROP TABLE menu_items;
DELETE FROM role_permissions WHERE permission_code IN ('rbac:manage','menu:manage');
DELETE FROM permissions WHERE code IN ('rbac:manage','menu:manage');
DELETE FROM user_roles WHERE role_id IN (SELECT id FROM roles WHERE code='dewa');
DELETE FROM merchant_users WHERE role_id IN (SELECT id FROM roles WHERE code='dewa');
DELETE FROM roles WHERE code = 'dewa';