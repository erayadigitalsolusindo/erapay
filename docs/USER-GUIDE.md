# Panduan Lengkap Pengguna EraPay

Panduan ini untuk **siapa saja yang mulai memakai EraPay**: pemilik merchant, kasir, integrator POS, staf platform, dan DEWA/administrator hak akses.

Dokumen ini menjawab:
- cara daftar dan masuk,
- cara mulai terima pembayaran QRIS,
- cara menggunakan dashboard,
- cara integrasi API,
- cara mengelola saldo, payout, refund, webhook,
- cara mengatur hak akses dan menu oleh DEWA,
- cara mencari bantuan bila bermasalah.

---

## 1. Apa itu EraPay

EraPay adalah platform payment aggregator yang menyediakan:
- satu API untuk aplikasi POS/merchant,
- dashboard merchant dan dashboard platform,
- pembayaran QRIS melalui gateway DOKU,
- ledger dan rekonsiliasi untuk setiap rupiah,
- payout ke rekening merchant,
- refund,
- webhook ke server merchant,
- RBAC (hak akses berbasis peran).

Di fase saat ini, gateway aktif adalah **DOKU**, dan metode awal adalah **QRIS**. Dana merchant dulu diterima oleh EraPay/PT, lalu dibayarkan ke merchant melalui payout yang disetujui Finance. EraPay bukan wallet bebas, bukan top-up, dan bukan transfer antar pengguna.

---

## 2. Peran pengguna

| Peran | Lingkup | Boleh |
|---|---|---|
| Merchant Owner | merchant | kelola profil, outlet, user, API key, webhook, rekening, payout, refund, transaksi |
| Cashier | merchant | buat/lihat transaksi sesuai outlet |
| Integrator | merchant | memakai API key terbatas dan webhook |
| Super Admin | platform | konfigurasi global, approval merchant, metrik, user/role |
| Ops Support | platform | cari merchant/transaksi, lihat timeline, buat catatan review |
| Finance | platform | settlement, review payout/refund, approve payout/refund |
| DEWA | platform | Super Admin++: kelola role, permission, akses menu, dan penugasan user |

Aturan penting:
- Pengguna merchant hanya bisa melihat data merchantnya sendiri.
- Staf platform bekerja lintas merchant, tetapi tetap dibatasi permission.
- DEWA mengatur hak akses; DEWA bukan pengganti approval finansial otomatis.

---

## 3. Mulai untuk merchant

### 3.1 Daftar akun

1. Buka halaman pendaftaran EraPay.
2. Pilih jenis akun: `individual` atau `business`.
3. Isi nama, nama usaha, email, nomor HP, dan password.
4. Setujui syarat & ketentuan.
5. EraPay mengirim tautan verifikasi email.

Pendaftaran **tidak langsung aktif**. Merchant harus:
1. verifikasi email,
2. menunggu review platform,
3. disetujui Super Admin,
4. hanya setelah itu pembayaran/payout bisa aktif.

### 3.2 Login

1. Buka halaman login.
2. Masukkan email dan password.
3. Jika akun merchant punya beberapa merchant, masukkan `merchant_id`.
4. Untuk akun platform yang mengaktifkan MFA, lanjutkan dengan kode TOTP/recovery code.

Jika lupa password:
- pakai fitur lupa password,
- buka tautan reset yang dikirim ke email,
- buat password baru.

### 3.3 Lengkapi data merchant

Di dashboard, Merchant Owner:
- memperbarui profil merchant,
- menambah/melihat outlet,
- menambahkan user dan peran kasir/integrator,
- memantau status verifikasi email,
- memantau status approval merchant.

Status merchant:
- `draft`: belum dikirim.
- `submitted`: menunggu review.
- `under_review`: sedang ditinjau.
- `action_required`: perlu perbaikan.
- `approved`: aktif.
- `rejected`: ditolak.
- `suspended`: ditangguhkan.

---

## 4. Menu dashboard

### 4.1 Merchant

Menu umum merchant:
- **Ringkasan**: performa transaksi.
- **Transaksi**: daftar pembayaran dan detail.
- **Buat QRIS**: membuat QRIS baru.
- **API Keys**: membuat, mencabut, dan merotasi key integrasi.
- **Saldo & Payout**: saldo, rekening tujuan, dan payout.
- **Refund**: mengajukan dan memantau refund.
- **Webhook**: endpoint notifikasi dan riwayat pengiriman.
- **Status akun**: verifikasi email dan status approval.

### 4.2 Platform

Menu platform:
- **Settlement**: melihat pembayaran menunggu settlement dan mencatat batch settlement.
- **Review payout**: verifikasi rekening bank, approve payout, tandai transfer.
- **Review refund**: approve/reject/eksekusi refund.
- **Review merchant**: approve/reject merchant.
- **Hak akses**: khusus DEWA.

