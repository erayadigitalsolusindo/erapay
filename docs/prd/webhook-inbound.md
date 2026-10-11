# Webhook inbound (per gateway) [whin] (PRD §4.5)

Baca juga: `../PRD.md`, `gateway-adapter.md`, `payment.md`.

Route `POST /webhooks/{gateway}` dengan verifier sendiri; HTTPS; verifikasi autentisitas sebelum proses; simpan event dengan dedup key/fingerprint; balas cepat sesuai kontrak gateway, proses berat async setelah event tersimpan durable; duplikat → tanpa ledger/notifikasi ganda; event tak terpetakan → quarantine/dead-letter; payload tersanitasi + akses terbatas; validasi jumlah/currency/merchant reference/gateway txn ID vs data lokal; tidak boleh mengubah transaksi merchant lain; sediakan job inquiry/rekonsiliasi untuk webhook hilang.
