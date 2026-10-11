# Gateway adapter (aktif: DOKU) [gw] (PRD §4.4)

Baca juga: `../PRD.md`, `architecture.md`, `gateway-new.md`. Detail DOKU di `docs/integrations/DOKU.md`.

Poin berlaku untuk semua adapter.
- Isolasi skema internal dari format gateway; kredensial per environment + scope merchant/platform sesuai arahan gateway; signature/auth persis sesuai dokumentasi.
- Verifikasi signature webhook, timestamp/nonce (jika ada), replay protection.
- Timeout koneksi/response sesuai SLA; status inquiry untuk transaksi ambigu (jika endpoint ada); error gateway → error EraPay stabil (detail sensitif disembunyikan); catat correlation ID + gateway reference; perubahan versi API gateway dicatat & diuji sebelum deploy.
- Status gateway → status normalized lewat tabel mapping per gateway (satu paket per adapter); raw status disimpan apa adanya.
- Uji sandbox: success, pending, failure, timeout, invalid signature, duplicate callback, out-of-order callback.
