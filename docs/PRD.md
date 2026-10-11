# EraPay PRD v1.4 (ringkas, untuk AI agent) — Indeks & Inti

Sumber: PRD v1.0 (11 Okt 2026), dipadatkan tanpa menghilangkan requirement, lalu dipecah per modul di `docs/prd/`. Rujuk dari `AGENTS.md`.

**Cara pakai:** selalu baca file ini (§0 Invarian, §1 Produk, §2 Peran). Untuk task, baca file ini + file modul terkait saja (tabel indeks di bawah).

**Model dana: Model B.** QRIS & akun DOKU atas nama PT. Eraya Digital Solusindo (merchant-of-record); dana pelanggan masuk ke PT, lalu dibayarkan ke merchant (payout).
**Status legal:** owner menyatakan (11 Okt 2026) masalah legal/PJP BI sudah aman. Tag **[LEGAL]** di dokumen dipertahankan sebagai checklist bukti; lampirkan dokumen pendukung sebelum go/no-go produksi (`prd/roadmap.md`). Tag **[KD]** (butuh konfirmasi DOKU) belum terkonfirmasi.
**Scope gateway saat ini: HANYA DOKU.** Desain wajib multi-gateway (Midtrans dll nanti), tapi tidak ada gateway lain yang diimplementasikan/diaktifkan sampai `prd/gateway-new.md` terpenuhi. Tanda **[KD]** → jangan diklaim "supported"; pakai feature flag + status `pending_confirmation`.

## Indeks modul
| File | Isi | Tag |
|---|---|---|
| `prd/architecture.md` | Arsitektur, adapter interface, registry, konvensi (§3) | [arch] |
| `prd/auth.md` | Auth, MFA, API key (§4.1) | [auth] |
| `prd/merchant.md` | Merchant, onboarding, rekening payout (§4.2) | [merchant] |
| `prd/payment.md` | Create payment, QRIS, status (§4.3) | [pay] |
| `prd/gateway-adapter.md` | Adapter gateway aktif (DOKU) (§4.4) | [gw] |
| `prd/webhook-inbound.md` | Webhook dari gateway (§4.5) | [whin] |
| `prd/webhook-outbound.md` | Webhook ke merchant, SSRF (§4.6) | [whout] |
| `prd/pricing.md` | Pricing & fee, MDR, Biaya Jasa (§4.7) | [price] |
| `prd/ledger.md` | Ledger double-entry, akun Model B (§4.8) | [ledger] |
| `prd/reconciliation.md` | Settlement & rekonsiliasi (§4.9) | [recon] |
| `prd/refund.md` | Refund, cancel, dispute (§4.10) | [refund] |
| `prd/ui.md` | Dashboard admin/merchant, dev portal, UX (§4.11) | [ui] |
| `prd/gateway-new.md` | Gate menambah gateway baru (§4.12) | [gw-new] |
| `prd/payout.md` | Payout & saldo merchant (§4.13) | [payout] |
| `prd/api.md` | API publik, error, idempotency (§5) | [api] |
| `prd/data.md` | Model data, state machine payment (§6) | [data] |
| `prd/security.md` | Keamanan & kepatuhan (§7) | [sec] |
| `prd/nfr.md` | Non-functional requirements (§8) | [nfr] |
| `prd/testing.md` | Strategi test (§9) | [test] |
| `prd/roadmap.md` | Roadmap, acceptance MVP, go/no-go (§10–11) | |
| `prd/agent-rules.md` | Aturan kerja agent, urutan, DoD, struktur repo (§12) | [agent] |
| `prd/open-questions.md` | Hal terbuka (§13) | |

Peta task → file yang dibaca (selain file ini):
- Payment/QRIS: payment, api, data, gateway-adapter, webhook-inbound
- Fee/ledger/saldo: pricing, ledger, data
- Payout: payout, ledger, reconciliation, security
- Dashboard: ui, auth, merchant
- Integrasi gateway: gateway-adapter, architecture, gateway-new
- Webhook ke merchant: webhook-outbound, security

