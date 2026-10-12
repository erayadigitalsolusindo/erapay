// Package payout moves merchant funds out of EraPay (PRD payout.md, §0 #14, #14a).
//
// Money rules enforced here:
//   - a request reserves the funds in the ledger inside the same transaction, after taking a
//     per-merchant lock, so concurrent requests can never exceed `available`;
//   - the requester can never approve (maker != checker, also a DB CHECK);
//   - only a verified bank account past its cooling period can receive a payout;
//   - there is no automatic transfer: Finance performs it and records the bank reference
//     (MVP: manual transfer + confirmation). Outside production a payout is a dry run, and in
//     production payouts stay disabled until ERAPAY_PAYOUT_LIVE=1 (go/no-go, roadmap.md).
package payout

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/ledger"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/outbound"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/settlement"
)

var (
	ErrValidation      = errors.New("payout: validation failed")
	ErrNotFound        = errors.New("payout: not found")
	ErrDisabled        = errors.New("payout: payouts are disabled in this environment")
	ErrMerchantState   = errors.New("payout: merchant is not active")
	ErrAccountUnusable = errors.New("payout: bank account is not verified, still cooling down, or not the merchant's")
	ErrLimit           = errors.New("payout: amount outside the allowed limits")
	ErrInsufficient    = errors.New("payout: amount exceeds the available balance")
	ErrState           = errors.New("payout: not allowed in the current status")
	ErrSelfApproval    = errors.New("payout: the requester cannot approve their own payout")
	ErrRefConflict     = errors.New("payout: already paid with a different bank reference")
	ErrAccountInUse    = errors.New("payout: bank account has open payouts")
)

// Config values marked OPEN in PRD payout.md are policy, not code: set them from the environment.
type Config struct {
	DryRun      bool          // non-production: no real transfer is expected
	LiveEnabled bool          // production gate (go/no-go); ignored when DryRun
	MinAmount   int64         // OPEN: minimum payout
	MaxAmount   int64         // OPEN: maximum per payout
	DailyLimit  int64         // OPEN: rolling 24h total of non-failed payouts per merchant
	Cooling     time.Duration // OPEN: wait after verification before a new account can be paid
}

func DefaultConfig(dryRun bool) Config {
	return Config{DryRun: dryRun, MinAmount: 10_000, MaxAmount: 50_000_000, DailyLimit: 200_000_000, Cooling: 24 * time.Hour}
}

type Service struct {
	db   *pgxpool.Pool
	box  *secretbox.Box
	cfg  Config
	idem *idempotency.Store
	now  func() time.Time
}

func NewService(db *pgxpool.Pool, box *secretbox.Box, cfg Config) *Service {
	return &Service{db: db, box: box, cfg: cfg, idem: idempotency.NewStore(db), now: time.Now}
}

type Actor struct{ UserID, RequestID string }

var (
	uuidRe   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	numberRe = regexp.MustCompile(`^[0-9]{5,32}$`)
	bankRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{2,20}$`)
)

// ---- bank accounts ----

type BankAccount struct {
	ID            string     `json:"id"`
	MerchantID    string     `json:"merchant_id,omitempty"`
	BankCode      string     `json:"bank_code"`
	AccountMasked string     `json:"account_masked"`
	AccountName   string     `json:"account_name"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	VerifiedAt    *time.Time `json:"verified_at"`
}

const bankCols = `id::text, merchant_id::text, bank_code, account_last4, account_name, status, created_at, verified_at`

