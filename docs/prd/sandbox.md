# Sandbox per tenant [sandbox]

Status: RENCANA (belum ada kode). Baca dulu `../PRD.md` §0 (Invarian), `ledger.md`, `payout.md`, `gateway-adapter.md`, `gateway-new.md`.

## 1. Tujuan

Tenant production bisa memilih sendiri mencoba API di sandbox lewat satu klik, tanpa deployment lain dan tanpa risiko bercampur dengan uang nyata.

## 2. Keputusan desain

**Merchant kembar.** Setiap merchant production boleh punya tepat satu merchant sandbox (`is_sandbox = true`, `parent_id` = merchant asli). Semua yang sudah dipisah per `merchant_id` (payment, ledger merchant, webhook, API key, payout) otomatis terpisah. Tidak ada filter `livemode` di tiap query.

Environment tidak lagi hanya milik proses. Turunannya: `env(merchant) = "sandbox" jika is_sandbox, selain itu "production"`. Proses dengan `ERAPAY_ENV=development|sandbox` memperlakukan semua merchant sebagai sandbox dan tidak boleh membuat kembaran.

## 3. Temuan kode sekarang (kenapa ini bukan sekadar kolom baru)

| Temuan | Lokasi | Dampak |
|---|---|---|
| Satu env per proses; adapter terdaftar hanya untuk env itu | `cmd/api/gateways.go` (`registerDOKU`, `registerFromPanel`), `cmd/api/main.go:141-152,234` | Perlu **registry per env** |
| Orchestrator, routing, gwaccount, gwcred, apikey menyimpan `env` tetap | `payment/orchestrator.go:47,122,138,322`, `routing/routing.go:50`, `gwaccount/gwaccount.go:46`, `apikey` | Env harus diambil per merchant/per request |
| `payment_routing`, `merchant_gateway_accounts`, `api_keys` sudah punya kolom `environment` | migrasi 0002, 0005, 0037 | Dipakai ulang, tidak perlu skema baru |
| Akun ledger platform global (`cash_doku_clearing`, `platform_fee_revenue`, `gateway_fee_expense`, `tax_payable`, `cash_bank_pt`, `merchant_id` NULL) | migrasi 0003, `ledger/ledger.go:169` | Sandbox **akan mencemari** pembukuan nyata jika tidak dipisah. Risiko terbesar |
| `DryRun` payout ditentukan proses (`cfg.Env != "production"`) | `cmd/api/main.go:163`, `payout/payout.go:321,379`, `payout/execute.go:43,50` | Harus per merchant dan dipaksa di service |
| Format key sudah `epk_sbx_` / `epk_prd_` | `apikey/apikey.go:3,154` | Tinggal validasi key vs merchant |

## 4. Rancangan

### 4.1 Data (migrasi `0045_sandbox_merchant.sql`)
- `merchants`: `is_sandbox boolean NOT NULL DEFAULT false`, `parent_id uuid REFERENCES merchants(id)`; `CHECK (is_sandbox = (parent_id IS NOT NULL))`; `UNIQUE (parent_id)`.
- `ledger_accounts`: `livemode boolean NOT NULL DEFAULT true`; unique index `ledger_accounts_uq` ditambah `livemode`; akun platform sandbox dibuat terpisah. Pencarian akun di `ledger.go` memakai `livemode = NOT merchants.is_sandbox`.
- Trigger/constraint: transaksi ledger tidak boleh memuat akun dengan `livemode` berbeda (satu transaksi = satu dunia).

### 4.2 Registry per env
- `gateway.Registry` dibuat satu per env (`regs["production"]`, `regs["sandbox"]`), diisi dari `gwcred.Load(code, env)`.
- Env fallback `DOKU_*` hanya untuk env proses.
- Orchestrator/routing/gwaccount menerima env dari merchant, bukan dari konstruktor.
- Webhook inbound: URL sandbox terpisah, `/v1/webhooks/sandbox/{gateway}`. Signature diverifikasi dengan kredensial env itu. Callback sandbox tidak boleh memproses payment milik merchant production, dan sebaliknya.

### 4.3 Akses
- API key: merchant sandbox hanya menerbitkan `epk_sbx_`; merchant production hanya `epk_prd_`. `Verify` menolak jika env key != env merchant (403).
- Dashboard: switch "Sandbox / Live" di header, memakai pola switch merchant/impersonation (`0024_impersonation.sql`); user yang sama ditautkan ke merchant kembar lewat `merchant_users`.
- Pembuatan kembaran: auto-approved, tanpa KYC dan tanpa rekening bank. Dicatat di audit.

### 4.4 Uang
- Payout merchant sandbox **selalu dry-run**, dipaksa di `payout.Create` dan `payout.Execute`, tidak bergantung config. Disburser nyata tidak boleh terpanggil.
- Settlement/rekonsiliasi, laporan keuangan, dan statistik platform mengabaikan merchant sandbox (`is_sandbox = false`). Tampilan panel platform menyembunyikannya secara default.
- Pricing sandbox: pakai plan default, nilai tidak berpengaruh ke pendapatan.

### 4.5 Simulator
- Adapter `fake` (QRIS) hanya terdaftar di registry sandbox. Flag default OFF, mengikuti `gateway-new.md` (flag per merchant).
- `POST /v1/sandbox/payments/{id}/simulate` dengan body `{"outcome":"paid|expired|failed"}`. Hanya key `epk_sbx_`. Memanggil jalur pemrosesan webhook yang sama dengan jalur asli, supaya posting ledger dan outbound webhook ikut teruji.
- `docs/API-INTEGRATOR.md` §8 diperbarui dengan alur ini.