## 0. Invarian (wajib, tidak bisa ditawar)
1. Uang = integer satuan terkecil + currency eksplisit. Tidak pernah float.
2. Status pembayaran final hanya dari webhook gateway tervalidasi atau inquiry resmi. Bukan dari redirect/screenshot/tutup modal/klien.
3. Timeout/ambigu → `unknown` (inquiry/rekonsiliasi), bukan `failed`. Jangan retry create-payment secara buta.
4. Idempotency: unique constraint PostgreSQL. Redis/lock bukan satu-satunya pengaman.
5. Webhook (in/out) bisa diproses >1x tanpa efek finansial ganda.
6. Ledger append-only; koreksi via reversal/adjustment. Tanpa hard delete untuk payments, ledger, webhook, audit.
7. `merchant_id` selalu dari identitas terautentikasi, bukan input klien. Tenant isolation di setiap query.
8. Fee/status/gateway ref dihitung/ditentukan server; tidak pernah diterima dari POS. Snapshot pricing tidak berubah retroaktif.
9. Payment status ≠ settlement status ≠ refund status (tiga state machine terpisah, tampil terpisah di UI).
10. Jangan mengarang endpoint/field/signature/format QR gateway mana pun. Pakai dokumentasi/SDK resmi; catat versinya di `docs/integrations/<gateway>.md`.
11. Domain, API publik, ledger, dan UI tidak boleh bergantung pada skema/istilah satu gateway. Detail gateway hanya di adapter + mapping. Saat ini hanya adapter DOKU yang aktif; gateway lain = tidak ada kode adapter/stub, tidak tampil di UI/docs sebagai supported.
12. Secret tidak boleh ada di code, fixture, log, prompt, UI (tampil sekali saja). Gateway secret dienkripsi at-rest.
13. Semua validasi keamanan & bisnis di Go; SvelteKit hanya UX. Tidak ada logic finansial/secret di browser.
14. Model B: PT. Eraya Digital Solusindo menerima dana dari DOKU lalu membayar merchant. Dana merchant = kewajiban PT (`merchant_payable` di ledger), bukan pendapatan EraPay. Dilarang: memakai dana merchant untuk operasional, mencampur dengan dana operasional, atau mengeluarkan dana tanpa ledger + approval. Bukan wallet bebas/top-up/transfer antar pengguna.
14a. Payout: idempotent, maker-checker, maksimal sebesar saldo `available`; timeout → `unknown` + inquiry, tidak pernah retry buta (payout ganda = uang hilang).
14b. Saldo merchant selalu turunan ledger (sum entri), bukan kolom yang diedit. Fee dihitung server-side; `merchant_net = gross − MDR − Biaya Jasa EraPay` dan harus identik di semua laporan.
15. Tidak ada klaim "selesai" untuk mock/stub. Tidak menyimpan data kartu.
16. Tidak pasti soal uang/status/keamanan/kontrak → tulis `OPEN QUESTION` dan hentikan bagian terkait.

## 1. Produk
**Apa:** platform aggregator multi-gateway (target: DOKU, Midtrans, dll): satu API + dashboard untuk POS/merchant; **saat ini hanya DOKU**, fase awal QRIS; dana settle gateway → PT. Eraya Digital Solusindo, lalu payout ke rekening merchant. Pendapatan: Biaya Jasa EraPay 1% dari nominal transaksi, dibebankan ke merchant (vendor), sesuai kontrak merchant tertulis. Domain tidak dikunci ke satu gateway (adapter).
**Tujuan:** API konsisten untuk POS; onboarding/manajemen merchant; QRIS via DOKU; jejak audit transaksi/fee/settlement yang bisa direkonsiliasi; dashboard merchant+admin; ketahanan thd webhook duplikat/retry/timeout/gangguan gateway; kesiapan menambah gateway tanpa mengubah domain.
**Bukan tujuan awal:** wallet/top-up/transfer antar pengguna; menahan dana merchant di luar siklus payout yang disetujui; pengganti aktivitas berizin gateway; implementasi gateway selain DOKU & semua metode sejak hari 1; split settlement/potong fee tanpa mekanisme resmi; settlement real-time; data kartu; routing otomatis antar gateway (harga/availability/failover); payout tanpa approval/limit; app mobile native; AI fraud scoring; loyalty/cicilan; microservices/K8s tanpa kebutuhan nyata.
**Asumsi:** kerja sama DOKU sudah disetujui; fase awal QRIS; merchant punya rekening bank sendiri (tujuan payout); QRIS & akun DOKU atas nama PT. Eraya Digital Solusindo, DOKU settle ke PT, PT bayar ke merchant; owner menyatakan DOKU mengizinkan akun PT menampung transaksi merchant pihak ketiga ([KD] perlu konfirmasi tertulis); izin PT [LEGAL] dinyatakan aman oleh owner (11 Okt 2026), bukti dilampirkan sebelum go/no-go; EraPay = platform/API/dashboard/monitoring/rekonsiliasi.

## 2. Peran
| Peran | Boleh | Tidak boleh |
|---|---|---|
| Super Admin | konfigurasi global, user/role, gateway, feature flag, metrik | lihat gateway secret plaintext; aksi finansial sensitif perlu audit (+approval kedua bila dikonfigurasi) |
| Ops/Support | cari merchant/transaksi, timeline, catatan | ubah nilai transaksi, hapus ledger; refund hanya dg permission khusus |
| Finance | laporan gross/fee/net, impor/cocokkan laporan gateway, tandai & catat exception, buat/setujui payout (maker-checker) | ubah transaksi sukses langsung; menyetujui payout buatannya sendiri |
| Merchant Owner | profil, user, outlet, rekening, transaksi, QRIS, laporan, API key, webhook | lihat data merchant lain |
| Cashier | buat/lihat transaksi sesuai outlet | akses secret, fee config, laporan lintas merchant, ubah status manual |
| Integrator POS | API key terbatas, docs, sandbox, webhook | akses merchant yang tidak mengizinkan |
