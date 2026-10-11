# Merchant [merchant] (PRD §4.2)

Baca juga: `../PRD.md`, `payout.md` (rekening payout).

- Admin: buat/review/aktifkan/suspend/nonaktifkan. Merchant isi profil legal/bisnis/kontak/alamat/kategori/data onboarding.
- Status onboarding: `draft, submitted, under_review, action_required, approved, rejected, suspended`. Status EraPay & status per gateway (mis. DOKU) disimpan **terpisah**; simpan referensi merchant di gateway + status aktivasi setelah dikonfirmasi.
- Rekening payout merchant: diverifikasi (nama rekening sesuai merchant) sebelum payout; perubahan dibatasi + diaudit + cooling period + notifikasi, dan menahan payout sampai terverifikasi.
- Satu merchant: banyak outlet, terminal, user.
