# Data [data] (PRD §6)

Baca juga: `../PRD.md`, `pricing.md`, `ledger.md`.

**Multi-gateway di data:** `payments` punya `gateway` + `gateway_reference` + `gateway_raw_status`; unique `(gateway, gateway_txn_id)`; `gateway_accounts` per (gateway, environment, scope); `gateway_merchant_mappings` per (merchant, gateway, payment_method) + status aktivasi + default; webhook_events, payment_attempts, settlement_batches, reconciliation_runs juga membawa `gateway`. Tambah gateway = tambah baris/adapter, bukan ubah skema inti.

**Entitas:** users, roles, permissions, user_roles, merchants, merchant_users, merchant_outlets, merchant_bank_accounts (sensitif: minimalkan, enkripsi, mask), merchant_onboarding_cases, gateway_accounts, gateway_merchant_mappings, api_keys, pricing_plans, merchant_pricing_assignments, payments, payment_attempts, payment_events, refunds, webhook_endpoints, webhook_events, webhook_deliveries, idempotency_records, ledger_accounts, ledger_transactions, ledger_entries, settlement_batches, settlement_items, payouts, payout_items, payout_approvals, merchant_balances (derived dari ledger), reconciliation_runs, reconciliation_exceptions, audit_logs, outbox_events, background_jobs (bila perlu).

**Fee di data:** `payments` menyimpan snapshot immutable `fee_bearer, gross_amount, mdr_fee, platform_fee, tax, merchant_net` (integer).

**Aturan:** `created_at`/`updated_at`/`version` bila relevan; unique: gateway txn ID (per gateway), idempotency key (per scope operasi); index: merchant_id, created_at, status, gateway, gateway_reference + kombinasi filter utama; dokumen KYC mentah dihindari (jika wajib: object storage privat, enkripsi, akses terbatas, retention); PII/rekening dienkripsi/masked.

**State machine payment (usulan):** `created→pending→{succeeded|failed|expired|cancelled}`; `pending→unknown→{pending|succeeded|failed|expired}`. Transisi final mengikuti mapping resmi gateway; `unknown` bukan final.
