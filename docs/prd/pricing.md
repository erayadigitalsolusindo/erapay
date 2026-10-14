# Pricing & fee [price] (PRD §4.7)

Baca juga: `../PRD.md` (§0 #8, #14b), `ledger.md`, `data.md`. **Ini satu-satunya sumber kebenaran aturan fee.**

- Plan + assignment per merchant sesuai kontrak; versi, tanggal efektif, pembuat, approver (bila perlu). Tarif persen/tetap/kombinasi hanya jika diizinkan kontrak & legal. Plan memuat dimensi `gateway` & `payment_method` (tarif bisa beda per gateway).
- Server-side; snapshot per transaksi: gateway code, fee_bearer, gross_amount, gateway fee (MDR), EraPay fee, pajak (bila ada), expected net. Bedakan MDR, fee EraPay, pajak, refund, biaya operasional. Tidak retroaktif.
- **Model fee (default, sesuai owner):** per transaksi sukses: `gross_amount` (nominal transaksi, mis. Rp1.000); `mdr_fee` = 0,7% × gross (MDR QRIS, ikut kontrak DOKU [KD]); `platform_fee` = 1% × gross (Biaya Jasa EraPay); `merchant_net` = gross − mdr_fee − platform_fee. Pelanggan membayar tepat `gross_amount`; semua biaya ditanggung merchant (vendor).
- **Contoh:** gross 1.000 → MDR 7 → Biaya Jasa 10 → merchant terima 983. Kedua persen dihitung dari gross, tidak berantai (basis & pembulatan final mengikuti DOKU [KD]).
- **Pembulatan:** integer satuan terkecil; aturan per komponen dikonfigurasi dan mengikuti DOKU; wajib `gross = mdr_fee + platform_fee + merchant_net` (selisih pembulatan dimasukkan ke satu komponen yang ditetapkan, tidak boleh hilang).
- **Kap MDR (opsional):** tiap komponen plan boleh punya kap (`RATE.Cap`) dengan ambang nominal (`CapUpTo`) — kap hanya berlaku saat gross ≤ ambang (0 = berlaku semua nominal); kap hanya menurunkan biaya, net menyerap selisihnya sehingga invarian `gross = mdr + platform + net` tetap. Dipakai untuk aturan BI QRIS: MDR 0,7% dibatasi **maks Rp500 untuk nominal Rp0–Rp100.000**, efektif **1 Nov 2026** ([KD] kontrak; lihat `../integrations/doku.md` §6). Diaktifkan lewat `ERAPAY_MDR_CAP` / `ERAPAY_MDR_CAP_UPTO` (default 0 = tanpa kap) → plan baru di-approve dengan kap; merchant yang sudah aktif butuh **plan v2 + assignment** baru (tarif tidak berubah retroaktif).
- **Konfigurasi:** tarif (persen + fixed opsional) per merchant/plan/gateway/metode, berversi, snapshot per transaksi. `fee_bearer` default `merchant`; opsi `customer` (gross-up, pelanggan menanggung) di luar MVP, OFF, butuh [LEGAL]/[KD]. Angka 0,7% dan 1% adalah **default**, bukan konstanta di kode.
- **Ledger:** DOKU umumnya settle gross − MDR ke PT [KD]; kewajiban ke merchant = gross − mdr_fee − platform_fee. Bandingkan MDR laporan DOKU vs `mdr_fee` yang dibebankan; selisih = exception. Pendapatan EraPay diakui dari akun `platform_fee_revenue` saja.
- **Laporan merchant:** per transaksi dan total periode: gross, MDR QRIS, Biaya Jasa EraPay, net.
- Simulasi tarif sebelum aktivasi merchant; dokumentasikan siapa menanggung tiap komponen.

## Aktivasi: kap MDR 1 Nov 2026 (runbook)

Aturan BI QRIS: MDR 0,7% dibatasi maksimum Rp500 untuk nominal Rp0–Rp100.000 — **efektif 1 Nov 2026** ([KD] kontrak final; `../integrations/doku.md` §6). Kap hanya menurunkan biaya (lihat poin di atas); tarif tidak pernah berubah retroaktif (invarian #8).

**Langkah (operator/platform):**

1. Pastikan deploy kode berisi migrasi `0020` + kap sudah jalan.
2. Set env production: `ERAPAY_MDR_CAP=500`, `ERAPAY_MDR_CAP_UPTO=100000` (default 0 = kap mati). Merchant yang **di-approve setelah ini** otomatis dapat plan dengan kap (alur `onboarding.Approve` memakai config itu).
3. Untuk merchant yang **sudah aktif sebelum env flip** (plan v1 tanpa kap), jalankan SQL di bawah **sekali** (sebaiknya tepat sebelum 1 Nov 2026 00:00 GMT+7; `effective_from` sengaja di tanggal itu agar transaksi sebelumnya tetap pakai v1). Ganti `<actor>` = UUID user platform yang menjalankan (tercatat sebagai `created_by` plan/assignment; catat juga manual di log ops karena ini jalan di luar aplikasi).
4. Verifikasi: untuk beberapa merchant, `SELECT` plan aktif baru punya `mdr_cap=500, mdr_cap_up_to=100000`; transaksi < 1 Nov tetap `pricing_plan_id` v1 (lihat `payments.pricing_plan_id` snapshot — tidak berubah).

```sql
-- Membuat plan v2 per merchant aktif: tarif DISALIN dari plan aktif (snapshot, tidak retroaktif),
-- kap ditambah. Idempoten untuk merchant yang sudah punya plan berkapis (lolos WHERE p.mdr_cap = 0).
BEGIN;
CREATE TEMP TABLE mdr_v2 AS
WITH latest AS (
  SELECT DISTINCT ON (merchant_id) merchant_id, plan_id
  FROM merchant_pricing_assignments
  WHERE effective_from <= now()
  ORDER BY merchant_id, effective_from DESC, created_at DESC
)
SELECT l.merchant_id, p.mdr_bps, p.mdr_rounding, p.platform_bps, p.platform_rounding
FROM latest l JOIN pricing_plans p ON p.id = l.plan_id
WHERE p.mdr_cap = 0;

INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding, mdr_cap, mdr_cap_up_to, created_by, approved_by)
SELECT 'qris-'||merchant_id::text, 2, 'doku', 'QRIS', mdr_bps, mdr_rounding, platform_bps, platform_rounding,
       500, 100000, '<actor>', '<actor>'
FROM mdr_v2;

INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from, created_by)
SELECT v.merchant_id, p.id, TIMESTAMPTZ '2026-11-01 00:00:00+07', '<actor>'
FROM mdr_v2 v
JOIN pricing_plans p ON p.code = 'qris-'||v.merchant_id::text AND p.version = 2
ON CONFLICT (merchant_id, plan_id, effective_from) DO NOTHING;

DROP TABLE mdr_v2;
COMMIT;
```

**Membatalkan** (bila kontrak DOKU final berbeda): kosongkan `ERAPAY_MDR_CAP` + jangan jalankan/jalankan rollback SQL assignment v2 (plan v2 tetap disimpan — append-only — tapi tanpa assignment ia tidak pernah aktif; atau assignment dengan `effective_from` di masa datang tidak di-lookup `<= now()`).

**Catatan uji:** `now()` server vs jam klien bisa berbeda beberapa ms; untuk uji lokal, backdate assignment (`now() - interval '1 second'`, lihat catatan pengujian fee).
