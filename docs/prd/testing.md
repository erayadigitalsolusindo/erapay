# Testing [test] (PRD §9)

Baca juga: `../PRD.md`, `agent-rules.md`.

- **Unit:** pricing+pembulatan, state transition, idempotency, signature, fee snapshot, ledger balance, authz tenant, matching rekonsiliasi, jadwal retry webhook, registry/capability gateway.
- **Integration:** Go API+PG, Redis fail/reconnect, outbox publish/retry, adapter DOKU sandbox, signature & duplikat webhook, rotasi/revoke API key, refund (hanya jika didukung). Siapkan **contract test adapter generik** (dipakai ulang untuk gateway baru).
- **E2E:** onboarding→approved; create QRIS; sukses+webhook; pending→sukses; timeout create+inquiry; webhook duplikat/out-of-order; QR expired; merchant A tak bisa akses B; rekonsiliasi temukan hilang/selisih; backup restore + recovery worker.
- **Security:** IDOR/tenant escape, brute force/rate limit, replay webhook, SSRF, secret bocor di log/UI, SQLi/XSS/CSRF, privilege escalation, API key revoked ditolak, otorisasi ekspor sensitif.
- **Payout/saldo/fee:** double-submit payout, payout paralel vs saldo, timeout→unknown→inquiry, maker≠checker, ganti rekening menahan payout, saldo = sum(ledger), property test fee (`gross == mdr + platform + net`, pembulatan, tanpa selisih hilang), refund mengurangi saldo.
- Perubahan finansial wajib uji idempotency + concurrency.
