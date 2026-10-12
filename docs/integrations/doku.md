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

## 6. Settlement, split, sub-account (relevan Model B)
- Laporan settlement: format, jadwal, cara akses (API/SFTP/dashboard), dan T+N **tidak ditemukan** di dokumentasi publik. **[?] [KD]** → blokir parser rekonsiliasi.
- MDR/`feeAmount`: besaran dan basis tidak disebut. **[?] [KD]**
- **Split Settlement**: hanya untuk *Aggregator Merchant*; dikonfigurasi per transaksi lewat `additional_info.settlement` (rekening `bank_account_settlement_id`, `FIX`/`PERCENTAGE`, maks 8 rekening); dihitung dari *net settlement* (setelah fee DOKU). Hanya Checkout & Direct API; **QRIS tidak disebut**. **[V]/[?]**
- **Sub-Account V2**: sub-account per pihak, dana masuk `DOKU_MERCHANT_PENDING_IDR`, split setelah fee PG, payout via inquiry → payment (`BANK_ACCOUNT` lewat BI_FAST/ONLINE). Hanya Indonesian Business Account; **kompatibilitas QRIS tidak dijelaskan di halaman**. **[V]/[?]**
- Disbursement “Kirim DOKU” (`POST /snap/v1.1/emoney/transfer-bank`, ada account inquiry, balance inquiry, check status): butuh data pengirim (`sender*`) dan penerima, `partnerReferenceNo` unik, `X-EXTERNAL-ID` unik/hari; `409` untuk duplikat; timeout/pending harus dicek status (T+1 rekonsiliasi). **[V]** Fee transfer tidak disebut. **[?]**

## 7. Implikasi untuk EraPay (bukan klaim DOKU)
1. **Adapter QRIS bisa mulai** untuk token, signature, generate, query, cancel, karena spesifikasi lengkap dan ada simulator.
2. **Webhook inbound QRIS tertahan**: payload dan sumber access-token verifikasi harus dikonfirmasi/ditangkap di sandbox. Sementara itu status final hanya dari **query** (sesuai invarian #2); polling/inquiry jadi jalur utama.
3. **Pemotongan fee platform 1% (Model B)**: split settlement/sub-account untuk QRIS belum terbukti → asumsi PRD (dana masuk ke PT, fee dipotong di ledger EraPay) tetap, tanpa bergantung fitur split.
4. **Payout**: tersedia Kirim DOKU (transfer bank); wajib dry-run default, `unknown` saat timeout/pending, inquiry sebelum retry.
5. **Pertanyaan [KD] baru**: payload notifikasi QRIS + verifikasi; kode `latestTransactionStatus` QRIS; MDR QRIS & basis; jadwal/format laporan settlement QRIS; apakah QRIS bisa dengan sub-account/split; refund QRIS per issuer; dukungan satu `merchantId` untuk banyak merchant pihak ketiga.

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
Belum diimplementasikan (menunggu [?]/[KD]): refund, parser settlement.
