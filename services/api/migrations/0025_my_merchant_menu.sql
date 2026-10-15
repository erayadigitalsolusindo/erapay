-- +goose Up
-- Menu "Merchant saya" (scope merchant). Bukan platform_only; akses awal mengikuti izin merchant:read.
INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order)
VALUES ('/dashboard/merchant','/dashboard/merchant','nav.myMerchant','merchant:read',false,45)
ON CONFLICT (id) DO UPDATE SET href = EXCLUDED.href, label_key = EXCLUDED.label_key,
    permission_code = EXCLUDED.permission_code, platform_only = EXCLUDED.platform_only, sort_order = EXCLUDED.sort_order;

INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT mi.id, r.id,
       CASE
         WHEN r.scope <> 'merchant' THEN false
         ELSE EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_code = mi.permission_code)
       END
FROM menu_items mi CROSS JOIN roles r
WHERE mi.id = '/dashboard/merchant'
ON CONFLICT (menu_id, role_id) DO NOTHING;

-- +goose Down
DELETE FROM menu_role_access WHERE menu_id = '/dashboard/merchant';
DELETE FROM menu_items WHERE id = '/dashboard/merchant';
