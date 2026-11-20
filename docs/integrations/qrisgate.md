# QRISGate [gw] (adapter `qrisgate`, buatan sendiri)

**QRISGate di EraPay adalah gateway buatan sendiri, bukan klien layanan pihak ketiga.** Konsepnya meniru layanan "QRIS berbasis deteksi mutasi": QRIS statis milik tenant diubah menjadi QRIS dinamis dengan nominal unik, dan pembayaran dikenali dari notifikasi aplikasi bank/e-wallet di HP tenant yang diteruskan aplikasi Listener (`apps/listener-android`) ke webhook bertanda tangan. Dana masuk **langsung** ke rekening/e-wallet tenant; EraPay tidak memegang dana. Gateway ini berkapabilitas `Direct`.

## Alur
1. **Penyiapan tenant** (menu Integrasi → tab QRISGate; butuh `merchant:write` dan `ERAPAY_ENCRYPTION_KEY`):
   1. QRIS statis: tempel teks atau unggah gambar (dibaca di browser dengan jsQR; gambar tidak dikirim). Server memvalidasi: TLV utuh, CRC16, format 01, inisiasi statis `11`, mata uang `360`, negara `ID`, tanpa tag nominal (54).
   2. Daftar nama paket aplikasi yang notifikasinya diterima (1-20, mis. `id.dana`). Notifikasi dari paket lain diabaikan.
   3. Rahasia listener (`qls_…`, 32 byte acak): tampil **sekali**, disimpan terenkripsi AES-GCM; membuat yang baru menggugurkan yang lama.
   Tiga langkah ini membuat akun gateway `qrisgate` berstatus `pending_confirmation` (`gateway_ref` = merchant id) = **flag per merchant, bawaan OFF**. DEWA mengaktifkannya di panel (Tenant → Gateway).
2. **Membuat pembayaran**: orchestrator memanggil adapter. Adapter memesan **nominal bayar** = harga + kode unik 1-99 (terkecil yang bebas) di `qrisgate_claims` (unik per tenant selama aktif) dan mengembalikan QRIS dinamis (tag 01 → `12`, tag 54 = nominal bayar, CRC dihitung ulang). Payment EraPay menyimpan **harga dasar**; pembeli membayar nominal bayar. Pemesanan idempoten per payment; klaim kedaluwarsa dilepas saat pemesanan berikutnya.
3. **Mengenali pembayaran**: Listener mengirim `POST /webhooks/qrisgate/<merchant_id>` dengan header `X-Listener-Timestamp`, `X-Listener-Signature` (`sha256=` + hex HMAC-SHA256 atas `timestamp.body` dengan rahasia listener), `X-Listener-Delivery-Id`. Server: memverifikasi tanda tangan (selisih waktu maks 300 dtk, banding constant-time), menyimpan notifikasi di `qrisgate_events` (idempoten per `(merchant, delivery id)`), lalu mencocokkan.
4. **Pelunasan otomatis** hanya bila semua benar: paket ada di daftar, nominal terbaca (server membaca ulang "Rp …" dari teks, mengalahkan angka dari perangkat), teks tidak berciri **keluar** ("mengirim", "transfer ke", "pembayaran ke", "tarik tunai", "pembelian", "debit", "top up", …), nominal **sama persis** dengan nominal bayar satu klaim aktif, dan waktu notifikasi dalam jendela klaim (dari 2 menit sebelum QR dibuat sampai 5 menit setelah kedaluwarsa). Lalu `payments` → `succeeded` lewat jalur webhook biasa (idempoten, dedup `qrisgate:listener:<delivery id>`). Yang tidak cocok/diabaikan tidak mengubah payment dan tampil di daftar "Notifikasi yang diterima" untuk ditangani tenant.
5. **Kedaluwarsa**: tidak ada pihak luar yang memberi tahu, jadi inquiry (poller) melapor `EXPIRED` saat klaim lewat waktu; payment ditutup `expired`. Inquiry **tidak pernah** melaporkan sukses.

