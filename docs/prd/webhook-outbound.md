# Webhook outbound [whout] (PRD §4.6)

Baca juga: `../PRD.md`, `security.md`.

Endpoint HTTPS merchant tervalidasi; event: ID unik, nama, waktu, data minimum (tanpa secret/PII tak perlu); payload bertanda tangan EraPay (terdokumentasi); retry exponential backoff + jitter + dead-letter; simpan attempt count, response code, latency, next retry; merchant lihat history & redeliver (izin); at-least-once → konsumen idempotent.

**SSRF:** blok loopback, link-local, private IP, metadata endpoint, redirect berbahaya; validasi DNS/IP saat registrasi **dan** saat kirim.