### 4.3 Sidebar dinamis

Sidebar tidak lagi hanya hardcoded. DEWA bisa mengatur menu mana yang tampil untuk role mana. Jika sebuah menu hilang padahal biasanya masuk, kemungkinan akses menu role kamu diubah oleh DEWA.

---

## 5. Cara menerima pembayaran QRIS

### 5.1 Dari dashboard

Cara paling cepat untuk mencoba:

1. Login sebagai Merchant Owner/Cashier.
2. Buka **Transaksi → Buat QRIS**.
3. Isi referensi, nominal dalam rupiah penuh, dan deskripsi.
4. Simpan.
5. Tampilkan QR code kepada pelanggan.
6. Pantau status di halaman transaksi.

### 5.2 Dari API/Integrasi POS

Untuk aplikasi POS/merchant, gunakan API key.

Langkah ringkas:
1. Buat API key di dashboard.
2. Simpan secret key karena hanya tampil sekali.
3. Panggil `POST /v1/payments`.
4. Tampilkan `qr_code.value`.
5. Tunggu status final dari webhook atau inquiry API.

Contoh:

```bash
curl -X POST https://<host>/v1/payments \
  -H "Authorization: Bearer epk_sbx_..." \
  -H "Idempotency-Key: ord-1001-try1" \
  -H "Content-Type: application/json" \
  -d '{
    "merchant_reference": "ORD-1001",
    "amount": 100000,
    "currency": "IDR",
    "payment_method": "QRIS",
    "description": "Kopi"
  }'
```

Respons berisi QR code, status awal, dan fee. Pelanggan membayar nominal transaksi tanpa biaya tambahan; fee adalah potongan terhadap merchant.

Aturan penting:
- `Idempotency-Key` wajib untuk operasi yang mengubah uang.
- Jika timeout/jaringan putus, ulangi dengan key yang sama.
- Jangan membuat payment baru ketika status `unknown`.
- Jangan menandai pesanan lunas dari layar kasir; pakai status API/webhook.

Dokumentasi API lengkap: `docs/API-INTEGRATOR.md`.

---

## 6. Status pembayaran

Ada tiga status terpisah yang harus dibaca berbeda:

| Field | Nilai | Arti |
|---|---|---|
| `status` | `created`, `pending`, `succeeded`, `failed`, `expired`, `cancelled`, `unknown` | status pembayaran |
| `settlement_status` | `not_expected`, `expected`, `settled` | status dana dari gateway |
| `refunded_amount` / `refunding_amount` | integer | status refund, tidak mengubah `status` pembayaran |

Arti `settlement_status`:
- `not_expected`: pembayaran belum/sedang proses.
- `expected`: pembayaran berhasil, menunggu dana settle dari gateway.
- `settled`: dana sudah diterima EraPay.

Arti status `unknown`:
- gateway tidak memberi jawaban pasti.
- **bukan gagal**.
- tunggu webhook atau inquiry, jangan retry create payment.

---

## 7. Saldo merchant

Saldo merchant selalu berasal dari ledger, bukan kolom yang bisa diedit.

Field `GET /v1/balance`:
- `payable`: kewajiban EraPay ke merchant.
- `pending`: dana belum settle dari gateway.
- `available`: dana boleh di-payout.
- `in_payout`: dana sedang diproses payout.
- `on_hold`: dana tertahan untuk refund.

Rumus penting:
- `available` = saldo yang sudah settle dan bisa dicairkan.
- `merchant_net` = gross - MDR - biaya jasa EraPay.

---

## 8. Payout

Payout adalah pencairan dana ke rekening bank merchant.

Alur:
1. Merchant menambahkan rekening bank tujuan.
2. Finance memverifikasi rekening.
3. Rekening menunggu masa tunggu.
4. Merchant mengajukan payout.
5. Finance/EraPay approve.
6. Finance menandai transfer ke bank.
7. Webhook opsional `payout.paid` masuk ke server merchant.

Status payout:
- `pending_approval`
- `approved`
- `paid`
- `failed`
- `cancelled`

Aturan:
- Dana payout hanya dari saldo `available`.
- Maker ≠ checker.
- Payout bersifat idempotent.
- Timeout tidak boleh di-retry buta karena bisa menyebabkan payout ganda.
- Payout di luar produksi adalah dry-run: pembukuan dan alur berjalan, tetapi tidak ada transfer nyata.

Di dashboard merchant:
- buka **Saldo & Payout**,
- tambah rekening,
- ajukan payout.

---

## 9. Refund

Refund pengembalian dana ke pelanggan.

