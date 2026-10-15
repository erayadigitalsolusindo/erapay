-- +goose Up
-- Payout & refund punya tampilan per-tenant baca-saja (memakai /v1/payouts dan /v1/refunds milik sendiri),
-- jadi menu finance-nya boleh diberikan ke role merchant lewat matriks akses menu. Review merchant tetap platform.
-- merchant_permission: izin yang disyaratkan untuk role merchant (tampilan tenant), menggantikan permission_code.
ALTER TABLE menu_items ADD COLUMN merchant_permission text REFERENCES permissions(code) ON DELETE SET NULL;
UPDATE menu_items SET merchant_view = true, merchant_permission = 'payout:read' WHERE id = '/dashboard/finance/payouts';
UPDATE menu_items SET merchant_view = true, merchant_permission = 'refund:read' WHERE id = '/dashboard/finance/refunds';

-- Awal: role merchant yang sudah boleh membaca payout/refund mendapat menunya (bisa dicabut DEWA di matriks).
INSERT INTO menu_role_access (menu_id, role_id, allowed)
SELECT mi.id, r.id, true
FROM menu_items mi
JOIN roles r ON r.scope = 'merchant'
WHERE (mi.id = '/dashboard/finance/payouts' AND EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_code = 'payout:read'))
   OR (mi.id = '/dashboard/finance/refunds' AND EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = r.id AND rp.permission_code = 'refund:read'))
ON CONFLICT (menu_id, role_id) DO UPDATE SET allowed = true;

-- +goose Down
DELETE FROM menu_role_access
WHERE menu_id IN ('/dashboard/finance/payouts','/dashboard/finance/refunds')
  AND role_id IN (SELECT id FROM roles WHERE scope = 'merchant');
UPDATE menu_items SET merchant_view = false WHERE id IN ('/dashboard/finance/payouts','/dashboard/finance/refunds');
ALTER TABLE menu_items DROP COLUMN merchant_permission;
