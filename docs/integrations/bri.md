# Integrasi Bank BRI (BRIAPI SNAP-BI) [gw-bri]

Status: **rencana, belum ada kode.** Sumber: `docs/integrations/BRI Dev SNAP/*.html` (versi SNAP; dokumentasi non-SNAP diabaikan atas keputusan pemilik). Bagian callback bersumber dari halaman "QRIS MPM Dinamis Notification v1.1" yang pernah dibaca, file-nya tidak ada lagi di repo (lihat §5).

## 1. Peran BRI di EraPay
- EraPay = agregator. Uang pembayaran masuk ke rekening BRI penampung milik pemilik. Klien ARUS meminta penarikan; EraPay membayar ke rekening mereka dan mengambil selisih (margin) per transaksi. Klien tidak berhubungan langsung dengan BRI.
- BRI adalah satu jalur di samping IDS. DEWA memilih jalur di panel (Gateway → Jalur pembayaran, `payment_routing`), tanpa keterikatan pada DOKU. BRI memakai satu akun platform (`Capabilities.PlatformAccount = true`), seperti IDS.
- Gate `gateway-new.md`: pengecualian untuk BRI perlu dicatat di file itu (keputusan pemilik, 2026-11-04), sama seperti IDS. Belum ada dana nyata sebelum paket harga dan akun ledger BRI diputuskan dan go/no-go.

## 2. Capability matrix

| Fitur | API BRI | Adapter |
|---|---|---|
| QRIS MPM dinamis: buat QR | `POST /snap/v1.1/qr/qr-mpm-generate` | `CreatePayment` |
| QRIS: cek status | `POST /snap/v1.1/qr/qr-mpm-query` | `GetPayment` |
| QRIS: callback | BRI memanggil kita (§5) | `WebhookVerifier` |
| QRIS: cancel / refund | tidak ada di dokumen | tidak diiklankan |
| Payout ke rekening BRI | `POST /intrabank/snap/v2.0/transfer-intrabank` (+ `account-inquiry-internal`) | `Disburser` |
| Payout ke bank lain / BI-FAST | `POST /interbank/snap/v2.0/transfer-interbank` (+ `account-inquiry-external`) | `Disburser` |
| Status transfer | `POST /snap/v2.0/transfer/status` | `Disburser.TransferStatus` |
| Settlement report, fee/MDR | tidak ada di dokumen | [?] tanya BRI |

## 3. Autentikasi dan signature
- Host sandbox `https://sandbox.partner.api.bri.co.id`; produksi [?] belum tercantum.
- Kunci RSA 2048 dibuat sendiri; public key didaftarkan di devportal (Aplikasi Saya → Manage Snap Key). Private key hanya di env/secret store.
- Token: `POST /snap/v1.0/access-token/b2b`. Header `X-CLIENT-KEY`, `X-TIMESTAMP`, `X-SIGNATURE` = `SHA256withRSA(privateKey, clientId + "|" + timestamp)`. Body `{"grantType":"client_credentials"}`. Respons `accessToken`, `expiresIn` 900 detik: cache dan refresh sebelum habis, satu refresh per proses (singleflight).
- Request bisnis: `Authorization: Bearer`, `X-TIMESTAMP`, `X-SIGNATURE`, `X-PARTNER-ID` (≤36), `CHANNEL-ID` (≤5), `X-EXTERNAL-ID` (≤36, unik per request, mis. turunan dari reference + percobaan).
- `X-SIGNATURE` bisnis = `HMAC_SHA512(clientSecret, METHOD ":" path ":" accessToken ":" lowerhex(sha256(minify(body))) ":" timestamp)`. Path tanpa host dan query. Timestamp ISO8601 `yyyy-MM-ddTHH:mm:ss.SSS+07:00`. Body kosong: hash string kosong.
- Env: `BRI_BASE_URL`, `BRI_CLIENT_ID`, `BRI_CLIENT_SECRET`, `BRI_PRIVATE_KEY[_FILE]`, `BRI_PUBLIC_KEY_FILE`, `BRI_PARTNER_ID`, `BRI_CHANNEL_ID`, `BRI_MERCHANT_ID`, `BRI_TERMINAL_ID`, `BRI_SOURCE_ACCOUNT_NO` (lihat `.env.example`). Pola akhirnya mengikuti DOKU/IDS: kredensial lewat panel DEWA terenkripsi.

## 4. QRIS MPM Dinamis
- Generate: body `partnerReferenceNo`, `amount{value:"10000.00",currency:"IDR"}`, `merchantId`, `terminalId`. Respons sukses `2004700`: `qrContent`, `referenceNo` (12).
- Inquiry: body `originalReferenceNo`, `serviceCode`, `additionalInfo.terminalId`. Respons `2005100`: `latestTransactionStatus`, `paidTime`, `amount`, `additionalInfo.issuerRrn/issuerName/customerName/mpan`.
- Amount: integer rupiah di domain, string `"%d.00"` ke BRI; respons di-parse ke integer dan ditolak bila pecahan bukan `.00`.
- Mapping status (`MapStatus`): `00`→succeeded, `01/02/03`→pending, `04`→ tidak ada mapping domain (refunded; tidak diterapkan, `ok=false`), `05`→cancelled, `06`→failed, `07`→ pending (belum tercatat; jangan dianggap gagal), selain itu `ok=false`. Kadaluarsa QR (`403 15 QR Expired`) → expired. Mapping dikunci sampai terverifikasi di sandbox (seperti `DOKU_QRIS_STATUS_MAP_CONFIRMED`).
- Klasifikasi error: kode yang tercantum Gagal (400 01, 403 02/15/23, 404 08/11/12/13) = Rejected. `404 14 Paid Bill` = sudah dibayar: selesaikan lewat inquiry. 202, 409, 5xx, timeout, dan semua kode di luar daftar = Ambiguous: selesaikan lewat inquiry, jangan buat ulang.
- Idempotency: `partnerReferenceNo` unik per payment. Panjang field: docs menulis 6, contoh 13 digit [?]; bila ada batas pendek, pakai alias pendek unik yang disimpan di payment.