func scanBank(row pgx.Row) (BankAccount, error) {
	var b BankAccount
	var last4 string
	err := row.Scan(&b.ID, &b.MerchantID, &b.BankCode, &last4, &b.AccountName, &b.Status, &b.CreatedAt, &b.VerifiedAt)
	b.AccountMasked = "••••" + last4
	if errors.Is(err, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}

// AddBankAccount registers a destination; it cannot receive payouts until Finance verifies it.
func (s *Service) AddBankAccount(ctx context.Context, merchantID, bankCode, number, name string, a Actor) (BankAccount, error) {
	if !bankRe.MatchString(bankCode) || !numberRe.MatchString(number) || len(name) < 2 || len(name) > 100 {
		return BankAccount{}, fmt.Errorf("%w: bank_code, account_number (5-32 digits) and account_name are required", ErrValidation)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BankAccount{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT 1 FROM merchants WHERE id = $1 FOR NO KEY UPDATE`, merchantID); err != nil {
		return BankAccount{}, err
	}
	var open int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM merchant_bank_accounts WHERE merchant_id = $1 AND status <> 'disabled'`, merchantID).Scan(&open); err != nil {
		return BankAccount{}, err
	}
	if open >= 10 {
		return BankAccount{}, fmt.Errorf("%w: too many bank accounts", ErrValidation)
	}
	var id string
	if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		return BankAccount{}, err
	}
	sealed, err := s.box.Seal([]byte(number), []byte(id))
	if err != nil {
		return BankAccount{}, err
	}
	b, err := scanBank(tx.QueryRow(ctx, `
		INSERT INTO merchant_bank_accounts (id, merchant_id, bank_code, account_enc, account_last4, account_name, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid) RETURNING `+bankCols,
		id, merchantID, bankCode, sealed, number[len(number)-4:], name, a.UserID))
	if err != nil {
		return BankAccount{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "bank_account.create", EntityType: "bank_account",
		EntityID: id, RequestID: a.RequestID, Metadata: map[string]any{"merchant_id": merchantID, "bank_code": bankCode, "last4": number[len(number)-4:]}}); err != nil {
		return BankAccount{}, err
	}
	return b, tx.Commit(ctx)
}

func (s *Service) ListBankAccounts(ctx context.Context, merchantID, status string) ([]BankAccount, error) {
	rows, err := s.db.Query(ctx, `SELECT `+bankCols+` FROM merchant_bank_accounts
		WHERE ($1 = '' OR merchant_id::text = $1) AND ($2 = '' OR status = $2) ORDER BY created_at DESC LIMIT 200`, merchantID, status)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (BankAccount, error) { return scanBank(r) })
}