## Model keamanan dan batasnya (baca sebelum dipakai uang sungguhan)
- Notifikasi HP **bukan** sumber setara webhook gateway resmi. Ini keputusan pemilik: pelunasan otomatis berdasarkan notifikasi bertanda tangan dengan nominal unik. Perlindungan: HMAC per tenant, dedup per delivery id, nominal unik per tenant, jendela waktu, daftar paket, saringan "keluar", dan satu notifikasi hanya bisa melunasi satu payment.
- **Risiko yang tersisa**: (a) aplikasi lain di HP yang bisa menampilkan notifikasi palsu dengan nama paket yang sama; (b) transfer masuk tak terkait dengan nominal yang kebetulan sama persis dengan klaim aktif; (c) notifikasi tidak terbaca (HP mati/hemat baterai) → payment kedaluwarsa walau sudah dibayar (tampil sebagai "tidak ada" sampai tenant mengecek mutasi); (d) teks bank yang tidak berformat "Rp …".
- Notifikasi yang masuk setelah payment kedaluwarsa menjadi "tidak cocok"; **belum ada konfirmasi manual** di aplikasi (tenant menyelesaikannya di luar sistem). Menambahkan konfirmasi manual butuh sumber status baru (invarian #2) dan keputusan pemilik.
- Teks notifikasi dapat memuat nama pengirim: disimpan di `qrisgate_events` dan hanya tampil ke tenant pemilik. **Belum ada penghapusan otomatis**; tentukan masa simpan sebelum produksi besar.

## Fee, ledger, saldo
- Plan platform tetap `qrisgate-direct` (MDR 0, persen 0): `merchant_net = gross`. Tenant menerima nominal bayar (harga + kode unik); selisih kode unik (maks Rp99) ada di tenant, bukan EraPay.
- Tidak ada jurnal penjualan (uang tidak lewat PT). Hanya **potongan per transaksi** (setelan platform `qrisgate_fee`, bawaan 0) dari saldo kredit QRISGate tenant: `Dr qrisgate_credit / Cr platform_fee_revenue` (dedup per payment, saldo boleh minus). Lihat `../prd/pricing.md`.
- Payment langsung (`payments.direct`) dikecualikan dari daftar belum-settle, batch settlement, saldo `pending`, dan refund. Top-up saldo tidak pernah dirutekan ke gateway langsung.

## Tombol Tes webhook
Menandatangani notifikasi `listener.test` dengan rahasia tersimpan dan memverifikasinya di server (tanpa jaringan, tanpa HP, rahasia tidak ditampilkan). Membuktikan rahasia tersimpan dan skema tanda tangan berjalan; tidak membuktikan HP dapat menjangkau URL webhook. Tes sebenarnya: tombol "Kirim tes ke webhook" di aplikasi Listener.

## Diagnosa penolakan webhook
Penolakan tanda tangan dicatat di log API (`webhook signature rejected`) dengan `reason` (`missing_signature_header`, `missing_or_bad_timestamp_header`, `timestamp_skew:NNNs`, `no_listener_secret_for_merchant`, `signature_mismatch`, `no_merchant_id_in_path`), metode, dan **nama** header yang diterima (tanpa nilai). Pemanggil hanya melihat `WEBHOOK_REJECTED`.

## Uji
`internal/qris` (CRC, validasi, QRIS dinamis), `internal/gateway/qrisgate` (kontrak adapter + uji ujung ke ujung dengan PostgreSQL: pelunasan sekali walau 6 pengiriman bersamaan, replay tidak melunasi payment berikutnya, 8 payment bersamaan mendapat kode berbeda, jendela waktu, kedaluwarsa, tanda tangan salah), `internal/qrisgate` (penyiapan, enkripsi). **Belum diuji dengan HP dan aplikasi bank/e-wallet sungguhan**: bentuk teks notifikasi tiap aplikasi, izin akses notifikasi, dan ketahanan terhadap penghemat baterai harus dicoba dengan transaksi kecil.

## APK listener
Aplikasi di `apps/listener-android`; DEWA mengunggahnya di panel Gateway dan tenant mengunduhnya di menu QRISGate (lihat `ERAPAY_APK_STORAGE_DIR`, `BODY_SIZE_LIMIT`). Isi di aplikasi: URL webhook = `<API publik>/webhooks/qrisgate/<merchant_id>`, rahasia = `qls_…` dari langkah 3, nama paket sama dengan langkah 2.