## 5. Tahapan

| # | Isi | Selesai jika |
|---|---|---|
| 1 | Migrasi 0045 + `livemode` di ledger + akun platform sandbox | Test: transaksi lintas `livemode` ditolak; data lama tetap `livemode = true` |
| 2 ✔ | Registry per env dan env per merchant. Selesai: Diterapkan lewat `SetSandbox(...)` di orchestrator, gwcred, gwaccount, dan `apikey.MerchantEnv`; sandbox merchant tidak memakai pilihan jalur platform (`payment_routing`), hanya akun gateway sandbox miliknya. `routing.Service` dan webhook inbound belum berubah (tahap 4). Registry sandbox hanya diisi dari kredensial panel env `sandbox` | Test isolasi world di orchestrator lulus |
| 3 ✔ | Key mengikuti env merchant; endpoint `GET/POST /v1/sandbox` dan `/v1/sandbox/api-keys` (sesi merchant asli); kembaran tanpa tautan `merchant_users` (sesi memilih merchant pertama menurut urutan UUID, jadi kembaran tertaut bisa menjadi default tanpa sengaja); hapus merchant ikut menghapus kembaran bila tanpa jejak keuangan. Kembaran belum punya akun gateway (tahap 6: `fake`) | Test: key sandbox/production tidak lintas, 20 enable paralel menghasilkan satu kembaran |
| 4 ✔ | Webhook sandbox di `/webhooks/sandbox/{gateway}` (adapter sandbox, hanya payment merchant sandbox; lintas dunia = not found, dicatat sebagai `sandbox-{gateway}`); payout merchant sandbox selalu dry-run di `payout.Request`/`Execute` (flag per payout, bukan config), gate go/no-go tidak berlaku untuk sandbox | Test: callback silang env tidak diterapkan, 20 callback paralel satu journal, Disburser tidak pernah terpanggil |
| 5 ✔ | Settlement menolak payment sandbox (`RecordBatch` → `ErrIneligible`, plus trigger DB `settlement_items_no_sandbox`, migrasi 0046); antrean platform (unsettled, payout, refund, review merchant, picker impersonasi, statistik, metrik payout) mengabaikan sandbox; dana merchant sandbox langsung available (tidak pernah disettle) supaya payout/refund sandbox bisa dicoba | Test: batch campuran ditolak, 8 percobaan paralel tidak membuat batch, insert langsung ditolak DB, antrean dan statistik tanpa sandbox |
| 6 ✔ | Adapter `fake` (`gateway/fake`, flag `ERAPAY_SANDBOX_FAKE=1`, default OFF, hanya registry sandbox); kembaran otomatis mendapat akun + plan harga `fake` (tarif disalin dari plan DOKU); `POST /v1/sandbox/payments/{id}/simulate`; `docs/integrations/fake.md`, `API-INTEGRATOR.md` §8. Keterbatasan: server non-production (development) belum punya fake | Test: alur create → simulate paid/expired/failed → ledger sandbox, 15 simulate paralel satu journal, key produksi ditolak |
| 7 ✔ | Switch Sandbox/Live per sesi: `POST /v1/sandbox/switch` mengarahkan `sessions.merchant_id` ke kembaran (izin diambil dari keanggotaan user di merchant induk, kembaran tetap tanpa anggota, jadi login tidak berubah); `GET /v1/sandbox` melaporkan `available/enabled/active/simulator`; menu Sandbox (migrasi 0047), halaman `/dashboard/sandbox`, banner emas SANDBOX di semua halaman dashboard dengan tombol kembali ke Live; menu Payout dan Refund disembunyikan saat di Sandbox (persetujuan Finance tidak mencakup sandbox) | Test: switch, izin via induk, isolasi data per mode, sesi berakhir bila keanggotaan induk dicabut; `svelte-check` 0 error dan build web lulus (belum dicek di browser) |

Urutan 1 sampai 5 wajib sebelum tenant boleh mengaktifkan sandbox. Tahap 6 dan 7 bisa menyusul.

## 6. Uji wajib (AGENTS.md: idempotency dan concurrency)

- Idempotency: create payment sandbox dua kali dengan key sama, simulate dua kali, webhook ganda; ledger hanya satu kali posting.
- Concurrency: `POST` buat kembaran paralel hanya menghasilkan satu kembaran; simulate paralel pada payment yang sama.
- Isolasi: gateway production tidak terjangkau dari merchant sandbox; akun ledger sandbox dan production tidak pernah dalam satu transaksi; settlement dan laporan tidak memuat sandbox.
- Regresi: seluruh test merchant production lama tetap hijau tanpa perubahan.

## 7. Pertanyaan terbuka (butuh keputusan pemilik)

1. Apakah semua tenant boleh mengaktifkan sandbox sendiri, atau perlu persetujuan DEWA?
2. Apakah sandbox memakai kredensial sandbox upstream (DOKU/Midtrans) atau hanya simulator `fake`? Rekomendasi: `fake` dulu, upstream menyusul.
3. Retensi data sandbox (hapus otomatis setelah N hari?).
4. Rate limit sandbox: sama dengan production atau lebih longgar?

## 8. Estimasi

Sekitar 1 sampai 1,5 minggu kerja. Perubahan paling berat ada di tahap 2 (env per merchant) dan tahap 1 (ledger).
