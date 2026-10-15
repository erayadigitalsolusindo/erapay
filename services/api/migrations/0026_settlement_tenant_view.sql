-- +goose Up
-- Menu platform yang punya tampilan per-tenant (data hanya milik merchant sendiri) boleh diberikan
-- ke role merchant lewat matriks akses menu. Settlement: merchant melihat pembayaran miliknya yang
-- belum di-settle; batch & exception rekonsiliasi (lintas tenant) tetap hanya untuk staf platform.
ALTER TABLE menu_items ADD COLUMN merchant_view boolean NOT NULL DEFAULT false;
UPDATE menu_items SET merchant_view = true WHERE id = '/dashboard/finance/settlement';

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, 'settlement:read' FROM roles r WHERE r.code = 'merchant_owner'
ON CONFLICT DO NOTHING;

INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT '/dashboard/finance/settlement', r.id, true FROM roles r WHERE r.code = 'merchant_owner'
ON CONFLICT (menu_id, role_id) DO UPDATE SET allowed = true;

-- +goose Down
DELETE FROM menu_role_access WHERE menu_id = '/dashboard/finance/settlement'
  AND role_id IN (SELECT id FROM roles WHERE scope = 'merchant');
DELETE FROM role_permissions WHERE permission_code = 'settlement:read'
  AND role_id IN (SELECT id FROM roles WHERE code = 'merchant_owner');
ALTER TABLE menu_items DROP COLUMN merchant_view;
