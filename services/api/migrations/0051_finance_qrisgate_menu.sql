-- +goose Up
-- Menu Finance "Top-up QRISGate" (platform saja): akses mengikuti izin settlement:read, sama seperti menu settlement.
INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order)
SELECT '/dashboard/finance/qrisgate', '/dashboard/finance/qrisgate', 'nav.fQrisgate', 'settlement:read', true,
       COALESCE((SELECT sort_order FROM menu_items WHERE id = '/dashboard/finance/settlement'), 80) + 1
ON CONFLICT (id) DO NOTHING;

INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT '/dashboard/finance/qrisgate', r.id,
       r.scope = 'platform' AND EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_code = 'settlement:read')
FROM roles r
ON CONFLICT (menu_id, role_id) DO NOTHING;

-- +goose Down
DELETE FROM menu_role_access WHERE menu_id = '/dashboard/finance/qrisgate';
DELETE FROM menu_items WHERE id = '/dashboard/finance/qrisgate';
