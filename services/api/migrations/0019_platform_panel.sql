-- +goose Up
-- DEWA pindah ke panel platform khusus (/platform). Menu "Hak akses" tidak lagi
-- bagian dari sidebar dashboard: DEWA login lewat panel (cookie erapay_platform_session),
-- dan /platform/(panel)/access menjadi satu-satunya tempat mengelola RBAC.
-- super_admin/finance/ops tetap di dashboard dan tidak melihat menu ini (tanpa rbac:manage).

DELETE FROM menu_role_access WHERE menu_id = '/dashboard/admin/access';
DELETE FROM menu_items WHERE id = '/dashboard/admin/access';

-- +goose Down
INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order)
VALUES ('/dashboard/admin/access','/dashboard/admin/access','nav.access','rbac:manage',true,120)
ON CONFLICT (id) DO UPDATE SET href = EXCLUDED.href, label_key = EXCLUDED.label_key,
    permission_code = EXCLUDED.permission_code, platform_only = EXCLUDED.platform_only, sort_order = EXCLUDED.sort_order;

INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT mi.id, r.id,
       CASE
         WHEN mi.platform_only AND r.scope <> 'platform' THEN false
         WHEN NOT mi.platform_only AND r.scope <> 'merchant' THEN false
         WHEN mi.permission_code IS NULL THEN true
         ELSE EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_code = mi.permission_code)
       END
FROM menu_items mi CROSS JOIN roles r
WHERE mi.id = '/dashboard/admin/access'
ON CONFLICT (menu_id, role_id) DO NOTHING;
