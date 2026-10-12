# Troubleshooting EraPay

Catatan masalah nyata yang pernah terjadi beserta cara mendiagnosisnya. Tambahkan kasus baru di bawah dengan format yang sama: gejala, cara mendiagnosis, penyebab, perbaikan.

Aturan umum saat diagnosa: jangan menempel isi `erapay.env`, `ERAPAY_DATABASE_URL`, atau secret apa pun ke chat/ticket/log. Kalau sudah terlanjur, ganti secret-nya.

## Peta lokasi di server (aaPanel)

| Hal | Lokasi |
|---|---|
| Env API | `/www/wwwroot/erapay-api.erayadigital.co.id/erapay.env` |
| Env web | `/www/wwwroot/erapay.erayadigital.co.id/erapay-web.env` |
| Log API / web | `journalctl -u erapay-api -f` / `journalctl -u erapay-web -f` |
| Binary diagnostik | `/www/wwwroot/erapay-api.erayadigital.co.id/bin/` (`erapay-webhookverify`, dll.) |
| PostgreSQL | Container Docker, bukan service host. Tidak ada user OS `postgres`; `sudo -u postgres psql` gagal. |

Akses database lewat container (cari namanya dengan `docker ps --format '{{.Names}}\t{{.Ports}}' | grep -E "5433|postgres"`):

```bash
docker exec -i <nama-container> psql -U erapay -d erapay -c "select 1;"
```

Password diminta di prompt; ketik di sana, jangan ditempel ke tempat lain.

---

## Kasus 1: QRIS berhasil di lokal, di server status "Belum pasti (sedang dicek)"

### Gejala
Halaman pembayaran menampilkan badge **Belum pasti (sedang dicek)** dan QR tidak pernah muncul / tidak pernah selesai. Di lokal alurnya normal.

