# Gateway `fake` (hanya sandbox)

Kode: `services/api/internal/gateway/fake`. Ini bukan gateway sungguhan: tidak ada upstream, kredensial, atau uang. Dipakai
untuk sandbox per tenant (`../prd/sandbox.md`) supaya integrator bisa menjalankan alur lengkap tanpa memindai QR.

## Gate aktivasi (`../prd/gateway-new.md`)
- Flag: `ERAPAY_SANDBOX_FAKE=1`, default OFF. Hanya berlaku di server `ERAPAY_ENV=production`. Adapter ada **hanya** di registry sandbox, tidak pernah di registry produksi.
- Per merchant: kembaran sandbox mendapat akun `fake` (environment `sandbox`, ref `FAKE`) dan plan harga `fake-qris-<id kembaran>` saat sandbox diaktifkan atau diaktifkan lagi. Tarifnya disalin dari plan DOKU merchant induk, jadi biaya di sandbox terlihat nyata.
- Contract test: `fake_test.go` (create, inquiry tidak pernah menggerakkan status, pemetaan status, outcome tak dikenal). Alur penuh: `internal/httpapi/sandbox_simulate_integration_test.go`.

## Capability
QRIS, inquiry, cancel. Tidak ada refund gateway, settlement report, atau disbursement (payout sandbox selalu dry-run, `../prd/payout.md`).

## Pemetaan status
| Raw | Status |
|---|---|
| `PENDING` | pending |
| `PAID` | succeeded |
| `EXPIRED` | expired |
| `FAILED` | failed |

Inquiry (`GetPayment`) selalu menjawab `PENDING`: hasil hanya datang dari simulator, tidak pernah dari polling.

## Simulator
`POST /v1/sandbox/payments/{id}/simulate` dengan body `{"outcome":"paid|expired|failed"}`.
- Hanya key `epk_sbx_` dengan scope `payment:create`; key produksi dijawab 403.
- Hanya payment milik merchant pemilik key yang memakai gateway yang mendukung simulasi (`gateway.Simulator`); selain itu 404 atau 409.
- Hasil diterapkan lewat jalur yang sama dengan webhook terverifikasi (`ApplyWebhookSandbox`): ledger dunia sandbox, webhook keluar, idempotency (dedup per payment + outcome).
- Status final tidak berbalik: simulasi berikutnya yang bertentangan dijawab 200 dengan `"applied": false`.
