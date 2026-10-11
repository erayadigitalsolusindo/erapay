# Tema & referensi visual UI [ui-theme]

Baca juga: `ui.md` (fungsional/UX), `../PRD.md`.

Referensi visual (prototipe HTML + Tailwind, **hanya acuan tampilan**, bukan kode produksi) ada di `docs/refrence/`:

| File | Isi |
|---|---|
| `erapay-ui-kit.html` | Sumber utama token & komponen: form, status input, badge, card, modal (sukses/getar), toast, profil member. Tailwind v4 (`@theme`). |
| `erapay-dashboard.html` | Layout dasbor admin/merchant: sidebar + halaman Overview, Transaksi, Payment Link, Provider, Merchant, Settlement, Rekonsiliasi, Laporan, API & Webhook, Risiko. Tailwind v3 CDN. |
| `erapay-masuk-daftar.html` | Halaman Masuk/Daftar: panel hero gradien merah + pola, kartu form dengan tab masuk/daftar. |

Seluruh tampilan produksi (`apps/web`) **harus mengikuti tema ini**. Jika ada beda antar-file, `erapay-ui-kit.html` yang menang.

## Token
- Warna: `merah #c8102e` (aksi utama), `merah-tua #8a0a1f` (hover/aksen; di dashboard bernama `marun`), `merah-gelap #4a0510` (teks, sidebar; di dashboard bernama `tua`), `gading #fff8f0` (background halaman), `emas #e8b84a` (fokus, aksen, badge sandbox). Status: hijau (berhasil), amber (menunggu), merah `red-*` (gagal), `merah-gelap/10` (kedaluwarsa/netral).
- Font: **Plus Jakarta Sans** (UI, 400–800), **Fraunces** (heading/display, 600/800). Heading `font-display font-extrabold`.
- Bentuk: card `rounded-2xl` border `merah-gelap/10` + `shadow-sm`, putih di atas gading; input/tombol `rounded-xl`; badge `rounded-full`.
- Fokus: ring `emas/60` untuk tombol, `merah/15` untuk input (`focus:border-merah`). Jangan hapus indikator fokus.
- Logo: lingkaran emas bertuliskan "E" (Fraunces) + wordmark `ERAPAY` tracking lebar.
- Pola latar: ornamen elips putih ~16% opasitas (kelas `.pola`) di panel merah/hero.

## Komponen (nama kelas di kit)
`.card`, `.lbl`, `.inp` (+ `.inp-err`), `.btn`, `.bdg`, `.hint`, `.sec`, `.nav` (`aria-current=page`). Di SvelteKit jadikan komponen/utility terpisah dengan nama yang sama; token masuk ke `@theme` Tailwind v4 (bukan CDN).

## Layout
- Shell: sidebar `merah-gelap` lebar ±16rem (sticky di `lg`, jadi bar horizontal scroll di mobile) + konten `p-5 lg:p-8` di atas `gading`.
- Sidebar menandai env: kotak "Lingkungan **Sandbox**" bila sandbox.
- Dasbor: header (judul + filter periode + CTA), kartu statistik 1/2/4 kolom, bar chart volume + porsi per provider, tabel transaksi terbaru, kartu rincian potongan (nilai transaksi, MDR, Jasa EraPay, diterima merchant).
- Tabel: `overflow-x-auto`, header `gading` kecil, `divide-y`.
- Auth: dua kolom di desktop (hero merah kiri, form kanan), satu kolom di mobile.

## Gerak
Animasi masuk (`naik`), modal pegas, ring/bar terisi saat terlihat, toast. Wajib hormati `prefers-reduced-motion` (sudah ada di kit).

## Aturan penerapan
1. Ikuti [UX di `ui.md`](ui.md): status **selalu berlabel teks**, bukan warna saja; bedakan "Pembayaran berhasil" vs "Settlement selesai".
2. Bahasa Indonesia default (copy di referensi sudah ID); string lewat struktur i18n.
3. Data pada referensi adalah **dummy**. Jangan salin nama provider/angka: opsi gateway di UI hanya yang aktif & terkonfirmasi (saat ini **hanya DOKU**; Midtrans/Xendit di mockup tidak boleh tampil).
4. Jangan salin pola mockup yang melanggar keamanan (mis. API key/secret di HTML). Secret tampil sekali; rekening di-mask.
5. Format uang `Rp1.000.000` (titik ribuan); nilai tetap integer di backend.
6. Jangan load Tailwind via CDN/`<script>` di produksi.