## 5. Callback QRIS (BRI → EraPay)
- Spek callback: `POST …/qr-mpm-notify` di sisi kita. Header: `Authorization: Bearer <token yang KITA terbitkan>`, `X-SIGNATURE` (SHA256withRSA, `stringToSign = clientId|X-TIMESTAMP`), `X-TIMESTAMP`, `X-PARTNER-ID`, `X-EXTERNAL-ID` (unik per notifikasi → `DedupKey`), `CHANNEL-ID`.
- Kita juga harus menyediakan endpoint token (form-urlencoded) agar BRI mendapat Bearer.
- Body: `originalReferenceNo` (12), `originalPartnerReferenceNo` (**hanya 6 digit terakhir**), `latestTransactionStatus`, `amount`, `customerNumber`, `destinationAccountName`, `additionalInfo{reffId,issuerName,issuerRrn}`. Data pembayar (nama, nomor) dibuang saat parsing (tanpa PII).
- Balasan: `{"responseCode":"2005200","responseMessage":"Successful"}`; error 400 52 01/02, 500 52 00/01.
- Aturan EraPay: callback bukan satu-satunya sumber kebenaran. Tetap ada inquiry sebagai cadangan; payment dicocokkan lewat `originalReferenceNo` (referensi BRI yang kita simpan), bukan 6 digit.
- Masih perlu BRI: public key BRI untuk verifikasi `X-SIGNATURE`; `client_id` apa yang masuk `stringToSign`; path callback versi SNAP (docs lama memakai `/v1.1/qr-dynamic/...`); URL yang didaftarkan.

## 6. Payout (agregator → rekening klien ARUS)
- Pemisahan tegas: QRIS hanya untuk MENERIMA pembayaran; QRIS tidak bisa dipakai untuk payout. Payout hanya lewat API transfer (intrabank/interbank/BI-FAST) dari rekening penampung. Adapter QRIS tidak mengiklankan `Disbursement`; `Disburser` terpisah dari `Adapter`.
- Sumber dana: rekening BRI penampung (`BRI_SOURCE_ACCOUNT_NO`). Akun ledger sumber dana ditentukan di `ERAPAY_PAYOUT_CASH_ACCOUNT`; perlu akun untuk BRI [KD].
- Pilih rute per tujuan: rekening BRI → intrabank (17); bank lain → BI-FAST (80) atau interbank (18). Inquiry rekening dulu (nama dicocokkan dengan data merchant), lalu transfer.
- `partnerReferenceNo` ≤20 (intrabank/interbank), ≤28 (BI-FAST), unik per percobaan. Duplikat = 409 Pending: jangan kirim ulang.
- Simpan `transactionDate` dan `serviceCode` asli di payout: wajib untuk `transfer/status`.
- Status transfer: `00` sukses, `01` initiated, `03` pending, `06` gagal. Endpoint hanya di luar EOD; transfer pending saat EOD dicek ulang pukul 08:00 WIB. `404 36 01` bukan gagal pasti.
- Pending 202/403 16/409/5xx/504/kode tak dikenal = Ambiguous.
- Intrabank punya `feeType` (OUR/BEN/SHA|n); kebijakan fee mengikuti `pricing.md` (margin EraPay per transaksi). Interbank/BI-FAST wajib data pengirim (`senderIdentityNumber`, `senderType`, `senderResidentStatus`, `senderTownName`) dan, untuk BI-FAST, `referenceNo` + `externalId` dari inquiry.
- Payout tetap dry-run (`ERAPAY_PAYOUT_LIVE` kosong) sampai go/no-go.

## 7. Pertanyaan terbuka ke BRI
1. Public key BRI dan `client_id` di `stringToSign` callback; path callback SNAP; cara mendaftarkan URL callback.
2. `serviceCode` inquiry QRIS (contoh "47") dan panjang sebenarnya `partnerReferenceNo`.
3. URL produksi; sandbox: apakah ada simulator pembayaran QR.
4. Fee/MDR QRIS dan biaya transfer (intra, inter, BI-FAST); laporan settlement (format, jadwal) untuk rekonsiliasi.
5. `merchantId`/`terminalId`: satu untuk platform, atau per klien ARUS.
6. Limit transaksi/harian rekening penampung dan jam EOD sebenarnya.
7. Legal: izin yang dibutuhkan untuk menampung dan menyalurkan dana klien (PJP) dan batas peran EraPay terhadap BRI.

## 8. Rencana implementasi
1. Catat pengecualian BRI di `gateway-new.md`; isi pertanyaan §7 ke BRI.
2. Adapter `internal/gateway/bri`: signer, token manager, `do()` dengan klasifikasi Rejected/Ambiguous, QRIS create/get/map, `TestConnection`.
3. Wiring di `cmd/api`, `payment_routing`, kredensial panel DEWA.
4. Callback + endpoint token (setelah §5 terjawab).
5. `Disburser` intra/inter/BI-FAST + status; akun ledger dan paket harga BRI.
6. Uji: idempotency, concurrency, contract test (sukses, pending, gagal, timeout, signature salah, callback duplikat dan out-of-order).
