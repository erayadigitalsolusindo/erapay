# Auth [auth] (PRD §4.1)

Baca juga: `../PRD.md`, `security.md`.

- Login/logout admin & merchant; password hashing modern; MFA admin wajib, merchant owner direncanakan; RBAC granular; session/token bisa dicabut + expiry jelas; rate limit + audit di login & aksi sensitif; sandbox & production terpisah.
- API key: prefix publik, secret tampil sekali, disimpan hash, revoke/rotate, scoped.