Alur:
1. Merchant mengajukan refund dari detail transaksi.
2. Finance approve/reject.
3. Finance menandai refund sudah dieksekusi.
4. Webhook `refund.*` dikirim ke merchant.

Status refund:
- `pending_approval`
- `approved`
- `refunded`
- `failed`
- `cancelled`

Catatan:
- Refund tidak mengubah status pembayaran.
- Refund bisa sebagian.
- Dana refund yang diproses membuat saldo tertahan.

---

## 10. Webhook

Webhook adalah cara paling aman untuk mengetahui status akhir pembayaran/refund/payout.

### 10.1 Daftar endpoint

Di dashboard merchant:
1. Buka **Webhook**.
2. Tambah endpoint HTTPS publik.
3. Simpan secret `whsec_...` karena hanya tampil sekali.
4. Alamat IP privat, loopback, redirect, dan kredensial di URL ditolak.

### 10.2 Event

Event yang dikirim:
- `payment.succeeded`
- `payment.failed`
- `payment.expired`
- `payment.cancelled`
- `refund.refunded`
- `refund.failed`
- `refund.cancelled`
- `payout.paid`
- `payout.failed`
- `payout.cancelled`

### 10.3 Verifikasi signature

Header:
- `X-EraPay-Signature`
- `X-EraPay-Event-Id`
- `X-EraPay-Delivery-Attempt`

Format signature:
- `t=<unix>,v1=<hex>`
- `v1 = HMAC-SHA256(secret, "<t>." + raw_body)`

Server Anda wajib:
1. mengambil raw body sebelum parsing,
2. hitung ulang HMAC,
3. bandingkan constant-time,
4. tolak jika timestamp lebih dari 5 menit dari waktu server Anda,
5. balas HTTP 2xx secepatnya.

### 10.4 Aturan pengiriman

- At-least-once: event bisa datang lebih dari sekali.
- Urutan tidak dijamin.
- Gunakan `id` event untuk deduplikasi.
- Jika endpoint gagal, pengiriman diulang hingga beberapa kali, lalu bisa dikirim ulang manual dari dashboard.

---

## 11. API key dan scope

API key dipakai aplikasi POS/server, bukan manusia.

Scope:
- `payment:create`
- `payment:read`
- `report:read`

Operasi uang keluar berikut ini **tidak bisa** dilakukan dengan API key:
- payout,
- refund,
- kelola rekening bank,
- kelola webhook.

Operasi tersebut hanya lewat sesi dashboard.

Keamanan API key:
- simpan di server,
- jangan commit ke git,
- jangan taruh di frontend,
- rotasi bila rawan bocor,
- cabut segera bila bocor.

---

## 12. MFA dan keamanan akun

Untuk akun platform/admin, MFA sangat disarankan/diwajibkan:
- login memakai password + kode TOTP.
- secret MFA disimpan terenkripsi.
- recovery code disimpan di tempat aman.

Aturan keamanan pengguna:
- jangan berbagi sesi.
- jangan copy secret key ke chat/email.
- gunakan password kuat.
- logout dari perangkat bersama.
- laporkan sesi mencurigakan ke support.

---

## 13. Hak akses dan menu oleh DEWA

Fitur DEWA memungkinkan super admin++ mengatur hak akses tanpa membuat migrasi baru.

### 13.1 Buka halaman

Masuk sebagai DEWA, lalu buka panel khusus platform:
`/platform/login`

Setelah login, buka **Hak akses** di `/platform/access`. Panel platform (auth/audit
terpisah dari dashboard, 2FA wajib) hanya bisa diakses oleh role `dewa`.

### 13.2 Tab Role

Bisa:
- membuat role baru,
- mengubah kode/scope role,
- menghapus role.

Scope:
- `platform`: staf EraPay.
- `merchant`: pengguna merchant.

### 13.3 Tab Matriks Izin

Bisa mengatur permission untuk tiap role.

Permission bawaan mencakup:
- merchant: `merchant:read`, `merchant:write`, `merchant:review`, `merchant:approve`
- outlet: `outlet:read`, `outlet:write`
- payment: `payment:read`, `payment:create`
- key/user: `api_key:manage`, `user:manage`
- report/audit: `report:read`, `audit:read`
- gateway: `gateway:manage`
- payout: `payout:read`, `payout:request`, `payout:review`, `payout:approve`, `payout:execute`
- settlement: `settlement:read`, `settlement:manage`
- webhook: `webhook:manage`
- refund: `refund:request`, `refund:read`, `refund:review`, `refund:approve`, `refund:execute`
- bank: `bank_account:manage`, `bank_account:verify`
- DEWA: `rbac:manage`, `menu:manage`

### 13.4 Tab Akses Menu

Bisa menentukan menu mana muncul untuk role mana.