// VerifyBankAccount is Finance's confirmation that the account name matches the merchant.
func (s *Service) VerifyBankAccount(ctx context.Context, id string, a Actor) (BankAccount, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BankAccount{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	b, err := scanBank(tx.QueryRow(ctx, `
		UPDATE merchant_bank_accounts SET status = 'verified', verified_by = $2::uuid, verified_at = now()
		WHERE id = $1 AND status = 'pending_verification' AND created_by IS DISTINCT FROM $2::uuid RETURNING `+bankCols, id, a.UserID))
	if errors.Is(err, ErrNotFound) {
		return BankAccount{}, ErrState
	}
	if err != nil {
		return BankAccount{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "bank_account.verify", EntityType: "bank_account",
		EntityID: id, RequestID: a.RequestID, Metadata: map[string]any{"merchant_id": b.MerchantID}}); err != nil {
		return BankAccount{}, err
	}
	return b, tx.Commit(ctx)
}

// DisableBankAccount retires an account (tenant-scoped). It refuses while payouts are open
// against it, so the destination of an approved payout can never be swapped from under Finance.
func (s *Service) DisableBankAccount(ctx context.Context, merchantID, id string, a Actor) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT 1 FROM merchants WHERE id = $1 FOR NO KEY UPDATE`, merchantID); err != nil {
		return err
	}
	var open bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payouts WHERE bank_account_id = $1 AND status IN ('pending_approval','approved'))`, id).Scan(&open); err != nil {
		return err
	}
	if open {
		return ErrAccountInUse
	}
	tag, err := tx.Exec(ctx, `UPDATE merchant_bank_accounts SET status = 'disabled', disabled_at = now()
		WHERE id = $1 AND merchant_id = $2 AND status <> 'disabled'`, id, merchantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "bank_account.disable", EntityType: "bank_account",
		EntityID: id, RequestID: a.RequestID, Metadata: map[string]any{"merchant_id": merchantID}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---- payouts ----

type Payout struct {
	ID            string     `json:"id"`
	MerchantID    string     `json:"merchant_id,omitempty"`
	BankAccountID string     `json:"bank_account_id"`
	Amount        int64      `json:"amount"`
	Currency      string     `json:"currency"`
	Status        string     `json:"status"`
	DryRun        bool       `json:"dry_run"`
	RequestedBy   string     `json:"requested_by,omitempty"`
	ApprovedAt    *time.Time `json:"approved_at"`
	DecisionNote  *string    `json:"decision_note"`
	BankReference *string    `json:"bank_reference"`
	PaidAt        *time.Time `json:"paid_at"`
	FailureReason *string    `json:"failure_reason"`
	CreatedAt     time.Time  `json:"created_at"`
}

const payoutCols = `id::text, merchant_id::text, bank_account_id::text, amount, currency, status, dry_run, requested_by::text,
	approved_at, decision_note, bank_reference, paid_at, failure_reason, created_at`

func scanPayout(row pgx.Row) (Payout, error) {
	var p Payout
	err := row.Scan(&p.ID, &p.MerchantID, &p.BankAccountID, &p.Amount, &p.Currency, &p.Status, &p.DryRun, &p.RequestedBy,
		&p.ApprovedAt, &p.DecisionNote, &p.BankReference, &p.PaidAt, &p.FailureReason, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

type RequestInput struct {
	MerchantID     string // from the authenticated principal only
	RequestedBy    string
	BankAccountID  string
	Amount         int64
	Currency       string
	IdempotencyKey string
	RequestHash    []byte
	RequestID      string
}

// Request creates a payout and reserves its funds. Idempotent per (merchant, Idempotency-Key):
// a retry returns the first payout, and a different body under the same key is an idempotency conflict.
func (s *Service) Request(ctx context.Context, in RequestInput) (Payout, bool, error) {
	if !idempotency.ValidKey(in.IdempotencyKey) {
		return Payout{}, false, fmt.Errorf("%w: Idempotency-Key header is required (8-128 chars)", ErrValidation)
	}
	if in.Amount <= 0 || in.Currency != "IDR" || in.BankAccountID == "" {
		return Payout{}, false, fmt.Errorf("%w: bank_account_id, amount > 0 and currency IDR are required", ErrValidation)
	}
	if !s.cfg.DryRun && !s.cfg.LiveEnabled {
		return Payout{}, false, ErrDisabled
	}
	sc := idempotency.Scope{MerchantID: in.MerchantID, Operation: "payout.request", Key: in.IdempotencyKey}
	resp, replayed, err := s.idem.Do(ctx, sc, in.RequestHash, func(ctx context.Context, tx pgx.Tx) (idempotency.Response, error) {
		id, err := s.createTx(ctx, tx, in)
		return idempotency.Response{Status: 201, ResourceType: "payout", ResourceID: id}, err
	})
	if err != nil {
		return Payout{}, false, err
	}
	p, err := s.Get(ctx, in.MerchantID, resp.ResourceID)
	return p, replayed, err
}

func (s *Service) createTx(ctx context.Context, tx pgx.Tx, in RequestInput) (string, error) {
	// Serializes payout requests of one merchant: the balance read below cannot go stale before the reserve posts.
	var status string
	if err := tx.QueryRow(ctx, `SELECT onboarding_status FROM merchants WHERE id = $1 FOR NO KEY UPDATE`, in.MerchantID).Scan(&status); err != nil {
		return "", err
	}
	if status != "approved" {
		return "", ErrMerchantState
	}
	if in.Amount < s.cfg.MinAmount || in.Amount > s.cfg.MaxAmount {
		return "", ErrLimit
	}
	if !uuidRe.MatchString(in.BankAccountID) {
		return "", ErrAccountUnusable
	}
	var usable bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM merchant_bank_accounts WHERE id = $1 AND merchant_id = $2 AND status = 'verified'
		               AND verified_at <= now() - $3::interval)`, in.BankAccountID, in.MerchantID, fmt.Sprintf("%d microseconds", s.cfg.Cooling.Microseconds())).Scan(&usable); err != nil {
		return "", err
	}
	if !usable {
		return "", ErrAccountUnusable
	}
	var day int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount),0) FROM payouts WHERE merchant_id = $1 AND currency = $2
		AND status NOT IN ('failed','cancelled') AND created_at > now() - interval '24 hours'`, in.MerchantID, in.Currency).Scan(&day); err != nil {
		return "", err
	}
	if day+in.Amount > s.cfg.DailyLimit {
		return "", ErrLimit
	}
	bal, err := settlement.BalanceOf(ctx, tx, in.MerchantID, in.Currency)
	if err != nil {
		return "", err
	}
	if in.Amount > bal.Available {
		return "", ErrInsufficient
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO payouts (merchant_id, bank_account_id, amount, currency, dry_run, requested_by)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		in.MerchantID, in.BankAccountID, in.Amount, in.Currency, s.cfg.DryRun, in.RequestedBy).Scan(&id); err != nil {
		return "", err
	}
	if _, err := ledger.PostTx(ctx, tx, reserveJournal(in.MerchantID, id, in.Currency, in.Amount)); err != nil {
		return "", err
	}
	return id, audit.Record(ctx, tx, audit.Entry{ActorID: in.RequestedBy, Action: "payout.request", EntityType: "payout", EntityID: id,
		RequestID: in.RequestID, Metadata: map[string]any{"merchant_id": in.MerchantID, "amount": in.Amount, "currency": in.Currency, "dry_run": s.cfg.DryRun}})
}

// Journals. The reserve moves the liability from "owed" to "in payout"; the bank leg happens only on paid.
func reserveJournal(merchantID, payoutID, cur string, amt int64) ledger.Txn {
	return ledger.Txn{DedupKey: "payout_reserved:" + payoutID, SourceType: "payout_reserved", SourceID: payoutID, Currency: cur,
		Entries: []ledger.Entry{
			{Account: ledger.MerchantPayable, MerchantID: merchantID, Direction: ledger.Debit, Amount: amt},
			{Account: ledger.PayoutInTransit, MerchantID: merchantID, Direction: ledger.Credit, Amount: amt}}}
}

func releaseJournal(merchantID, payoutID, cur string, amt int64) ledger.Txn {
	return ledger.Txn{DedupKey: "payout_released:" + payoutID, SourceType: "payout_released", SourceID: payoutID, Currency: cur,
		Entries: []ledger.Entry{
			{Account: ledger.PayoutInTransit, MerchantID: merchantID, Direction: ledger.Debit, Amount: amt},
			{Account: ledger.MerchantPayable, MerchantID: merchantID, Direction: ledger.Credit, Amount: amt}}}
}

func paidJournal(merchantID, payoutID, cur string, amt int64) ledger.Txn {
	return ledger.Txn{DedupKey: "payout_paid:" + payoutID, SourceType: "payout_paid", SourceID: payoutID, Currency: cur,
		Entries: []ledger.Entry{
			{Account: ledger.PayoutInTransit, MerchantID: merchantID, Direction: ledger.Debit, Amount: amt},
			{Account: ledger.CashBankPT, Direction: ledger.Credit, Amount: amt}}}
}

// Get is tenant-scoped.
func (s *Service) Get(ctx context.Context, merchantID, id string) (Payout, error) {
	p, err := scanPayout(s.db.QueryRow(ctx, `SELECT `+payoutCols+` FROM payouts WHERE id::text = $1 AND merchant_id::text = $2`, id, merchantID))
	if err != nil {
		return p, err
	}
	return p, nil
}

func (s *Service) List(ctx context.Context, merchantID, status string, limit int) ([]Payout, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT `+payoutCols+` FROM payouts WHERE ($1 = '' OR merchant_id::text = $1)
		AND ($2 = '' OR status = $2) ORDER BY created_at DESC, id LIMIT $3`, merchantID, status, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Payout, error) { return scanPayout(r) })
}

// GetAny is for platform users (Finance / Ops); the caller's permission gates it.
func (s *Service) GetAny(ctx context.Context, id string) (Payout, error) {
	return scanPayout(s.db.QueryRow(ctx, `SELECT `+payoutCols+` FROM payouts WHERE id::text = $1`, id))
}

type lockedPayout struct {
	Payout
	approvedBy string
}

// lock reads the payout under a row lock; scope restricts it to a merchant ("" = platform).
func lock(ctx context.Context, tx pgx.Tx, id, merchantID string) (lockedPayout, error) {
	var lp lockedPayout
	err := tx.QueryRow(ctx, `SELECT `+payoutCols+`, COALESCE(approved_by::text,'') FROM payouts
		WHERE id::text = $1 AND ($2 = '' OR merchant_id::text = $2) FOR NO KEY UPDATE`, id, merchantID).
		Scan(&lp.ID, &lp.MerchantID, &lp.BankAccountID, &lp.Amount, &lp.Currency, &lp.Status, &lp.DryRun, &lp.RequestedBy,
			&lp.ApprovedAt, &lp.DecisionNote, &lp.BankReference, &lp.PaidAt, &lp.FailureReason, &lp.CreatedAt, &lp.approvedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return lp, ErrNotFound
	}
	return lp, err
}

// settle ends a payout without paying it (cancel / reject / failed) and releases the reservation.
func (s *Service) end(ctx context.Context, id, merchantID string, from []string, to, note string, a Actor, action string) (Payout, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	p, err := lock(ctx, tx, id, merchantID)
	if err != nil {
		return Payout{}, err
	}
	if p.Status == to {
		return p.Payout, nil // retried request: already done
	}
	if !contains(from, p.Status) {
		return Payout{}, ErrState
	}
	col := "decision_note"
	if to == "failed" {
		col = "failure_reason"
	}
	if _, err := tx.Exec(ctx, `UPDATE payouts SET status = $2, `+col+` = NULLIF($3,''), updated_at = now(), version = version + 1 WHERE id = $1`, id, to, note); err != nil {
		return Payout{}, err
	}
	if _, err := ledger.PostTx(ctx, tx, releaseJournal(p.MerchantID, id, p.Currency, p.Amount)); err != nil {
		return Payout{}, err
	}
	if err := notify(ctx, tx, p.Payout, to); err != nil {
		return Payout{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: action, EntityType: "payout", EntityID: id, RequestID: a.RequestID,
		Metadata: map[string]any{"merchant_id": p.MerchantID, "amount": p.Amount, "from": p.Status}}); err != nil {
		return Payout{}, err
	}
	out, err := scanPayout(tx.QueryRow(ctx, `SELECT `+payoutCols+` FROM payouts WHERE id = $1`, id))
	if err != nil {
		return Payout{}, err
	}
	return out, tx.Commit(ctx)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// Cancel is the merchant withdrawing a payout that has not been approved yet.
func (s *Service) Cancel(ctx context.Context, merchantID, id string, a Actor) (Payout, error) {
	return s.end(ctx, id, merchantID, []string{"pending_approval"}, "cancelled", "cancelled by merchant", a, "payout.cancel")
}

// Reject is the checker declining a pending payout. A note is mandatory.
func (s *Service) Reject(ctx context.Context, id, note string, a Actor) (Payout, error) {
	if len(note) < 3 || len(note) > 500 {
		return Payout{}, fmt.Errorf("%w: note (3-500 chars) is required", ErrValidation)
	}
	return s.end(ctx, id, "", []string{"pending_approval"}, "cancelled", note, a, "payout.reject")
}

// MarkFailed records that an approved payout's transfer did not happen; the funds return to the merchant.
func (s *Service) MarkFailed(ctx context.Context, id, reason string, a Actor) (Payout, error) {
	if len(reason) < 3 || len(reason) > 500 {
		return Payout{}, fmt.Errorf("%w: reason (3-500 chars) is required", ErrValidation)
	}
	return s.end(ctx, id, "", []string{"approved"}, "failed", reason, a, "payout.mark_failed")
}

// Approve is the checker step. It re-checks the destination and refuses the requester.
func (s *Service) Approve(ctx context.Context, id string, a Actor) (Payout, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	p, err := lock(ctx, tx, id, "")
	if err != nil {
		return Payout{}, err
	}
	if p.Status == "approved" && p.approvedBy == a.UserID {
		return p.Payout, nil // retried by the same checker
	}
	if p.Status != "pending_approval" {
		return Payout{}, ErrState
	}
	if p.RequestedBy == a.UserID {
		return Payout{}, ErrSelfApproval
	}
	var usable bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM merchant_bank_accounts WHERE id = $1 AND status = 'verified')`, p.BankAccountID).Scan(&usable); err != nil {
		return Payout{}, err
	}
	if !usable {
		return Payout{}, ErrAccountUnusable
	}
	if _, err := tx.Exec(ctx, `UPDATE payouts SET status = 'approved', approved_by = $2::uuid, approved_at = now(), updated_at = now(), version = version + 1
		WHERE id = $1`, id, a.UserID); err != nil {
		return Payout{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "payout.approve", EntityType: "payout", EntityID: id, RequestID: a.RequestID,
		Metadata: map[string]any{"merchant_id": p.MerchantID, "amount": p.Amount}}); err != nil {
		return Payout{}, err
	}
	out, err := scanPayout(tx.QueryRow(ctx, `SELECT `+payoutCols+` FROM payouts WHERE id = $1`, id))
	if err != nil {
		return Payout{}, err
	}
	return out, tx.Commit(ctx)
}

