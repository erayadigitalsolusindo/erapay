-- +goose Up
-- Menu Tutorial (panduan integrasi) untuk semua role merchant; ditaruh tepat setelah Integrasi.
INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order)
SELECT '/dashboard/tutorial', '/dashboard/tutorial', 'nav.tutorial', NULL, false,
       COALESCE((SELECT sort_order FROM menu_items WHERE id = '/dashboard/integrasi'), 40) + 1
ON CONFLICT (id) DO NOTHING;

INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT '/dashboard/tutorial', r.id, r.scope = 'merchant' FROM roles r
ON CONFLICT (menu_id, role_id) DO NOTHING;

-- +goose Down
DELETE FROM menu_role_access WHERE menu_id = '/dashboard/tutorial';
DELETE FROM menu_items WHERE id = '/dashboard/tutorial';
