# Pricing & fee [price] (PRD §4.7)

Baca juga: `../PRD.md` (§0 #8, #14b), `ledger.md`, `data.md`. **Ini satu-satunya sumber kebenaran aturan fee.**

- Plan + assignment per merchant sesuai kontrak; versi, tanggal efektif, pembuat, approver (bila perlu). Tarif persen/tetap/kombinasi hanya jika diizinkan kontrak & legal. Plan memuat dimensi `gateway` & `payment_method` (tarif bisa beda per gateway).
- Server-side; snapshot per transaksi: gateway code, fee_bearer, gross_amount, gateway fee (MDR), EraPay fee, pajak (bila ada), expected net. Bedakan MDR, fee EraPay, pajak, refund, biaya operasional. Tidak retroaktif.
- **Model fee (default, sesuai owner):** per transaksi sukses: `gross_amount` (nominal transaksi, mis. Rp1.000); `mdr_fee` = 0,7% × gross (MDR QRIS, ikut kontrak DOKU [KD]); `platform_fee` = 1% × gross (Biaya Jasa EraPay); `merchant_net` = gross − mdr_fee − platform_fee. Pelanggan membayar tepat `gross_amount`; semua biaya ditanggung merchant (vendor).
- **Contoh:** gross 1.000 → MDR 7 → Biaya Jasa 10 → merchant terima 983. Kedua persen dihitung dari gross, tidak berantai (basis & pembulatan final mengikuti DOKU [KD]).
- **Pembulatan:** integer satuan terkecil; aturan per komponen dikonfigurasi dan mengikuti DOKU; wajib `gross = mdr_fee + platform_fee + merchant_net` (selisih pembulatan dimasukkan ke satu komponen yang ditetapkan, tidak boleh hilang).
- **Konfigurasi:** tarif (persen + fixed opsional) per merchant/plan/gateway/metode, berversi, snapshot per transaksi. `fee_bearer` default `merchant`; opsi `customer` (gross-up, pelanggan menanggung) di luar MVP, OFF, butuh [LEGAL]/[KD]. Angka 0,7% dan 1% adalah **default**, bukan konstanta di kode.
- **Ledger:** DOKU umumnya settle gross − MDR ke PT [KD]; kewajiban ke merchant = gross − mdr_fee − platform_fee. Bandingkan MDR laporan DOKU vs `mdr_fee` yang dibebankan; selisih = exception. Pendapatan EraPay diakui dari akun `platform_fee_revenue` saja.
- **Laporan merchant:** per transaksi dan total periode: gross, MDR QRIS, Biaya Jasa EraPay, net.
- Simulasi tarif sebelum aktivasi merchant; dokumentasikan siapa menanggung tiap komponen.