### Arti status
Badge itu adalah status `unknown` (lihat `apps/web/src/lib/components/StatusBadge.svelte`). `unknown` bukan gagal: hasil di gateway belum diketahui (PRD §0 #3). Status `pending` yang tidak kunjung berubah juga terlihat serupa di UI, jadi bedakan lewat data di bawah.

### Cara mendiagnosis
Lihat percobaan ke gateway untuk payment terbaru:

```sql
select p.id, p.status, p.gateway_reference, p.next_inquiry_at,
       a.operation, a.outcome, a.http_status, a.gateway_code, a.raw_status, a.created_at
from payments p
left join payment_attempts a on a.payment_id = p.id
where p.created_at > now() - interval '2 hours'
order by p.created_at desc, a.created_at desc
limit 15;
```

Nama kolom: `gateway_code` (bukan `response_code`); `payment_id` bertipe uuid, bukan angka yang tampil di judul halaman.

Cara membaca hasil:

| `operation` / `outcome` | `gateway_reference` | Arti |
|---|---|---|
| `create` / `ambiguous`, `http_status` 500, `gateway_code` `5004701` | kosong | **Pembuatan QR ditolak DOKU.** QR tidak pernah terbentuk. Lihat penyebab di bawah. |
| `inquiry` / `unmapped` | terisi | QR terbentuk, tapi kode status inquiry belum dipetakan (flag `DOKU_QRIS_STATUS_MAP_CONFIRMED=false`). |
| tidak ada baris `create` sama sekali | kosong | Request tidak sampai ke adapter (gateway tidak terdaftar, atau env DOKU belum diisi). Cek log startup: `gateway registered`. |
| `create` / `ok`, status `pending` | terisi | Normal; tunggu webhook atau inquiry. |

Karena `create` yang 5xx diklasifikasikan *ambiguous* (bukan *failed*) oleh `classify()` di `services/api/internal/gateway/doku/doku.go`, payment berstatus `unknown` walau sebenarnya permintaan ditolak. Itu disengaja: 5xx bisa berarti DOKU sempat memproses.

### Penyebab yang ditemukan
`merchant_gateway_accounts.gateway_ref` di database server berisi **Client ID DOKU** (`BRN-...`), bukan merchant ID. Nilai ini dikirim sebagai `merchantId` pada `qr-mpm-generate` (`CreatePayment` di `doku.go`), dan DOKU menjawab HTTP 500 / `5004701` alih-alih 4xx yang jelas. Di lokal nilainya benar (`111214`, akhiran dari merchant PAN `936008990000111214` pada payload webhook), karena itu lokal berhasil.

Petunjuk bahwa token dan kredensial sudah benar: kegagalan token tercatat sebagai `rejected`, bukan `ambiguous` 500. Jadi `DOKU_CLIENT_ID`, `DOKU_SECRET_KEY` dan private key kemungkinan benar, dan yang salah ada di body request (`merchantId`, `terminalId`, `postalCode`).

### Perbaikan
Cek dan samakan nilai antara lokal dan server, **per environment**:

```sql
select merchant_id, gateway, environment, gateway_ref from merchant_gateway_accounts;
```

```sql
update merchant_gateway_accounts
set gateway_ref = '<merchant-id-yang-benar>'
where gateway = 'doku' and environment = '<sandbox|production>';
```

- Yang dipakai adalah baris dengan `environment` sama dengan `ERAPAY_ENV` di `erapay.env`. Server production memakai baris `production`; memperbaiki baris `sandbox` tidak berefek.
- Tidak perlu restart service: nilainya dibaca dari DB setiap payment dibuat.
- Payment lama yang `unknown` tanpa `gateway_reference` **tidak bisa dipulihkan** karena QR-nya tidak pernah ada di DOKU. Buat payment baru.
- Pastikan juga `DOKU_TERMINAL_ID` dan `DOKU_POSTAL_CODE` di `erapay.env` sama dengan yang terbukti jalan, dan `DOKU_BASE_URL` sesuai environment (`https://api.doku.com` untuk production).
- Jangan jalankan `UPDATE` di environment production tanpa memastikan merchant ID-nya ke dashboard DOKU.

### Pelajaran
- 500 dari DOKU pada `qr-mpm-generate` sering berarti data di body tidak valid untuk merchant, bukan masalah DOKU. Periksa `merchantId`, `terminalId`, `postalCode` lebih dulu.
- Pesan error DOKU untuk `create` tidak disimpan di DB; hanya status HTTP dan `gateway_code`. Jika diagnosa berikutnya butuh pesan aslinya, tambahkan penyimpanan pesan (tanpa data sensitif) pada `payment_attempts`.
- Data gateway per merchant (`gateway_ref`) tidak ikut `.env`. Saat memindahkan/mengisi database server, periksa tabel ini secara terpisah.

---

## Kasus 2: QR terbentuk tetapi status tidak berubah setelah dibayar

Ada dua jalur yang menyelesaikan payment. Keduanya harus dicek.

1. **Webhook** (`POST /webhooks/doku`). Status `SUCCESS` selalu dipetakan, tetapi hanya diterapkan jika signature cocok. Signature = `HMACSHA256=` + base64(HMAC-SHA256(`DOKU_SECRET_KEY`, `Client-Id`, `Request-Id`, `Request-Timestamp`, `Request-Target`, `Digest` dipisah `\n`)).
   - Log `webhook signature rejected` berarti `DOKU_SECRET_KEY` / `DOKU_CLIENT_ID` di server tidak cocok dengan akun yang mengirim notifikasi.
   - Log `webhook applied` berarti sudah diterapkan.
   - Untuk menguji rumus terhadap notifikasi asli, jalankan `erapay-webhookverify <id-capture>` (DEPLOY-AAPANEL.md §10). `MATCH` = notifikasi berikutnya diterapkan.
2. **Inquiry (poller)**. Berjalan tiap 15 detik dan mengecek payment `pending`/`unknown` yang punya `gateway_reference`, dengan jeda 60 detik per payment. Selama `DOKU_QRIS_STATUS_MAP_CONFIRMED=false`, semua status inquiry "unmapped" dan payment tidak berubah (log: `inquiry status has no confirmed mapping`). Kode teramati di sandbox: `03` menunggu bayar, `00` sudah dibayar. Aktifkan flag hanya setelah owner mengonfirmasi kode untuk environment yang dipakai (belum tentu sama di production).

Mengubah flag butuh restart:

```bash
sudo systemctl restart erapay-api
journalctl -u erapay-api -n 30 | grep "gateway registered"   # harus menampilkan status_map_confirmed=true
```

---

## Kasus 3: Login web 403 / format env

- Login 403 di balik proxy: lihat DEPLOY-AAPANEL.md §9 (`csrf.trustedOrigins`, `ProxyPreserveHost On`).
- Nilai env terbaca dengan karakter `\r` di akhir: file env berakhiran CRLF. Perbaiki dengan `sudo sed -i 's/\r$//' <file>.env`.
- `erapay.env` memakai format systemd `KEY=value` (tanpa `export`); PEM multi-baris tidak didukung, pakai `DOKU_PRIVATE_KEY_FILE`.
