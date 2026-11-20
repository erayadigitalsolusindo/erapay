# DANA (calon adapter `dana`), QRIS Acquirer MPM

Sumber: dokumentasi resmi DANA Merchant Portal (`https://dashboard.dana.id/api-docs-v2/llms/...`, disalin Seira dan dibaca 20 Nov 2026): `guide/authentication/authentication-asymmetric`, `api/overview`, `api/qris-acquirer/{overview,generate-qris,finish-notify}`, `.../optional-api/{query-payment,cancel-order,refund-order}`, `guide/settlement-file/payment-services`. Kode: `services/api/internal/gateway/dana` (signing, create QRIS, inquiry, uji koneksi, verifikasi Finish Notify; tes dengan server palsu dan key RSA buatan sendiri). **Belum diuji ke sandbox DANA.** Terdaftar di `cmd/api/gateways.go` (hanya bila kredensial disimpan di panel, tanpa fallback env), `gwcred` (form kredensial), `routing`, `onboarding`, akun clearing `cash_dana_clearing` (migrasi `0056`) dan label panel. Tanpa kredensial di panel adapter tidak ada, jadi efektif OFF. Belum untuk uang nyata. Gate aktivasi: `docs/prd/gateway-new.md`.

Lingkup: hanya **QRIS MPM** (EraPay menampilkan QR, pembeli memindai). Tidak dibaca dan tidak dicakup: QRIS CPM, Gapura, DANA Widget, Disbursement, Merchant Management.

Item bertanda **BELUM JELAS** tidak ada atau bertentangan di dokumentasi. Jangan ditebak di kode; uji di sandbox atau tanyakan ke DANA (Discord developer, tertaut di docs mereka).

## Kredensial

Per environment, dari dasbor DANA (sandbox: `https://dashboard.dana.id/sandbox/`).

