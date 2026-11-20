-- +goose Up
-- Menu QRISGate untuk semua role merchant; tepat setelah Sandbox. Halaman masih informasi (integrasi belum aktif).
INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order)
SELECT '/dashboard/qrisgate', '/dashboard/qrisgate', 'nav.qrisgate', 'merchant:read', false,
       COALESCE((SELECT sort_order FROM menu_items WHERE id = '/dashboard/sandbox'), 42) + 1
ON CONFLICT (id) DO NOTHING;

INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT '/dashboard/qrisgate', r.id, r.scope = 'merchant' FROM roles r
ON CONFLICT (menu_id, role_id) DO NOTHING;

-- +goose Down
DELETE FROM menu_role_access WHERE menu_id = '/dashboard/qrisgate';
DELETE FROM menu_items WHERE id = '/dashboard/qrisgate';
