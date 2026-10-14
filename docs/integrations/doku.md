# Integrasi DOKU — catatan dokumentasi resmi

Sumber: https://developers.doku.com/ (indeks: `/llms.txt`), dibaca 11 Okt 2026. Wajib per PRD §0 #10: tidak ada endpoint/field/signature yang boleh ditulis ke kode tanpa tercantum di sini. Tanda:
- **[V]** tertulis di halaman resmi (ringkasan dari fetch; verifikasi ulang ke halaman sumber saat implementasi).
- **[?]** tidak ditemukan / ambigu → `OPEN QUESTION`, jangan diimplementasikan. Konfirmasi ke DOKU (**[KD]**).

Halaman SNAP/QRIS: https://developers.doku.com/accept-payments/direct-api/snap/integration-guide/qris.md

## 1. Pilihan jalur integrasi
- Direct API **SNAP** (standar BI) mencakup QRIS MPM. Jalur lama ada di `archive/non-snap/qris` — jangan dipakai. **[V]**
- Base URL: sandbox `https://api-sandbox.doku.com`, prod `https://api.doku.com`. **[V]**
- Simulator QRIS sandbox: `https://sandbox.doku.com/qris-simulator/`. **[V]**

## 2. Auth & signature (SNAP)
**Token B2B** — `POST /authorization/v1/access-token/b2b` **[V]**
- Header: `X-CLIENT-KEY` (client id), `X-TIMESTAMP` (UTC ISO8601, mis. `2020-09-22T01:51:00Z`), `X-SIGNATURE`, `Content-Type: application/json`.
- `X-SIGNATURE` = SHA256withRSA(private key merchant, `client_ID + "|" + X-TIMESTAMP`), base64.
- Body `{"grantType":"client_credentials"}`. Response: `accessToken`, `tokenType: Bearer`, `expiresIn: 900` (15 menit) → cache + refresh sebelum kedaluwarsa.

**Request transaksional (symmetric)** **[V]**
- `stringToSign = HTTPMethod:EndpointUrl:AccessToken:lowercase(hex(SHA256(minify(body)))):Timestamp`
- `X-SIGNATURE = base64(HMAC-SHA512(clientSecret, stringToSign))`. `AccessToken` tanpa prefix `Bearer`. `EndpointUrl` = path (bukan host).
- Body yang di-hash harus identik byte-per-byte dengan yang dikirim.
- Header: `X-PARTNER-ID` (client id), `X-EXTERNAL-ID` (string angka, unik per hari), `X-TIMESTAMP` (`YYYY-MM-DDTHH:mm:ssZD`), `X-SIGNATURE`, `Authorization: Bearer <token>`, `CHANNEL-ID: H2H`.

## 3. QRIS MPM **[V]** (semua `POST`, prefix `/snap-adapter/b2b/v1.0/qr/`)
| Fungsi | Path |
|---|---|
| Generate | `qr-mpm-generate` |
| Query/inquiry | `qr-mpm-query` |
| Refund | `qr-mpm-refund` |
| Cancel (expire) | `qr-expire` |

**Generate** — wajib: `partnerReferenceNo` (≤64), `amount.value` (pola `^\d{1,16}\.00$`, mis. `10000.00`), `amount.currency` (`IDR`), `merchantId` (dari DOKU setelah registrasi QRIS disetujui), `terminalId` (alfanumerik 3–16), `additionalInfo.postalCode`, `additionalInfo.feeType` (hanya `1`). Opsional: `validityPeriod` (ISO 8601; **default 30 hari bila kosong** → selalu kirim eksplisit). Response: `referenceNo`, `partnerReferenceNo`, `qrContent`, `terminalId`, `additionalInfo`.

**Query** — wajib: `originalReferenceNo`, `originalPartnerReferenceNo`, `serviceCode` = `47`, `merchantId`. Response: `latestTransactionStatus`, `transactionStatusDesc`, `paidTime`, `amount`, `feeAmount`, `additionalInfo.{approvalCode, issuerId, issuerName, customerName, convenienceFee, terminalId}`.
- Kode `latestTransactionStatus` yang terdokumentasi di halaman check-status lain (VA/DD/E-Wallet): `00` success, `01` initiated, `02` paying, `03` pending, `04` refunded, `05` canceled, `06` failed, `07` not found. **[?] belum terbukti berlaku sama untuk QRIS query** (halaman QRIS tidak mencantumkan daftar).