Contoh:
- role `cashier` tidak melihat menu payout,
- role `finance` melihat menu review payout/refund,
- role baru custom hanya melihat transaksi.

Perubahan ini langsung mempengaruhi sidebar pada request berikutnya.

### 13.5 Tab User

Bisa assign user ke role.

Untuk user merchant:
- masukkan `merchant_id`,
- pilih role merchant.

Untuk user platform:
- biarkan `merchant_id` kosong,
- pilih role platform.

### 13.6 Refresh dan efek

Permission dibaca fresh setiap request. Namun user yang sedang login sebaiknya logout-login kembali agar tampilan sidebar dan hak akses terbaru selalu sinkron.

---

## 14. Sandbox vs production

| Hal | Sandbox | Production |
|---|---|---|
| API key prefix | `epk_sbx_...` | `epk_live_...` |
| DOKU | sandbox | production |
| Payout | dry-run | nyata setelah go/no-go |
| Pembayaran QRIS | transaksi uji | transaksi nyata |

Gunakan sandbox untuk integrasi, uji webhook, uji status, dan uji pembukuan. Pindah ke production hanya setelah semua tes idempotency dan webhook lulus.

---

## 15. Troubleshooting umum

| Masalah | Kemungkinan | Solusi |
|---|---|---|
| Login gagal | email/password salah, akun disabled | coba reset password, hubungi admin |
| MFA ditolak | kode kedaluwarsa/device salah | sinkronkan jam, pakai recovery code |
| Pembayaran `unknown` | gateway tidak pasti | tunggu webhook/inquiry, jangan buat baru |
| Payment `pending` tidak berubah | webhook belum masuk | cek dashboard Webhook, cek signature |
| Webhook gagal | endpoint bukan HTTPS publik, signature salah | perbaiki endpoint dan verifikasi |
| Payout gagal | saldo `available` kurang, rekening belum verified | tunggu settle, verifikasi rekening |
| Refund gagal | melebihi nominal/status | cek detail transaksi |
| Sidebar menu hilang | akses menu diubah DEWA | minta DEWA cek Akses Menu |
| Muncul 403 | permission tidak sesuai | cek role/permission user |

Diagnosa lebih lanjut:
- `docs/TROUBLESHOOTING.md`
- `docs/API-INTEGRATOR.md`
- `docs/DEVELOPMENT.md`

---

## 16. Bantuan & kontak

Saat menghubungi support/admin, sertakan:
- `request_id` dari respons/error,
- `payment_id` bila ada,
- waktu kejadian,
- endpoint yang dipanggil,
- bukan secret, bukan API key, bukan password.

Contoh format laporan:

```text
request_id: req_...
payment_id: ...
waktu: 2026-10-13 14:00 WIB
endpoint: POST /v1/payments
gejala: response 202 status unknown
```

---

## 17. Istilah penting

| Istilah | Arti |
|---|---|
| API key | kunci server untuk POS |
| Scope | batas permission API key |
| Idempotency | perintah sama tidak dieksekusi dua kali |
| QRIS | metode pembayaran QR standar Indonesia |
| Ledger | pembukuan double-entry append-only |
| MDR | diskon kartu |
| Biaya Jasa EraPay | fee platform terhadap merchant |
| Merchant net | dana diterima merchant setelah fee |
| Settlement | dana diterima dari gateway |
| Payout | pencairan dana ke rekening merchant |
| Refund | pengembalian dana ke pelanggan |
| Webhook | notifikasi push ke server merchant |
| MFA | verifikasi dua faktor |
| DEWA | super admin++ pengelola RBAC/menu |
| Unknown | status ambigu, perlu inquiry/rekonsiliasi |

---

## 18. Ringkasan checklist pengguna baru

### Merchant Owner

1. Daftar dan verifikasi email.
2. Tunggu approval.
3. Login dashboard.
4. Tambah outlet/user.
5. Buat API key.
6. Daftarkan webhook.
7. Tambah rekening bank.
8. Uji pembayaran di sandbox.
9. Mulai produksi setelah review.

### Integrator POS

1. Minta API key.
2. Baca `docs/API-INTEGRATOR.md`.
3. Implement create payment.
4. Implement verifikasi webhook.
5. Implement idempotency.
6. Uji ulang pengiriman webhook.
7. Go live.

### Platform Staff

1. Login akun platform.
2. Aktifkan MFA.
3. Review merchant/transaksi sesuai permission.
4. Jangan menyetujui payout yang kamu buat sendiri.
5. Catat keputusan untuk audit.

### DEWA

1. Gunakan role `dewa`.
2. Login di `/platform/login`, kelola hak akses di `/platform/access`.
3. Buat role sesuai kebutuhan.
4. Atur permission dan menu.
5. Assign user.
6. Pantau audit log.