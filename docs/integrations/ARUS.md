# Integrasi Ekosistem ARUS

Endpoint provisioning ini hanya untuk backend-ke-backend. Jangan gunakan dari browser dan jangan memakai API key pembayaran merchant.

Konfigurasikan `ERAPAY_ARUS_PROVISION_TOKEN` pada API ERAPAY dan backend ARUS dengan nilai acak yang sama (minimal 32 karakter). API ERAPAY hanya memasang route integrasi bila token tersedia. Jalankan migration `0018_arus_tenant_links.sql` sebelum mengaktifkannya.

## Cek Status

`GET /v1/integrations/arus/tenants/{tenant_id}` dengan header `Authorization: Bearer <token>`. Respons `200` berisi `linked`, `tenant_id`, `merchant_id`, `merchant_status`, dan `owner_role`. `404 NOT_FOUND` berarti tenant ARUS belum tertaut.

## Hubungkan Tenant

`POST /v1/integrations/arus/tenants` menerima `tenant_id`, `business_name`, `owner_name`, `owner_email`, `owner_phone`, dan `accepted_terms: true`. Identitas pemilik harus dibaca dari sesi tenant oleh backend ARUS, bukan diterima langsung dari browser.

Operasi memakai kunci transaksi per tenant dan unique constraint `(source, source_tenant_id)`, sehingga retry mengembalikan merchant yang sama. Sebelum membuat merchant baru, ERAPAY juga menautkan satu merchant lama bila email owner dan nama usaha cocok persis. Jika ada beberapa kandidat, endpoint mengembalikan `409 MERCHANT_LINK_AMBIGUOUS` dan tidak membuat duplikat. Merchant baru dibuat sebagai `submitted` dengan outlet utama dan membership `merchant_owner`. Jika alamat email belum punya akun EraPay, pemilik mendapat tautan satu kali untuk membuat sandi. Merchant tetap harus memverifikasi email dan menunggu review/approval EraPay sebelum pembayaran atau payout dapat dilakukan.

Persetujuan syarat dicatat pada saat link dibuat. API key QRIS, pembayaran, dan status koneksi tenant adalah data yang terpisah.

## Terbitkan API Key Pembayaran

`POST /v1/integrations/arus/tenants/{tenant_id}/api-key` memakai **token terpisah**, `ERAPAY_ARUS_KEY_TOKEN` (minimal 32 karakter, harus berbeda dari token provisioning). Rute hanya dipasang bila token ini diisi, dan token provisioning tidak diterima di rute ini. Dipanggil backend ARUS saat Platform Admin (setelah verifikasi ulang password + TOTP di ARUS) menekan "Buat & pasang key".

- Tenant harus tertaut (`404 NOT_FOUND` bila belum) dan merchant berstatus `approved` (`409 MERCHANT_NOT_APPROVED`).
- Key hanya berisi izin `payment:create` dan `payment:read`; tidak ada payout, laporan, atau manajemen.
- Satu key aktif bernama "ARUS (otomatis)" per merchant; permintaan berikutnya dijawab `409 API_KEY_EXISTS`. Rotasi/pencabutan lewat dashboard ERAPAY.
- Respons `201` memuat `secret` satu kali (`Cache-Control: no-store`). Penerbitan tercatat di audit (`api_key.created`).

