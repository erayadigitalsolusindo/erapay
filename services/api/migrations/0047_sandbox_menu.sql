-- +goose Up
-- Menu Sandbox (aktifkan dan pindah antara Sandbox/Live) untuk semua role merchant; tepat setelah Tutorial.
INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order)
SELECT '/dashboard/sandbox', '/dashboard/sandbox', 'nav.sandbox', 'merchant:read', false,
       COALESCE((SELECT sort_order FROM menu_items WHERE id = '/dashboard/tutorial'), 41) + 1
ON CONFLICT (id) DO NOTHING;

INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT '/dashboard/sandbox', r.id, r.scope = 'merchant' FROM roles r
ON CONFLICT (menu_id, role_id) DO NOTHING;

-- +goose Down
DELETE FROM menu_role_access WHERE menu_id = '/dashboard/sandbox';
DELETE FROM menu_items WHERE id = '/dashboard/sandbox';
