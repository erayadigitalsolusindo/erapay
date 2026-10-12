# Webhook outbound [whout] (PRD §4.6)

Baca juga: `../PRD.md`, `security.md`.

Endpoint HTTPS merchant tervalidasi; event: ID unik, nama, waktu, data minimum (tanpa secret/PII tak perlu); payload bertanda tangan EraPay (terdokumentasi); retry exponential backoff + jitter + dead-letter; simpan attempt count, response code, latency, next retry; merchant lihat history & redeliver (izin); at-least-once → konsumen idempotent.

**SSRF:** blok loopback, link-local, private IP, metadata endpoint, redirect berbahaya; validasi DNS/IP saat registrasi **dan** saat kirim.

## Implementasi (v1)
- **Outbox transaksional:** `payment.Transition` menulis `webhook_events` + `webhook_deliveries` di transaksi yang sama dengan perubahan status final. Satu event per `(type, resource_id)`, jadi notifikasi gateway ganda atau status konflik tak menghasilkan notifikasi kedua. Event: `payment.succeeded|failed|expired|cancelled`.
- **Envelope:** `{"id": "<event id>", "type": "...", "created_at": "...", "data": {"payment": {...}}}`. `id` stabil untuk dedup di sisi merchant (pengiriman at-least-once). Header: `X-EraPay-Event-Id`, `X-EraPay-Delivery-Attempt`.
- **Signature:** `X-EraPay-Signature: t=<unix>,v1=<hex(HMAC-SHA256(secret, "<t>." + raw_body))>`. Penerima wajib hitung ulang atas raw body, bandingkan constant-time, dan tolak `t` di luar toleransi (disarankan 5 menit). Secret `whsec_...` tampil sekali saat create/rotate; disimpan terenkripsi AES-256-GCM (kunci `ERAPAY_ENCRYPTION_KEY`). Tanpa kunci, fitur ini nonaktif.
- **Retry:** sukses = HTTP 2xx. Gagal → backoff 30s·2^(n-1) (maks 6 jam, jitter ±20%), 8 percobaan lalu `dead`. Redirect tidak diikuti (dihitung gagal). Instance ganda aman (`FOR UPDATE SKIP LOCKED` + lease 2 menit).
- **SSRF:** wajib `https`, tanpa kredensial; semua IP hasil DNS harus publik; divalidasi saat registrasi, sebelum kirim, dan di dialer (alamat yang benar-benar dihubungi), tanpa proxy.
- **API (session merchant, izin `webhook:manage`):** `POST|GET /v1/webhook-endpoints`, `POST /v1/webhook-endpoints/{id}/rotate-secret`, `POST /v1/webhook-endpoints/{id}/disable`, `GET /v1/webhook-deliveries?endpoint_id=&status=&limit=&cursor=`, `POST /v1/webhook-deliveries/{id}/redeliver`. Maks 5 endpoint aktif per merchant.
- **Belum:** halaman dashboard untuk endpoint/riwayat, pilihan jenis event per endpoint, grace period rotasi secret.
