# Payment & QRIS [pay] (PRD §4.3)

Baca juga: `../PRD.md`, `api.md`, `data.md`, `gateway-adapter.md`, `webhook-inbound.md`.

- Create payment: nominal, currency, merchant_reference, deskripsi, metode, expiry (jika didukung). **Idempotency-Key wajib.**
- Respons: ID EraPay, status, data QR yang diizinkan, expiry, nominal, timestamp. Get status by ID berizin.
- Status: `created, pending, succeeded, failed, expired, cancelled, unknown` (mapping per gateway terdokumentasi). Simpan raw gateway status dan normalized status terpisah. Callback telat tidak boleh membalik status terminal yang bertentangan.
- Validasi: tolak nominal negatif/nol (bila tak diizinkan)/overflow/currency tak didukung/format salah; min/max dari konfigurasi + batas gateway.
- `merchant_reference` unik per scope (merchant+reference) bila kebijakan menuntut, bukan global.
- Simpan request/response gateway tersanitasi; tanpa secret/data sensitif tak perlu.
- QRIS: lewat endpoint gateway yang disetujui (saat ini DOKU); kembalikan payload/image/URL sesuai kontrak (jangan karang format); simpan & tangani expiry; UI kasir tampil nominal, order, QR, instruksi, countdown (jika expiry diketahui), polling status; tidak mencetak/menyimpan QR dinamis di luar konteks transaksi; bila QRIS/merchant belum aktif di gateway → `PAYMENT_METHOD_UNAVAILABLE`, tidak kirim transaksi produksi.