// MarkPaid records that Finance made the transfer. Money leaves the books exactly once: the journal's
// dedup key and the status check both guard it, and repeating it with the same reference is a no-op.
func (s *Service) MarkPaid(ctx context.Context, id, bankReference string, a Actor) (Payout, error) {
	if len(bankReference) < 3 || len(bankReference) > 128 {
		return Payout{}, fmt.Errorf("%w: bank_reference (3-128 chars) is required", ErrValidation)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	p, err := lock(ctx, tx, id, "")
	if err != nil {
		return Payout{}, err
	}
	if p.Status == "paid" {
		if p.BankReference != nil && *p.BankReference == bankReference {
			return p.Payout, nil
		}
		return Payout{}, ErrRefConflict
	}
	if p.Status != "approved" {
		return Payout{}, ErrState
	}
	if _, err := tx.Exec(ctx, `UPDATE payouts SET status = 'paid', bank_reference = $2, paid_by = $3::uuid, paid_at = now(), updated_at = now(), version = version + 1
		WHERE id = $1`, id, bankReference, a.UserID); err != nil {
		return Payout{}, err
	}
	if _, err := ledger.PostTx(ctx, tx, paidJournal(p.MerchantID, id, p.Currency, p.Amount)); err != nil {
		return Payout{}, err
	}
	if err := notify(ctx, tx, p.Payout, "paid"); err != nil {
		return Payout{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "payout.mark_paid", EntityType: "payout", EntityID: id, RequestID: a.RequestID,
		Metadata: map[string]any{"merchant_id": p.MerchantID, "amount": p.Amount, "dry_run": p.DryRun}}); err != nil {
		return Payout{}, err
	}
	out, err := scanPayout(tx.QueryRow(ctx, `SELECT `+payoutCols+` FROM payouts WHERE id = $1`, id))
	if err != nil {
		return Payout{}, err
	}
	return out, tx.Commit(ctx)
}

type Destination struct {
	BankCode      string `json:"bank_code"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
}

// RevealDestination decrypts the account number so Finance can make the transfer. Only approved
// payouts qualify, and every reveal is audited (access to sensitive data).
func (s *Service) RevealDestination(ctx context.Context, id string, a Actor) (Destination, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Destination{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var d Destination
	var enc []byte
	var acct, mid string
	err = tx.QueryRow(ctx, `
		SELECT b.id::text, p.merchant_id::text, b.bank_code, b.account_name, b.account_enc
		FROM payouts p JOIN merchant_bank_accounts b ON b.id = p.bank_account_id
		WHERE p.id::text = $1 AND p.status = 'approved'`, id).Scan(&acct, &mid, &d.BankCode, &d.AccountName, &enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return Destination{}, ErrNotFound
	}
	if err != nil {
		return Destination{}, err
	}
	num, err := s.box.Open(enc, []byte(acct))
	if err != nil {
		return Destination{}, err
	}
	d.AccountNumber = string(num)
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "payout.reveal_destination", EntityType: "payout", EntityID: id,
		RequestID: a.RequestID, Metadata: map[string]any{"merchant_id": mid, "bank_account_id": acct}}); err != nil {
		return Destination{}, err
	}
	return d, tx.Commit(ctx)
}

// notify queues the merchant webhook for a final payout status, in the same transaction as the change.
func notify(ctx context.Context, tx pgx.Tx, p Payout, to string) error {
	return outbound.EnqueueTx(ctx, tx, p.MerchantID, "payout."+to, p.ID, map[string]any{"payout": map[string]any{
		"id": p.ID, "amount": p.Amount, "currency": p.Currency, "status": to, "dry_run": p.DryRun}})
}