**Refund** — wajib: `originalPartnerReferenceNo`, `partnerRefundNo`, `merchantId`, `refundAmount`, `reason`, `additionalInfo.approvalCode` (dari query/notifikasi). Dukungan refund bergantung issuer. **[?]** deskripsi field `original*` di halaman tidak konsisten; verifikasi di sandbox/DOKU.

**Cancel** — wajib `partnerReferenceNo`, `referenceNo`, `merchantId`; mengubah expiry QR ke sekarang.

## 4. Notifikasi (webhook inbound)
- Verifikasi (SNAP): `stringToSign = METHOD:RELATIVE-URL:ACCESS-TOKEN:hex(SHA256(body)):TIMESTAMP`, HMAC-SHA512 dengan `clientSecret`, bandingkan dengan `X-SIGNATURE`. **[V]** tapi **[?] sumber `ACCESS-TOKEN` untuk notifikasi tidak dijelaskan.**
- Header notifikasi: `X-PARTNER-ID`, `X-EXTERNAL-ID` (unik per notifikasi → kunci dedupe), `X-TIMESTAMP`, `X-SIGNATURE`, `CHANNEL-ID`. **[V]**
- Balasan: HTTP 200 cukup (body opsional `responseCode`/`responseMessage`). Retry DOKU: 3x, di +30 menit, +6 jam, +12 jam; juga retry manual lewat Back Office. **[V]**
- Best practice DOKU: balas 2xx cepat dan pisahkan dari logika berat, dedupe event, parse JSON non-strict. **[V]**
- **Terbukti dari notifikasi QRIS sandbox asli (12 Okt 2026)**: format **non-SNAP**, bukan SNAP di atas. Header `Client-Id` (= Brand ID), `Request-Id` (unik per notifikasi → kunci dedupe), `Request-Timestamp`, `Signature: HMACSHA256=<base64>`. **Tidak ada** `Authorization`/`X-SIGNATURE`/`ACCESS-TOKEN`, jadi pertanyaan sumber token gugur. Body JSON: `order.invoice_number` (= `payments.id` kita), `order.amount` (angka desimal, mis. `1000.00`), `transaction.status` (`SUCCESS`), `transaction.date`, `transaction.original_request_id`; ditambah data pembayar (`customer.*`, PII, tidak dipakai/disimpan oleh parser). Tidak ada currency (QRIS = IDR).
- Verifikasi signature (dokumen DOKU non-SNAP; **[V]** rumus, belum dicocokkan dengan secret sandbox kita → jalankan `webhookverify` di server): `HMACSHA256=` + base64(HMAC-SHA256(`DOKU_SECRET_KEY`, komponen dipisah `\n` tanpa newline akhir: `Client-Id:…`, `Request-Id:…`, `Request-Timestamp:…`, `Request-Target:<path URL notifikasi kita>`, `Digest:` base64(SHA-256 body mentah))).
- Mapping status **webhook**: hanya `SUCCESS` → `succeeded` (satu-satunya yang teramati). Status lain dicatat tapi tidak diterapkan (poller inquiry yang memutuskan).
- Sumber status inquiry (`qr-mpm-query`) teramati di sandbox: `03` = menunggu bayar (pending), `00` = sudah dibayar. Cocok dengan hipotesis §8; aktifkan `DOKU_QRIS_STATUS_MAP_CONFIRMED=true` setelah konfirmasi owner.
- Daftar IP DOKU / whitelist: tidak ditemukan. **[?]**

