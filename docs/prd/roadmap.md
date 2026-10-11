# Roadmap, acceptance MVP, go/no-go (PRD §10–11)

Baca juga: `../PRD.md`.

## 10. Roadmap
- **P0 Discovery:** validasi kontrak/produk/API/sandbox/onboarding/QRIS/settlement/fee/refund/compliance DOKU; **legal Model B (izin PT, rekening penampung)** — owner menyatakan aman (11 Okt 2026), kumpulkan bukti; finalkan flow & harga; threat model + klasifikasi data; acceptance criteria DOKU.
- **P1 MVP internal+sandbox (DOKU saja):** auth admin/merchant; merchant/outlet/user/RBAC; gateway registry+config + adapter DOKU; create QRIS sandbox; status + webhook inbound; idempotency; timeline; audit + structured logs; dashboard transaksi dasar; rekonsiliasi impor/manual; ledger Model B + saldo merchant + perhitungan fee (MDR + Biaya Jasa); payout dry-run/sandbox; test suite + docs integrator.
- **P2 Pilot produksi:** onboarding sesuai proses DOKU; QRIS produksi; payout produksi (batch/manual dulu, setelah gate LEGAL); dashboard fee/saldo/settlement/payout; alerting, uji backup/restore, incident runbook; support tooling; pilot merchant terbatas.
- **P3 Pertumbuhan:** pricing per merchant/volume/gateway (bila kontrak boleh); laporan periodik/ekspor; dashboard webhook delivery; rotasi API key + scoped permission; otomasi rekonsiliasi; **gateway kedua (kandidat: Midtrans) lewat `gateway-new.md`**.

## 11. Acceptance MVP (siap pilot sandbox)
1 Admin kelola merchant/role + audit.
2 API key sandbox terikat scope benar.
3 POS buat QRIS via DOKU sandbox.
4 Idempotency key sama tak membuat pembayaran kedua.
5 Status berubah hanya dari sumber tervalidasi + transisi yang diizinkan.
6 Webhook duplikat tak menggandakan event finansial.
7 Merchant terima webhook EraPay bertanda tangan + retry.
8 Payment & settlement status tampil terpisah.
9 Fee server-side, snapshot historis tetap.
10 Ledger/rekonsiliasi traceable ke gateway reference.
11 Secret & PII tak perlu tersembunyi dari log.
12 Tes tenant isolation lulus.
13 Docs API, contoh, error codes tersedia.
14 Monitoring, alert, backup, restore, runbook teruji.
15 Semua fitur butuh persetujuan DOKU terverifikasi sebelum produksi.
16 Domain/ledger/API tidak mengimpor paket DOKU langsung (hanya lewat registry/interface).
17 Fee: `gross == mdr_fee + platform_fee + merchant_net` (uji pembulatan; contoh 1.000 → 7 + 10 + 983), snapshot tidak berubah, laporan menampilkan keempat angka.
18 Saldo = hasil ledger; payout > `available` ditolak; payout idempotent; maker≠checker.
19 Rekonsiliasi dua lapis (DOKU→PT, PT→merchant) menghasilkan laporan exception.

## Go/no-go produksi
Kontrak DOKU izinkan alur & akun PT menampung transaksi merchant pihak ketiga (tertulis); izin/legal PT untuk menerima & menyalurkan dana terkonfirmasi (owner: aman — lampirkan bukti); rekening penampung dana merchant terpisah; payout + rekonsiliasi dua lapis teruji dengan dana nyata kecil; MDR 0,7% & basis perhitungan fee terkonfirmasi di kontrak DOKU; fee disclosure ke merchant disetujui; QRIS+onboarding prod aktif; settlement & fee collection teruji dg laporan resmi; legal/compliance selesai; pentest/security review selesai; rekonsiliasi+exception siap; backup-restore & incident drill lulus; kontak eskalasi + prosedur refund/dispute jelas; merchant terms, privacy notice, fee disclosure disetujui; pilot terbatas disetujui owner & ops.
