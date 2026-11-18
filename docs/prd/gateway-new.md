# Menambah gateway baru [gw-new] (PRD §4.12)

Pengecualian (keputusan pemilik, 28 Okt 2026): adapter `ids` (IDS Payment Service, upstream yang membungkus DOKU) sudah ada di `internal/gateway/ids`. Hanya QRIS, tanpa cancel/refund/payout. Gateway untuk transaksi baru dipilih DEWA di panel (Gateway → Jalur pembayaran, tabel `payment_routing`, per environment, tercatat di audit). Tanpa pilihan, tiap merchant memakai akun gateway aktifnya sendiri (`merchant_gateway_accounts`, yang paling lama). IDS memakai satu akun untuk seluruh platform (`Capabilities.PlatformAccount`), jadi tidak butuh akun per merchant. Bila gateway terpilih tidak bisa melayani seorang merchant, `route()` memakai akun merchant itu sendiri SEBELUM memanggil gateway; tidak ada failover setelah panggilan create yang ambigu. Belum bisa menerima uang nyata sebelum paket harga `ids` dan akun ledger diputuskan. Gateway lain tetap memakai gate di bawah.

Kebijakan (keputusan pemilik, 18 Nov 2026): platform akan memakai banyak gateway seperti DOKU dan IDS, jadi adapter gateway baru BOLEH ditulis tanpa menunggu gate di bawah. Gate di bawah berubah dari syarat menulis kode menjadi syarat mengaktifkan: adapter baru wajib memenuhi poin 3, 4, dan 6 sebelum digabung (docs/integrations/X.md, contract test, feature flag per gateway dan per merchant, default OFF), dan poin 1, 2, 5 sebelum menerima uang nyata. Tidak ada gateway yang tampil sebagai supported di UI/docs sebelum flag ON dan terkonfirmasi. Domain, ledger, dan API publik tidak boleh berubah demi gateway baru. Mode sandbox boleh sejak adapter ada. Gateway pertama di bawah kebijakan ini: `finpay` (lihat `docs/integrations/finpay.md`).

Gate aktivasi wajib. Baca juga: `../PRD.md` (§0 #11), `gateway-adapter.md`, `architecture.md`.

Sebelum adapter gateway X (mis. Midtrans) diaktifkan, semua harus terpenuhi (urutan penulisan kode bebas, lihat kebijakan di atas):
1. DOKU lulus pilot produksi; interface adapter stabil.
2. Due diligence + kontrak X selesai; dokumentasi resmi, sandbox, SLA/rate limit terkonfirmasi; legal/compliance review untuk peran EraPay di X.
3. `docs/integrations/X.md` berisi: capability matrix, mapping status, skema signature/webhook, model fee/MDR, jadwal & format laporan settlement, kebijakan refund/expiry.
4. Adapter lulus contract test yang sama dengan DOKU (success, pending, failure, timeout, invalid signature, duplicate & out-of-order webhook, inquiry).
5. Fee/ledger/rekonsiliasi X teruji (parser laporan settlement terpisah).
6. Feature flag per gateway & per merchant; default OFF; UI/docs menampilkan X hanya setelah flag ON dan terkonfirmasi.

Tanpa itu: adapter tidak boleh aktif, tidak boleh tampil di UI/docs sebagai supported. Menambah gateway = adapter + mapping + docs + konfigurasi, tanpa mengubah domain/ledger/API publik.
