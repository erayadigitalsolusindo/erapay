-- +goose Up
-- Menu Finance adalah ruang kerja staf EraPay. Tenant (client EraPay) punya menu sendiri:
-- Saldo & Payout, Refund, dst. Cabut menu Finance dari semua role merchant lewat matriks akses
-- (data, bukan hardcode); DEWA masih bisa membukanya lagi di matriks bila perlu.
UPDATE menu_role_access mra
SET allowed = false
FROM roles r
WHERE r.id = mra.role_id
  AND r.scope = 'merchant'
  AND mra.menu_id LIKE '/dashboard/finance/%'
  AND mra.allowed;

-- +goose Down
-- Kembali ke kondisi 0026/0027: settlement, review payout & review refund terbuka untuk merchant_owner.
UPDATE menu_role_access mra
SET allowed = true
FROM roles r
WHERE r.id = mra.role_id
  AND r.code = 'merchant_owner'
  AND mra.menu_id IN ('/dashboard/finance/settlement', '/dashboard/finance/payouts', '/dashboard/finance/refunds');
