# QRISGate [gw] (adapter `qrisgate`)

Sumber: https://qrisgate.web.id/ , `/docs`, `/integrations`, `/terms`, dibaca 20 Nov 2026. **Belum diuji dengan akun nyata**: tiap butir bertanda **[?]** adalah asumsi dari dokumentasi publik, bukan hasil uji. Tidak ada sandbox terdokumentasi, jadi adapter hanya untuk merchant live.

## Konsep (beda dari gateway lain)
QRIS statis merchant diubah menjadi QRIS dinamis dengan nominal terkunci per pesanan. **Dana masuk langsung ke rekening/e-wallet merchant**; QRISGate hanya membaca mutasi (aplikasi Android "Listener" atau notifikasi email bank) lalu mengirim webhook. Maka: **tidak ada clearing, settlement, payout, refund**. Gateway ini berkapabilitas `Direct`.

## Capability matrix
| | |
|---|---|
| Metode | QRIS |
| Inquiry | ya, lewat `transaction_id` (tidak ada pencarian lewat order id yang terdokumentasi; create yang tak mengembalikan id tetap `unknown` sampai kedaluwarsa atau ditangani manual) |
| Cancel / refund / payout / laporan settlement | tidak |
| Akun | **per merchant** (API key + secret webhook milik tenant), bukan per platform |

## API (yang dipakai adapter)
- Base `https://qrisgate.web.id`, `Authorization: Bearer qg_live_…` (kunci milik tenant).
- `POST /api/v1/transactions` `{external_order_id, amount, payment_method:"QRIS"}`; `external_order_id` = ID payment EraPay. Respons `data`: `transaction_id`, `base_amount`, `unique_code`, `total_amount`, `status`, `qr_string`. Pembeli membayar `total_amount` (= harga + kode unik Rp1–10, bisa 20); **payment EraPay menyimpan nominal dasar** (`base_amount`), QR sudah memuat nominal bayar.
- `GET /api/v1/transactions/{transaction_id}` untuk inquiry. **[?]** autentikasi GET tidak jelas di dokumentasi; adapter memakai Bearer yang sama.
- Format galat **[?]** tidak terdokumentasi: adapter hanya memakai kode HTTP.

## Mapping status
`PENDING`→pending, `PAID`→succeeded, `EXPIRED`→expired, `CANCELLED`→cancelled (webhook `payment.paid|expired|cancelled`). **[?]** ejaan status selain `PENDING` belum terkonfirmasi; status yang tidak dikenal **tidak dipetakan** (payment tidak berubah).

Klasifikasi galat: 408/409/429/5xx dan kegagalan jaringan = **ambigu** (`unknown`, tidak pernah `failed`, create tidak diulang buta); 4xx lain = ditolak; tanpa kredensial tersimpan = ditolak sebelum memanggil.

## Webhook
URL per tenant: `<API publik>/webhooks/qrisgate/<merchant_id>` (diisi tenant di dashboard QRISGate; ditampilkan di menu QRISGate). Header: `X-QRISGate-Event`, `-Timestamp` (unix detik), `-Signature` (hex HMAC-SHA256 atas `timestamp + "." + rawBody` dengan secret webhook tenant; awalan `sha256=` diterima, **[?]** bentuk yang benar dicek saat uji nyata), `-Delivery-Id` (kunci idempotensi, tetap sama saat retry), `-Attempt`.

Verifikasi: secret dicari dari merchant di path, HMAC atas body mentah, banding constant-time, tolak bila |now−timestamp| > 300 dtk. **Notifikasi hanya boleh menyentuh payment milik merchant di path itu** (`WebhookEvent.MerchantID`); payment merchant lain = "tidak ditemukan". Duplikat: `DedupKey = qrisgate:webhook:<Delivery-Id>`. Jumlah retry/backoff **[?]** tidak terdokumentasi.

