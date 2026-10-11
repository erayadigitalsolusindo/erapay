# Settlement & rekonsiliasi [recon] (PRD §4.9)

Baca juga: `../PRD.md`, `ledger.md`, `payout.md`.

- Status settlement: `not_expected, expected, processing, settled, delayed, failed, unknown, disputed`. Simpan batch/ref, gateway, tanggal, gross, fee, net, rekening tersamarkan, sumber laporan.
- Dua lapis: (1) gateway → PT (settlement), (2) PT → merchant (payout). Dana menjadi `available` untuk payout hanya setelah settlement gateway terkonfirmasi, bukan sekadar payment success (kecuali kebijakan float disetujui [LEGAL]).
- Cocokkan transaksi EraPay ↔ data gateway ↔ settlement. Selisih nominal/hilang/duplikat/telat → `reconciliation_exceptions`; Finance beri catatan/resolusi (diaudit).
- Parser laporan settlement per gateway (format berbeda), output ke model internal yang sama.
- Impor file: validasi kolom, preview, duplicate detection, laporan error. MVP: impor manual terkontrol bila laporan otomatis belum ada. Jadwal & hari libur dari konfigurasi (bukan hardcode). Jangan klaim dana diterima hanya dari payment success; ikuti konfirmasi gateway.