## 5. Idempotency & error
- DOKU memakai Request ID/`X-EXTERNAL-ID` sebagai kunci; request ulang dengan ID + body sama → response sama dengan HTTP `409`. Durasi dan API yang mendukung tidak disebut. **[V]/[?]**
- Format `responseCode` SNAP = HTTP(3) + service(2) + case(2). Timeout (`504…`) dan pending (`202…`, `500…`) pada transfer = **pending, bukan gagal** (cocok dengan invarian #3). **[V]**
- Check Status disarankan dipanggil ≥60 detik setelah pembayaran. **[V]**

## 6. Settlement, split, sub-account, payout (relevan Model B)

Diperbarui 14 Okt 2026: gabungan jawaban **lisan** DOKU (tiket/pertemuan), verifikasi ulang dokumentasi publik (`developers.doku.com` + `docs.doku.com`), dan halaman harga `https://www.doku.com/harga`. Semua yang dari mulut DOKU = **[KD] menunggu konfirmasi tertulis**; bukan dasar go/no-go.

**Settlement QRIS (lisan DOKU, [KD] tertulis):**
- **T+1 hari kerja**, diproses setiap hari kerja **12:00–14:00 GMT+7**. Dana baru benar-benar di rekening PT setelah jendela itu → job settlement/rekonsiliasi dijadwalkan **setelah 14:00 GMT+7** (lihat `prd/reconciliation.md`).
- Pencairan settlement QRIS ke rekening bank: **tanpa biaya tambahan**.
- Format & jalur akses laporan settlement tetap **tidak ditemukan** di dokumentasi publik → parser rekonsiliasi tetap tertahan; Finance mencatat batch manual dulu.

**MDR QRIS (lisan DOKU, [KD] tertulis):**
- **0,7%** dengan **kap maks Rp500 untuk nominal Rp0–Rp100.000** (aturan Bank Indonesia), **berlaku 1 Nov 2026**. Basis (gross/net) + pembulatan akhir mengikuti kontrak.
- ⚠️ **Konflik sumber:** halaman harga publik menulis QR "0,7% — 0% untuk transaksi Rp0–Rp100.000 (DOKU Fee berlaku)", berbeda dari "0,7% kap 500" di atas. Yang berlaku = **kontrak tertulis DOKU** (belum final).
- Implikasi kode: **sudah diimplementasikan 14 Okt 2026** (`pricing.Rate.Cap` + `Rate.CapUpTo`, migration `0020_pricing_mdr_cap.sql`, env `ERAPAY_MDR_CAP`/`ERAPAY_MDR_CAP_UPTO`, bootstrap `ERAPAY_BOOTSTRAP_MDR_CAP`/`..._CAP_UPTO`; default 0 = tanpa kap — lihat `prd/pricing.md`()). **Rollout 1 Nov 2026:** set env, lalu assign plan v2 berkapsul ke merchant yang sudah approved (tarif bersifat snapshot; tidak berubah retroaktif).

**Sub-Account V2** (`wallet-as-a-service/sub-account`, `docs.doku.com`): satu merchant DOKU (PT) menampung sub-akun per pihak (seller/agent/merchant EraPay). **[V]** kecuali yang ditandai.
- Tiap sub-akun: `profileId` + maks 3 jenis akun — `DOKU_MERCHANT_IDR` (bisa ditarik), `DOKU_MERCHANT_PENDING_IDR` (pending), `DOKU_MERCHANT_POINT` (non-tunai) — + VA BRI statis otomatis. Register: `POST /sub-account/v2.0/register`.
- Pembayaran Checkout/Direct API mendarat di **PENDING_IDR**; split baru jalan **saat settlement**, dihitung dari **net setelah fee PG** (bukan gross). Ada gap (bisa >1 hari) sebelum saldo tujuan terisi.
- **Tabel kompatibilitas channel (resmi): QRIS = Checkout `Yes`, Direct API `No`**; endpoint Direct API yang disebut hanya VA (`create-va`), Credit Card (`payment-page`), Direct Debit host-to-host.
- Kegagalan senyap: `split_rule_id` / `additionalInfo.account.id` tidak valid → **tidak ada error**, dana masuk tanpa split, harus diselesaikan manual oleh ops.
- Money-out: `transfer-inquiry` → `transfer-payment` (`BANK_ACCOUNT` via BI_FAST/ONLINE, `DOKU_SUB_ACCOUNT`, `DOKU_WALLET`, `DOKU_NON_FIAT`/point); `beneficiaryAccountName` wajib dari hasil inquiry. Monitoring: `balance-inquiries`, `transaction-history-list` (baris `VOID` tidak masuk saldo), `transactions-status`.
- **DOKU (lisan, 14 Okt 2026) mengklaim QRIS Direct API bisa routing ke sub-account dengan `additionalInfo.account.id` + `additionalInfo.account.split_rule_id`** — bertentangan dengan tabel kompatibilitas publik → **[?] OPEN**: minta bukti tertulis + uji sandbox (register sub-akun → `qr-mpm-generate` dengan `additionalInfo.account.id` → bayar di simulator QRIS → cek `balance-inquiries`/`transaction-history-list` sub-akun). Sebelum jelas: **tidak ada kode sub-account**.
- **Cocok/tidak untuk EraPay:** split dihitung dari **net** sedangkan PRD: Biaya Jasa 1% dari **gross** (PERCENTAGE dari net ≠ 1% gross; FLAT hanya akurat untuk satu nominal) → meski routing QRIS-nya terbukti, perhitungan fee tetap wajib di **ledger EraPay**; sub-account paling jadi tempat mirror saldo. Juga perlu aktivasi via Sales (IDR, business account ID). Bukan tujuan EraPay: transfer antar pengguna, points.

**Partner API** (`partnership/partner-api`): program DOKU Partner — platform (mis. EraPay) mendaftarkan merchant-nya ke DOKU **lewat API**, bukan manual di dashboard DOKU. **[V]** Alur: Check Requirements → Upload File (dokumen KYB) → Business Registration (KYB oleh DOKU, hasil via webhook SLA **2×24 jam**; 10 callback: `BRAND.OPERATIONAL_STATUS`, `BUSINESS`, `BUSINESS_CONTACT`, `BUSINESS_ADDRESS`, `BUSINESS_DOCUMENT`, `BUSINESS_BANK`, `BRAND`, `BRAND_SOCIAL_MEDIA`, `BRAND_PHOTO_PROOF`, `BRAND_PRODUCT`) → Update Business Data. Alternatif representasi merchant di sisi DOKU (vs Sub-Account); EraPay baru memakainya di fase P3 ([KD] kontrak). Saat ini aktivasi DOKU per merchant tetap manual (`activation_status = pending_confirmation`).

**Disbursement "Kirim DOKU"** — semua **[V]** dari `developers.doku.com/payout/kirim-doku*` (diambil 14 Okt 2026) kecuali yang ditandai.
- **Flow resmi:** Access Token B2B → **Account Inquiry (wajib sebelum transfer)** → Balance Inquiry (opsional, cek saldo cukup) → Transfer Bank → Check Status. Notifikasi inbound: **Unpaid Notification** (status UNPAID berubah jadi Success/Failed) dan Refund Notification (max 3× retry) — keduanya butuh URL terdaftar, belum diimplementasikan (capture-only dulu, lihat §8).
- **Go-live butuh persetujuan ASPI**: untuk jadi KD SNAP partner harus lalui 2 ujian — *functionality test* (sandbox DOKU) + *developer site test* (portal ASPI) — dikoordinasikan dengan Sales & Integration DOKU. ⚠️ **Ini belum disebutkan DOKU secara lisan** → masuk checklist go/no-go (`prd/roadmap.md`).
- **Header semua panggilan** (sama seperti QRIS SNAP): `Authorization: Bearer <token B2B>`, `X-PARTNER-ID` (client id), `X-EXTERNAL-ID` (numerik, unik per hari), `X-TIMESTAMP`, `CHANNEL-ID`, `X-SIGNATURE` (HMAC-SHA512, rumus sama §2).
- **`POST /snap/v1.1/emoney/bank-account-inquiry`** (verifikasi rekening tujuan): required `customerNumber` (HP pengirim prefix 62/0), `beneficiaryAccountNumber` (maks 32), `beneficiaryBankCode`, `amount{value,currency}`; `additionalInfo` required: `beneficiaryBankCode`, `beneficiaryAccountName`, `senderCountryCode`; optional `channelCode` (`07` Bank Deposit, `11` BI-FAST). Response: `beneficiaryAccountName`, `beneficiaryBankCode/Name/ShortName`, `amount`, **`sessionId`** (maks 25; = "inquiry invoice id" yang **wajib** dikirim ke transfer-bank), `additionalInfo.feeAmount` (biaya transfer sebenarnya, per permintaan).
- **`POST /snap/v1.1/emoney/transfer-bank`**: required `partnerReferenceNo` (maks 64, unik per request = idempotency kita), `customerNumber`, `beneficiaryAccountNumber`, `beneficiaryBankCode`, `amount`, `sessionId` (dari inquiry); `additionalInfo` **required**: `beneficiaryFirstName/LastName/PhoneNumber/AccountName`, `senderCountryCode`, `senderFirstName/LastName`, `senderPersonalId`, `senderPersonalIdType` (`KTP`/`PASSPORT`); optional: `channelCode` (07/11), `beneficiaryBankName`, `remark`, `feeType` (**[?] nilai valid tidak didokumentasikan — jangan kirim dulu**). Response: `referenceNo`, `partnerReferenceNo`, `transactionDate`, `referenceNumber`, `additionalInfo{sessionId, amount}`. 400 = gagal definitif; 409/5xx/timeout = ambigu → **tidak pernah retry buta**, status `unknown` + check-status (rekonsiliasi T+1).
- **Check Status** — **[?] OPEN**: dokumen menulis path `POST /snap/v1.1/qr/qr-mpm-status` (= path QRIS, nyaris pasti salah salin dokumen). Konfirmasi ke DOKU sebelum dipakai; sampai konfirmasi, kode memakai path itu sebagai konstanta tunggal yang mudah diganti. Request required: `originalPartnerReferenceNo`, `serviceCode`, `beneficiaryAccountNumber`, `beneficiaryBankCode`; response: `latestTransactionStatus` `00` Success, `03` Pending, `04` Refunded, `06` Failed (+`transactionDate`, `additionalInfo{cashInTime,remark,beneficiaryAmount}`). `04` = mesin state refund-sendiri, tidak dipetakan ke payout langsung.
- **Bank tujuan:** 125 bank dengan kode BI (alfanumerik, mis. `011SY` untuk Danamon Syariah; tabel lengkap di `kirim-doku.md` — versi ringkas disimpan di `services/api/internal/gateway/doku/kdbanks.go`). E-wallet (GOPAY/DANA/SHOPEEPAY/OVO; LinkAja `911`) hanya via `channelCode=07` dengan **nomor HP sebagai `beneficiaryAccountNumber`**. VA Tokopedia/Bukalapak (`PVATOKO`/`PVABUKA` via Permata) — tidak dipakai EraPay.
- **Catatan uji sandbox sebelum commit**: `sender*` wajib diisi **data PT EraPay** (badan usaha) padahal field-anyamannya personal (`KTP`/`PASSPORT`) → **[?] bagaimana mengisi `senderPersonalId`/`senderPersonalIdType` untuk pengirim badan usaha** (kemungkinan NPWP perusahaan) — tanyakan DOKU; sementara dikonfigurasi via env.
- **Harga (doku.com/harga, belum termasuk PPN):** Domestic Payouts **Paket 1 = Rp1.500 per transfer berhasil** (Rp10.000–250.000.000, 24/7, BI Fast only); Paket 2 = hubungi sales (multi-channel/auto-rute); Paket 3 = Rp35.000 (≥ Rp250.000.000). **[V]** besaran publik; kontrak [KD]. Fee Rp1.500 pada payout minimum Rp10.000 = ~15% → tinjau kebijakan minimum payout (lihat `prd/payout.md`).
- **Bank Account Validation Rp500 per request berhasil** = pemanggilan `bank-account-inquiry` (bank/wallet terdaftar 118+ sesuai tabel di atas) — kandidat menggantikan verifikasi rekening manual oleh Finance. **[V]** Biaya ditanggung merchant; otomatisasi diaktifkan lewat config.
- **[KD] sumber dana Kirim DOKU:** endpoint `/emoney/transfer-bank` menyiratkan sumbernya **saldo DOKU** (lihat juga `balance-inquiry` yang menanyakan *saldo pengirim*), padahal settlement QRIS (di atas) dikatakan masuk ke **rekening bank PT** → tanyakan DOKU: apakah settlement bisa ditahan di saldo DOKU agar Kirim DOKU jadi jalur payout tunggal, atau harus turun ke bank dulu. Ini menentukan rancangan executor **dan** akun ledger yang di-credit (`cash_doku_clearing` vs `cash_bank_pt`).

## 7. Implikasi untuk EraPay (bukan klaim DOKU)
1. **Adapter QRIS bisa mulai** untuk token, signature, generate, query, cancel, karena spesifikasi lengkap dan ada simulator.
2. **Webhook inbound QRIS tertahan**: payload dan sumber access-token verifikasi harus dikonfirmasi/ditangkap di sandbox. Sementara itu status final hanya dari **query** (sesuai invarian #2); polling/inquiry jadi jalur utama.
3. **Pemotongan fee platform 1% (Model B)**: split settlement/sub-account untuk QRIS belum terbukti → asumsi PRD (dana masuk ke PT, fee dipotong di ledger EraPay) tetap, tanpa bergantung fitur split.
4. **Payout**: eksekutor yang dipilih = Kirim DOKU (transfer bank BI Fast), **bukan Sub-Account** (keputusan 14 Okt 2026; lihat §6). Wajib dry-run default, `unknown` saat timeout/pending, inquiry sebelum retry. Biaya transfer Rp1.500 ditanggung merchant, dipotong dari nominal payout; verifikasi rekening: cache-first + DOKU Bank Account Validation untuk yang belum dikenal, otomatisasi via config.
5. **Pertanyaan [KD] yang masih terbuka**: bukti tertulis untuk klaim "QRIS Direct API bisa routing ke sub-account" (konflik dengan tabel kompatibilitas, lihat §6); kode `latestTransactionStatus` QRIS; format/jalur akses laporan settlement QRIS; **sumber dana Kirim DOKU (saldo DOKU vs rekening bank PT)**; refund QRIS per issuer; dukungan satu `merchantId` untuk banyak merchant pihak ketiga. (Terjawab lisan 14 Okt 2026, rincian di §6: MDR QRIS 0,7% kap 500, settlement T+1 12:00–14:00 GMT+7, harga payout.)

## 8. Status implementasi (adapter `internal/gateway/doku`, 11 Okt 2026)
Diimplementasikan (semua [V] di atas): token B2B (cache + refresh 1 menit sebelum kedaluwarsa), signature simetris HMAC-SHA512, `qr-mpm-generate`, `qr-mpm-query` (`serviceCode=47`), `qr-expire`. Diuji dengan server DOKU **tiruan** yang memverifikasi signature secara independen; **belum diuji ke sandbox DOKU asli** (butuh kredensial).
Asumsi yang harus diverifikasi di sandbox:
- `X-TIMESTAMP` selalu UTC `YYYY-MM-DDTHH:mm:ssZ` (dokumen menulis `…ssZD`); `validityPeriod` dikirim sebagai ISO 8601 datetime `+07:00` (dokumen hanya menyebut "ISO 8601").
- Sukses = HTTP 200 dan `responseCode` berawalan `200` (dari format HTTP(3)+service(2)+case(2)). `202…`, 5xx, 409, timeout/koneksi putus = **ambigu** (payment `unknown`, create tidak di-retry); 4xx lain = ditolak (payment `failed`).
- Mapping `latestTransactionStatus` **mati secara default** (`DOKU_QRIS_STATUS_MAP_CONFIRMED=false`) karena kodenya belum terbukti untuk QRIS: semua status "unmapped" dan payment tidak berubah. Setelah dikonfirmasi di sandbox: `00`→succeeded, `01/02/03`→pending, `05`→cancelled, `06`→failed; `04` (refunded) dan `07` (not found) tetap tidak dipetakan.
- `X-EXTERNAL-ID`: epoch mikrodetik×1000 + counter (numerik, unik per proses); belum ada jaminan unik lintas instance.
Webhook capture-only (`ERAPAY_WEBHOOK_CAPTURE=1`): `POST /webhooks/doku[/path-apa-saja]` menyimpan request mentah ke `webhook_captures` (append-only, maks 5000 baris, body maks 64 KB) dan membalas 200 tanpa mengubah payment. Header `Authorization` tidak disimpan, hanya panjang dan SHA-256-nya agar hipotesis sumber token bisa diuji. Format balasan `2005600` adalah asumsi. Gunakan untuk menangkap payload QRIS asli, lalu tulis verifikasi + parser dari data itu.
Deploy domain + SSL via aaPanel: lihat `docs/DEPLOY-AAPANEL.md`.
Webhook terverifikasi (13 Okt 2026): `POST /webhooks/doku[/...]` memverifikasi signature (`internal/gateway/doku/webhook.go`), lalu `Orchestrator.ApplyWebhook` memanggil `Transition(source=webhook)` dengan `dedup_key = doku:webhook:<Request-Id>`. Signature salah → 400. Payment tidak dikenal / status belum dipetakan / nominal beda → dicatat dan dibalas 200 (retry tidak menyelesaikannya; nominal beda tidak pernah diterapkan). Idempotent dan aman paralel (lock baris payment + unique `dedup_key`); poller inquiry tetap jadi pengaman. `ERAPAY_WEBHOOK_CAPTURE=1` kini hanya menyimpan salinan mentah untuk diagnosis (berisi PII pembayar, append-only) dan sebaiknya dimatikan setelah dipakai. Validasi rumus signature terhadap capture asli: `webhookverify <capture-id>` di server (butuh `DOKU_CLIENT_ID`, `DOKU_SECRET_KEY`, `ERAPAY_DATABASE_URL`).
Disbursement Kirim DOKU diimplementasikan (14 Okt 2026): kontrak gateway-netral `gateway.Disburser` (`InquiryBankAccount` → `TransferBank` → `TransferStatus` + `MapTransferStatus`) + implementasi DOKU (`bank-account-inquiry`, `transfer-bank`, check-status) diikuti `Capabilities().Disbursement=true`. Diuji dengan server DOKU tiruan (verifikasi signature independen; body sesuai spec; 409/5xx/timeout = ambigu; response 200 tanpa `sessionId`/`referenceNo`/`latestTransactionStatus` = ambigu; validasi pra-kirim). Belum diuji ke sandbox DOKU asli.
Asumsi yang harus diverifikasi di sandbox (Kirim DOKU):
- `serviceCode` untuk transfer-bank **belum diketahui** ([?]: response code & check-status request membutuhkannya) — tanyakan DOKU; kode service pada response body hanya dipakai untuk awalan "200" sukses.
- Amount `value` tetap format `^\d{1,16}\.00$` (asumsi mengikuti QRIS; tidak ditegaskan spec).
- `CHANNEL-ID: H2H` (asumsi mengikuti QRIS) dan `X-EXTERNAL-ID` numerik unik/hari (asumsi; llms.txt spec KD menerangkan sama).
- Data pengirim (`sender*`) = data PT EraPay; `senderPersonalId/Type` untuk badan usaha **[?]** (kemungkinan NPWP perusahaan).
Belum diimplementasikan (menunggu [?]/[KD]): refund, parser settlement. Executor payout Kirim DOKU **sudah lengkap backend-nya** (14 Okt 2026): adapter DOKU (inquiry/transfer/check-status) + `gateway.Disburser` + domain `internal/payout/execute.go` + wiring `cmd/api` (env `ERAPAY_PAYOUT_*` + `SetDisburser` dari adapter doku) + endpoint admin `/v1/admin/payouts/{id}/execute` & `/check-status` (finance-only) + tes (adapter, domain konkuren, HTTP). Sisa: **UI admin** (tombol + status baru), uji sandbox asli (ASPI/DOKU), dan konfirmasi tertulis [KD].
