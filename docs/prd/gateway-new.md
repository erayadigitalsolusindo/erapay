# Menambah gateway baru [gw-new] (PRD §4.12)

Belum aktif; gate wajib. Baca juga: `../PRD.md` (§0 #11), `gateway-adapter.md`, `architecture.md`.

Sebelum menulis adapter gateway X (mis. Midtrans), semua harus terpenuhi:
1. DOKU lulus pilot produksi; interface adapter stabil.
2. Due diligence + kontrak X selesai; dokumentasi resmi, sandbox, SLA/rate limit terkonfirmasi; legal/compliance review untuk peran EraPay di X.
3. `docs/integrations/X.md` berisi: capability matrix, mapping status, skema signature/webhook, model fee/MDR, jadwal & format laporan settlement, kebijakan refund/expiry.
4. Adapter lulus contract test yang sama dengan DOKU (success, pending, failure, timeout, invalid signature, duplicate & out-of-order webhook, inquiry).
5. Fee/ledger/rekonsiliasi X teruji (parser laporan settlement terpisah).
6. Feature flag per gateway & per merchant; default OFF; UI/docs menampilkan X hanya setelah flag ON dan terkonfirmasi.

Tanpa itu: jangan membuat adapter/stub gateway lain, jangan tampil di UI/docs. Menambah gateway = adapter + mapping + docs + konfigurasi, tanpa mengubah domain/ledger/API publik.
