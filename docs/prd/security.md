# Keamanan & kepatuhan [sec] (PRD §7)

Baca juga: `../PRD.md`, `payout.md`, `webhook-outbound.md`.

- **Kontrol:** TLS semua trafik; **uang keluar (payout):** maker-checker, MFA saat approve, allowlist rekening + cooling period, limit harian + alert anomali, pemisahan tugas; secrets management & pisah kredensial sandbox/prod (per gateway); enkripsi transit+at-rest; RBAC least privilege; MFA admin; rate limit per API key/IP/endpoint; signature+replay protection webhook; audit log (login, API key, perubahan merchant/rekening/pricing, refund, akses data sensitif); redaksi secret/token/PII di log; dependency scan, static analysis, code review; backup terenkripsi + uji restore; runbook incident & rotasi kredensial; proteksi SSRF; CORS/CSRF/cookie session benar; jangan kirim data/secret produksi ke layanan AI eksternal tanpa kebijakan data.
- **Compliance gate (sebelum produksi, review legal; diulang untuk tiap gateway baru):** peran & izin EraPay; tanggung jawab KYC/KYB, AML, fraud, complaint handling, privasi (UU PDP Indonesia), retention; kontrak merchant (fee, settlement, refund, dispute, downtime, batas tanggung jawab); izin/kewajiban PT sebagai penerima & penyalur dana pihak ketiga (PJP/agregator; di bawah izin DOKU atau izin sendiri) [LEGAL — dinyatakan aman oleh owner 11 Okt 2026, lampirkan bukti]; pemisahan dana merchant dari operasional; kewajiban pengungkapan fee ke merchant (kontrak merchant) [LEGAL]; AML/CFT, pelaporan, retensi; security assessment.
- Dokumen ini bukan pendapat hukum.

## Kredensial gateway lewat panel DEWA [security-gw-credentials]
- Menggantikan edit `.env` di server untuk rotasi kredensial gateway (saat ini DOKU). Panel: `/platform/gateways`; API: `GET /v1/platform/gateway-credentials`, `PUT|POST .../{gateway}/{env}[/test|/apply]` (hanya DEWA; mati bila `ERAPAY_ENCRYPTION_KEY` kosong).
- Tabel `gateway_credentials`: satu baris per (gateway, environment), isi JSON disegel AES-256-GCM (`secretbox`), AAD = `gwcred/<gateway>/<env>`. Master key tetap hanya di env server.
- **Bukan editor env bebas**: hanya field yang dideklarasikan skema gateway (`internal/gwcred/schema.go`) yang diterima; `base_url` dibatasi allowlist (cegah kredensial terkirim ke host lain) dan harus cocok dengan environment. Gateway baru = tambah skema + factory di `cmd/api/gateways.go`.
- **Write-only**: setelah disimpan hanya pratinjau ter-mask (`••••abcd`; private key tanpa pratinjau). Field kosong saat simpan = pertahankan nilai lama.
- **Optimistic locking** (`version`): dua admin tidak saling menimpa; request ulang dengan versi lama ditolak (409).
- **Simpan ≠ terapkan**: `Terapkan` membangun adapter kandidat, menjalankan tes koneksi, dan baru menukar adapter di registry bila lulus; gagal = adapter lama tetap jalan. Payout mengikuti adapter terdaftar (`registryDisburser`).
- Prioritas saat start: kredensial DB lengkap untuk environment server → env `DOKU_*` (fallback; juga bila DB tak terbaca/ditolak).
- Audit: `gateway_credentials.set|tested|applied` mencatat nama field yang berubah, versi, alasan; tidak pernah nilainya.
- Batas saat ini: satu instans API (penukaran adapter hanya di proses yang menerima request; multi-instans butuh restart atau pub/sub), belum ada rollback otomatis ke versi lama, payout pertama kali aktif dari panel butuh restart bila DOKU tidak terdaftar saat start.

## Berkas KYC (dokumen, bukti, logo) [security-kyc-files]
- Disimpan sebagai file di `ERAPAY_KYC_STORAGE_DIR` (bukan BLOB); tabel hanya menyimpan metadata (`storage_key`, nama, MIME, ukuran).
- **Terenkripsi at rest**: AES-256-GCM (`secretbox`, kunci `ERAPAY_ENCRYPTION_KEY`), AAD = `kyc/<merchant_id>` sehingga berkas yang disalin ke merchant lain tidak bisa dibuka. Kolom `enc_v` (0 = berkas lama polos, 1 = terenkripsi) dan `sha256` (hash isi asli, diverifikasi saat dibuka; tidak cocok = `ErrFileCorrupt`).
- **Log akses**: setiap pembukaan dokumen/bukti (dan logo oleh reviewer) menulis `kyc.file.accessed` ke audit_logs sebelum berkas dikirim; gagal mencatat = tidak dikirim. Isi berkas tidak pernah masuk log.
- **Migrasi berkas lama** (sekali, idempoten): `go run ./cmd/kycencrypt -dry-run` (hitung) lalu `go run ./cmd/kycencrypt`. Backup folder KYC + database lebih dulu. Langkah di server (user `www`, bukan root): `docs/DEPLOY-AAPANEL.md` bagian 6a.
- **Kunci**: simpan backup `ERAPAY_ENCRYPTION_KEY` terpisah dari backup berkas; merotasi kunci butuh enkripsi ulang (belum otomatis).
- Retensi: kebijakan hapus/arsip setelah masa wajib simpan belum diimplementasikan (lihat `open-questions.md`).

