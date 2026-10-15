-- +goose Up
-- API Keys + Webhook digabung jadi satu menu "Integrasi". Izin per bagian tetap dijaga halaman & API
-- (api_key:manage / webhook:manage); menu muncul bagi role merchant yang punya salah satunya.
INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order)
SELECT '/dashboard/integrasi', '/dashboard/integrasi', 'nav.integrations', NULL, false,
       COALESCE((SELECT min(sort_order) FROM menu_items WHERE id IN ('/dashboard/api-keys','/dashboard/webhooks')), 40)
ON CONFLICT (id) DO NOTHING;

INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT '/dashboard/integrasi', r.id,
       r.scope = 'merchant' AND EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_code IN ('api_key:manage','webhook:manage'))
FROM roles r
ON CONFLICT (menu_id, role_id) DO NOTHING;

DELETE FROM menu_items WHERE id IN ('/dashboard/api-keys','/dashboard/webhooks');

-- +goose Down
INSERT INTO menu_items (id, href, label_key, permission_code, platform_only, sort_order) VALUES
 ('/dashboard/api-keys','/dashboard/api-keys','nav.apiKeys','api_key:manage',false,20),
 ('/dashboard/webhooks','/dashboard/webhooks','nav.webhooks','webhook:manage',false,40)
ON CONFLICT (id) DO NOTHING;
INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT mi.id, r.id, r.scope = 'merchant' AND EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_code = mi.permission_code)
FROM menu_items mi CROSS JOIN roles r WHERE mi.id IN ('/dashboard/api-keys','/dashboard/webhooks')
ON CONFLICT (menu_id, role_id) DO NOTHING;
DELETE FROM menu_items WHERE id = '/dashboard/integrasi';