| Kunci | Isi |
|---|---|
| `merchant_id` | `merchantId` di body |
| `client_id` | Header `X-PARTNER-ID` (juga disebut clientId) |
| `private_key` | RSA-2048 (PKCS#8 atau PKCS#1). Secret. Penanda tangan request. |
| `store_id` | `storeId` di Generate QRIS (**wajib**). Dari mana nilainya: **BELUM JELAS** (kemungkinan externalShopId dari Create Shop). |
| `sub_merchant_id` | Opsional, harus terdaftar lewat Create Division. |
| `channel_id` | Header `CHANNEL-ID`, maks 5 karakter. Nilai yang benar: **BELUM JELAS** (contoh docs `95221`). |
| `dana_public_key` | Memverifikasi webhook. Docs mencantumkan satu key; apakah itu sandbox saja: **BELUM JELAS**. Key produksi diminta ke tim DANA. |
| `callback_url` | Alamat Finish Notify, didaftarkan di dasbor developer. Host API, bukan host web. |

`Client Secret` dikeluarkan DANA tapi jalur asymmetric tidak memakainya. Jangan taruh nilai asli di repo, docs, atau `.env` yang di-commit.

## Host

| Environment | Base URL |
|---|---|
| Sandbox | `https://api.sandbox.dana.id` |
| Produksi | `https://api.saas.dana.id` |

Host mengikuti environment. `api/overview` menyebut body "form-encoded", tetapi semua endpoint QRIS menyatakan `application/json`. Ikuti halaman endpoint.

## Tanda tangan keluar (X-SIGNATURE)

Tanpa access token untuk API transaksional.

```
stringToSign = METHOD + ":" + relativePath + ":" + lowercase(hex(sha256(minify(body)))) + ":" + X-TIMESTAMP
X-SIGNATURE  = base64(RSA-SHA256-PKCS1v15(privateKey, stringToSign))
```

- `X-TIMESTAMP`: `YYYY-MM-DDTHH:mm:ss+07:00`, WIB.
- Body harus persis byte yang dihash: minify (tanpa spasi), lalu kirim byte yang sama.
- Header lain: `Content-Type: application/json`, `X-PARTNER-ID`, `X-EXTERNAL-ID` (unik per hari, maks 36), `CHANNEL-ID`, `ORIGIN` (opsional).
- Timeout yang diharapkan DANA: 8 detik.
- **BELUM JELAS**: apakah `relativePath` memuat query string (tidak ada query di API QRIS), dan apakah "minify" menjaga urutan key (jaga urutan struct di Go).

## Panggilan keluar

| Panggilan | Path | SNAP code |
|---|---|---|
| Generate QRIS | `POST /v1.0/qr/qr-mpm-generate.htm` | 47 |
| Query Payment | `POST /rest/v1.1/debit/status` | 55 (body `serviceCode: "47"`) |
| Cancel Order | `POST /v1.0/debit/cancel.htm` | 57 |
| Refund Order | `POST /v1.0/debit/refund.htm` | 58 |

**Generate QRIS.** Body wajib: `merchantId`, `storeId`, `partnerReferenceNo`, `amount{value,currency}`, `additionalInfo.envInfo{sourcePlatform="IPG", orderTerminalType, terminalType}` (kedua terakhir salah satu `APP|WEB|WAP|SYSTEM`; server-ke-server memakai `SYSTEM`). Opsional: `validityPeriod` (waktu kedaluwarsa absolut WIB, **bukan** durasi), `feeAmount`, `terminalId`, `subMerchantId`. Respons sukses: `responseCode "2004700"`, `qrContent` (string EMVCo MPM, maks 512), `referenceNo` (id di DANA, opsional), `qrUrl`, `qrImage` (opsional, mungkin kosong).

**Dua jebakan uang dan id:**
1. `amount.value` = string rupiah dengan 2 desimal, mis. `"10000.00"`. EraPay menyimpan rupiah utuh (integer, seperti adapter lain). Adapter mengirim `<rupiah>.00` dan hanya menerima `10000` atau `10000.00`; pecahan sungguhan ditolak, tidak dibulatkan, tanpa float. Laporan settlement memakai **sen** (`TXN_AMOUNT` Long), beda satuan.
2. `partnerReferenceNo` **maks 25 karakter untuk QRIS**. UUID payment EraPay (36) tidak muat. Keputusan: 128 bit UUID ditulis base36 selalu tepat 25 karakter (36^25 > 2^128), huruf kecil, diisi nol di depan. Bisa dibalik (`dana.LongRef`), jadi tanpa tabel pemetaan, dan webhook langsung kembali ke payment id. Hanya payment id berupa UUID kanonik huruf kecil yang diterima; selain itu ditolak sebelum memanggil DANA. **BELUM JELAS**: apakah DANA menerima huruf di `partnerReferenceNo`.

**Idempotensi.** Retry timeout/respons aneh memakai `partnerReferenceNo` yang sama. Parameter berbeda dengan referensi sama dijawab `4044718 Inconsistent Request`. Perilaku create ulang dengan referensi sama dan parameter sama (apakah mengembalikan `qrContent` yang sama): **BELUM JELAS**, uji di sandbox seperti di finpay.

## Kode respons

Format `HTTP-status(3) + service(2) + case(2)`, mis. `2004700`. Pemetaan ke tindakan, dari tabel DANA:

Generate: `2004700` sukses; `4004700/01/02`, `4014700`, `4034702` (limit), `4034705`, `4034715`, `4044708` (merchant tak valid), `4044718` → gagal; `4294700` → gagal, coba lagi berkala; `5004700` → gagal; **`5004701` → pending** (create tidak pasti, selesaikan dengan Query, jangan buat ulang dengan id baru); timeout → ulang maks 3x lalu gagal. Awalan respons `202`/`5xx` atau field kosong → gagal.

Catatan: untuk Generate, DANA menyuruh menandai **timeout 3x sebagai gagal**, sedangkan untuk uang lebih aman memastikan dulu dengan Query (404/`4045501`/status `07` → baru gagal). Ikuti kebijakan EraPay `gateway.IsAmbiguous` (tidak ada failover setelah create ambigu).

Query: `2005500` sukses (lihat status); `4045501` tidak ditemukan; `4295500`, `5005501`, timeout → pending; `400550x`, `4015500`, `5005500` → query gagal, payment tetap pending.

Cancel: `2005700`; `2025700` Request In Progress (pending, kirim ulang payload sama); `4035700` Transaction Expired (tidak bisa dibatalkan); `4045700` status tidak valid; `4045701` tidak ditemukan; `4035714` saldo merchant kurang.

Refund: `2005800`; `2025800` pending; `4035802` melebihi limit; `4035814` saldo kurang; `4045800` status tidak valid; `4045813` amount tidak cocok; `4045818` Inconsistent Request (pending, selidiki).

## Status

Query Payment `latestTransactionStatus`:

| Kode | Arti DANA | Pemetaan adapter |
|---|---|---|
| `00` | Sukses, final, sudah dibayar | succeeded |
| `01` | Initiated, belum dibayar | pending |
| `02` | Paying, belum final, **pembayaran sukses** | pending (lihat catatan) |
| `05` | Cancelled / order ditutup (Finish Notify: "closed because it is expired") | expired |
| `07` | Tidak ditemukan | failed |

Docs DANA menyuruh `02` dianggap sukses. Karena belum final, usulan EraPay: tetap pending sampai `00` atau Finish Notify; **BELUM JELAS** apakah `02` bisa berbalik. Putuskan setelah uji sandbox.

`additionalInfo.statusDetail.acquirementStatus`: `INIT`, `SUCCESS`, `CLOSED`, `PAYING`, `MERCHANT_ACCEPT`, `CANCELLED`. Dipakai hanya sebagai info tambahan.

Hanya `01`/`00` yang jelas teramati dari contoh docs; belum satu pun teramati di sandbox.

## Callback masuk (Finish Notify)

`POST /v1.0/debit/notify` ke `callback_url`. Header sama seperti request biasa (`X-SIGNATURE`, `X-TIMESTAMP`, `X-PARTNER-ID`, `X-EXTERNAL-ID`, `CHANNEL-ID`).

- Body penting: `originalPartnerReferenceNo`, `originalReferenceNo`, `merchantId`, `amount{value,currency}`, `latestTransactionStatus` (**hanya `00` sukses atau `05` ditutup/kedaluwarsa**), `createdTime`, `finishedTime`, `additionalInfo.paymentInfo{paidTime, payOptionInfos[]{payMethod,payOption,payAmount,chargeAmount}}`, `additionalInfo.extendInfo.closedReason` (bila ditutup).
- Verifikasi: string `POST:<path>:lowercase(hex(sha256(minify(body)))):X-TIMESTAMP`, diverifikasi dengan **public key DANA** (kebalikan dari signing keluar). Gagal berarti tolak.
- Path yang ditandatangani: adapter menerima path URL yang dipanggil DANA **atau** `/v1.0/debit/notify`. **BELUM JELAS** yang mana yang DANA pakai (tangkap satu callback sandbox), dan toleransi `X-TIMESTAMP` untuk anti-replay, dan format persis minify pihak DANA. Tangkap callback sandbox nyata sebelum menulis verifikasi, dan verifikasi atas byte mentah yang diterima, jangan re-encode.
- Callback tidak boleh mengubah status sendirian: cocokkan dengan Query Payment dulu (pola sama dengan IDS dan finpay).
- Balasan: JSON `{"responseCode":"2005600","responseMessage":"Successful"}` (handler webhook memakainya lewat `gateway.WebhookAcker`; gateway lain tetap menerima `{"status":"ok"}`). Signature salah dijawab 400 dengan body umum EraPay, bukan format DANA. Jawab `5005601` bila gagal diproses; DANA mengulang berkala sampai **7 hari**, maka handler harus idempoten dan menangani urutan terbalik atau duplikat.
- `payOption` mencakup `NETWORK_PAY_PG_QRIS` dan dompet lain (acquirer QRIS menerima e-wallet lain), jadi `payMethod` bukan hanya DANA.

## Cancel dan Refund

- Cancel memakai `originalPartnerReferenceNo` dan `merchantId` wajib, `amount` opsional. Docs menaruh Cancel **setelah pembayaran** pada alur MPM ("merchant error, void transaksi"). Apakah Cancel bisa membatalkan QR yang belum dibayar: **BELUM JELAS**. Kode `4035700` menunjukkan ada batas waktu cancel. Jangan iklankan cancel sebelum diuji.
- Refund: `partnerRefundNo` wajib (kunci idempotensi refund), `refundAmount` (parsial didukung secara tersirat), `additionalInfo.asyncRefund` (`true`/`false`). Respons sukses memuat `refundNo`, `refundTime`. Refund mengurangi **saldo merchant di DANA** (`4035814` saldo kurang): model dana dan siapa yang menanggung: **BELUM JELAS**.
- Jangan tampil sebagai supported di UI sebelum flag ON dan terkonfirmasi.

## Settlement

File CSV harian, delimiter `;`, nama `MERCHANT_SETTLEMENT_<MID>_<UNIT>_<UNITNAME>_<YYYYMMDD>.csv`, cut-off 00:00, payout T+X (X >= 1, dipilih merchant), unit `MERCHANT`/`DIVISION`/`SHOP`. Kolom tabel: `SETTLEMENT_TXN_ID, TXN_ID, TXN_TYPE (ACQUIRING_ORDER|VOUCHER_DEDUCTION|CAPTURE|REFUND|CANCEL), ACQUIREMENT_ID (= referenceNo), MID, SETTLE_DATE, MERCHANT_NAME, TXN_AMOUNT (sen), TXN_CURRENCY, TXN_DATE, ORIGINAL_TXN_ID, ORIGINAL_TXN_DATE, MERCHANT_COMMISSION_EDIK, SERVICE_TAX_EDIK, WITHHOLDING_TAX_EDIK, MERCHANT_COMMISSION_ASR, SERVICE_TAX_ASR, WITHHOLDING_TAX_ASR`. Contoh CSV memuat kolom tambahan di luar tabel (`SETTLE_AMOUNT, MERCHANT_TRANS_ID, MERCHANT_REQUEST_ID, PAY_METHOD, ..., PRODUCT_TYPE` termasuk `MPM (Kode QR Penjual)`); `MERCHANT_TRANS_ID` kemungkinan = `partnerReferenceNo`.

**BELUM JELAS**: cara pengiriman file (SFTP, email, unduh), apakah dokumen ini berlaku untuk QRIS Acquirer (halaman "Related Solution" hanya menyebut Gapura dan Widget, tetapi overview QRIS menautkannya), arti dua set kolom fee `EDIK` vs `ASR`, dan model fee/MDR QRIS (tarif, siapa yang menanggung). Parser settlement harus terpisah (gate poin 5).

## Gate aktivasi (`gateway-new.md`)

| # | Syarat | Status |
|---|---|---|
| 3 | Docs ini | Ditulis; banyak BELUM JELAS |
| 4 | Contract test | Sebagian: success, failure, ambiguous/timeout, 5xx, signature salah/hilang/dirusak, webhook duplikat (dedup key), inquiry. Belum: webhook out-of-order terhadap domain (itu di lapisan webhook) dan uji ke sandbox nyata |
| 6 | Feature flag per gateway dan merchant, default OFF | Per gateway: OFF sampai kredensial disimpan di panel. Per merchant: lewat routing DEWA dan `merchant_gateway_accounts` (platform account, seperti finpay) |
| 1, 2, 5 | Pilot DOKU, kontrak dan due diligence DANA, fee/ledger/rekonsiliasi | Belum, wajib sebelum uang nyata |

## Yang harus dijawab sebelum kode (urut prioritas)

1. Skema referensi pendek untuk `partnerReferenceNo` (maks 25 karakter).
2. Asal `storeId` dan nilai `CHANNEL-ID`.
3. Tangkap callback sandbox nyata: path yang ditandatangani, timestamp, urutan key.
4. Perilaku create ulang dengan referensi sama, dan status `02`.
5. Pengiriman dan format settlement QRIS, model fee.