## Fee, ledger, saldo
- Harga: plan platform tetap `qrisgate-direct` (MDR 0, Biaya Jasa persen 0): `merchant_net = gross`. Tanpa assignment per merchant.
- Saat payment sukses tidak ada jurnal penjualan (uang tidak lewat PT; tidak menyentuh `merchant_payable`, clearing, atau settlement). Hanya **potongan per transaksi** (setelan platform `qrisgate_fee`, bawaan 0) dari saldo kredit QRISGate tenant: `Dr qrisgate_credit / Cr platform_fee_revenue` (dedup per payment, saldo boleh minus). Lihat `../prd/pricing.md`.
- Payment langsung (`payments.direct`) dikecualikan dari daftar belum-settle, batch settlement, saldo `pending`, dan refund.
- Top-up saldo kredit tidak pernah dirutekan ke gateway langsung.

## Kredensial dan aktivasi (feature flag per merchant, bawaan OFF)
1. Tenant mengisi API key dan secret webhook di menu QRISGate. Disimpan terenkripsi (AES-GCM, `qrisgate_accounts`), write-only; hanya petunjuk 4 karakter terakhir yang tampil. Butuh `ERAPAY_ENCRYPTION_KEY`.
2. Itu membuat akun gateway `qrisgate` berstatus `pending_confirmation` (`gateway_ref` = merchant id) → **tidak menerima transaksi**.
3. DEWA mengaktifkan di panel (Tenant → Gateway → `qrisgate` → `active`). Sejak itu merchant dirutekan ke QRISGate bila itu akun aktif tertuanya (atau bila DEWA memilihnya di jalur pembayaran).
4. Web: set `ERAPAY_PUBLIC_API_URL` agar menu QRISGate menampilkan URL webhook lengkap.

## Uji
Contract test adapter (`internal/gateway/qrisgate`): sukses, pending, gagal, timeout/ambigu, tanda tangan salah/ubah isi/timestamp lewat batas/merchant lain, duplikat, status tak dikenal. Uji domain di `internal/qrisgate`. **Belum ada uji dengan layanan asli.**

## Belum terkonfirmasi / harus dicek dengan akun nyata
Awalan signature, autentikasi GET, ejaan semua status, format galat, jumlah retry webhook, batas rate/transaksi aktif, perilaku `external_order_id` duplikat (dipetakan ambigu), nominal tujuan bila pembeli salah ketik (QRISGate: konfirmasi manual di dashboard mereka; EraPay menolak sukses bila `base_amount` berbeda), dan keandalan deteksi mutasi (HP/email bank). Syarat gate `../prd/gateway-new.md`: poin 3 (dokumen ini), 4 (contract test), 6 (flag per merchant) terpenuhi; poin 1, 2, 5 tidak berlaku sama (tanpa settlement/kontrak gateway) tetapi **uji nyata wajib sebelum dipakai merchant sungguhan**.

## APK listener (aplikasi Android EraPay Listener)
Aplikasi terpisah di `apps/listener-android` (meneruskan notifikasi bank/e-wallet ke webhook, mis. ARUS). DEWA mengunggahnya di panel Gateway (kartu "APK Listener QRISGate"); versi terbaru muncul di menu QRISGate tenant lengkap dengan sha256 dan tombol Unduh. Berkas disimpan di `ERAPAY_APK_STORAGE_DIR` (bawaan `<ERAPAY_KYC_STORAGE_DIR>/apk`), metadata di `app_releases` (append-only, versi unik, diaudit). Unggahan harus berupa ZIP berisi `AndroidManifest.xml` dan `classes.dex`, maksimal 60 MB. Web server wajib menaikkan `BODY_SIZE_LIMIT` (adapter-node bawaan 512 KB). APK debug ditandatangani kunci debug mesin pembuatnya: build dari mesin lain tidak bisa menimpa pemasangan lama. Untuk distribusi tetap, tandatangani dengan keystore sendiri dan simpan keystore itu aman.
