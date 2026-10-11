// Package ledger posts immutable double-entry journals (PRD ledger.md, §0 #5 #6 #14).
//
// Idempotency: ledger_transactions.dedup_key is UNIQUE, so the same event never
// posts twice even under concurrency. Balances are always derived from entries.
// The database additionally enforces debit == credit at commit (deferred trigger)
// and forbids UPDATE/DELETE on journals.
package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/pricing"
)

type Direction string

const (
	Debit  Direction = "debit"
	Credit Direction = "credit"
)

// Account codes (Model B minimal set).
const (
	CashDokuClearing   = "cash_doku_clearing"
	CashBankPT         = "cash_bank_pt"
	MerchantPayable    = "merchant_payable"
	PayoutInTransit    = "payout_in_transit"
	RefundPayable      = "refund_payable"
	PlatformFeeRevenue = "platform_fee_revenue"
	GatewayFeeExpense  = "gateway_fee_expense"
	TaxPayable         = "tax_payable"
)

var merchantScoped = map[string]bool{MerchantPayable: true, PayoutInTransit: true, RefundPayable: true}

var (
	ErrUnbalanced    = errors.New("ledger: debits != credits")
	ErrInvalid       = errors.New("ledger: invalid transaction")
	ErrDedupMismatch = errors.New("ledger: dedup key already used by a different event")
)

// Entry is one leg. MerchantID is required for merchant-scoped accounts and
// must be empty for platform accounts.
type Entry struct {
	Account    string
	MerchantID string
	Direction  Direction
	Amount     int64
}

// Txn is one balanced journal for one source event.
type Txn struct {
	DedupKey   string
	SourceType string
	SourceID   string
	Currency   string
	Entries    []Entry
}

// Result.Duplicate is true when the dedup key was already posted (no new entries written).
type Result struct {
	TxnID     string
	Duplicate bool
}

func (t Txn) validate() error {
	if t.DedupKey == "" || t.SourceType == "" || t.SourceID == "" || t.Currency == "" || len(t.Entries) < 2 {
		return fmt.Errorf("%w: missing fields", ErrInvalid)
	}
	var d, c int64
	for _, e := range t.Entries {
		if e.Amount <= 0 || (e.Direction != Debit && e.Direction != Credit) {
			return fmt.Errorf("%w: bad entry %+v", ErrInvalid, e)
		}
		if merchantScoped[e.Account] != (e.MerchantID != "") {
			return fmt.Errorf("%w: account %s merchant scope mismatch", ErrInvalid, e.Account)
		}
		if e.Direction == Debit {
			d += e.Amount
		} else {
			c += e.Amount
		}
	}
	if d != c {
		return fmt.Errorf("%w: debit %d credit %d", ErrUnbalanced, d, c)
	}
	return nil
}

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

// Post runs PostTx in its own transaction.
func (s *Service) Post(ctx context.Context, t Txn) (Result, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	r, err := PostTx(ctx, tx, t)
	if err != nil {
		return Result{}, err
	}
	// The balance trigger is deferred: an unbalanced journal fails here, at commit.
	return r, tx.Commit(ctx)
}

// PostTx posts inside the caller's transaction so a state change and its journal
// commit atomically. A losing concurrent duplicate blocks on the unique index,
// then reports Duplicate once the winner commits.
func PostTx(ctx context.Context, tx pgx.Tx, t Txn) (Result, error) {
	if err := t.validate(); err != nil {
		return Result{}, err
	}
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO ledger_transactions (dedup_key, source_type, source_id, currency)
		VALUES ($1,$2,$3,$4) ON CONFLICT (dedup_key) DO NOTHING RETURNING id::text`,
		t.DedupKey, t.SourceType, t.SourceID, t.Currency).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		var st, sid, cur string
		if err := tx.QueryRow(ctx,
			`SELECT id::text, source_type, source_id, currency FROM ledger_transactions WHERE dedup_key = $1`,
			t.DedupKey).Scan(&id, &st, &sid, &cur); err != nil {
			return Result{}, err
		}
		if st != t.SourceType || sid != t.SourceID || cur != t.Currency {
			return Result{}, ErrDedupMismatch
		}
		return Result{TxnID: id, Duplicate: true}, nil
	}
	if err != nil {
		return Result{}, err
	}
	for _, e := range t.Entries {
		acc, err := accountID(ctx, tx, e.Account, e.MerchantID, t.Currency)
		if err != nil {
			return Result{}, err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO ledger_entries (txn_id, account_id, direction, amount) VALUES ($1,$2,$3,$4)`,
			id, acc, string(e.Direction), e.Amount); err != nil {
			return Result{}, err
		}
	}
	return Result{TxnID: id}, nil
}

// accountID gets or creates the account (race-safe via the unique expression index).
func accountID(ctx context.Context, tx pgx.Tx, code, merchantID, currency string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO ledger_accounts (code, merchant_id, currency) VALUES ($1, NULLIF($2,'')::uuid, $3)
		ON CONFLICT (code, COALESCE(merchant_id, '00000000-0000-0000-0000-000000000000'::uuid), currency)
		DO UPDATE SET currency = EXCLUDED.currency RETURNING id::text`,
		code, merchantID, currency).Scan(&id)
	return id, err
}

// PaymentSucceeded builds the Model B journal for a successful payment:
//
//	Dr cash_doku_clearing  gross - MDR   (expected from the gateway; [KD] pending DOKU confirmation)
//	Cr merchant_payable    merchant_net  (liability to the merchant, not EraPay revenue)
//	Cr platform_fee_revenue platform_fee
//
// Zero-valued legs are omitted. MDR is retained by the gateway and never enters
// EraPay's books here; reconciliation compares it with the settlement report.
func PaymentSucceeded(merchantID, paymentID string, b pricing.Breakdown) (Txn, error) {
	if b.Gross != b.MDRFee+b.PlatformFee+b.MerchantNet || b.Gross <= 0 {
		return Txn{}, fmt.Errorf("%w: breakdown does not add up", ErrInvalid)
	}
	t := Txn{
		DedupKey: "payment_succeeded:" + paymentID, SourceType: "payment_succeeded",
		SourceID: paymentID, Currency: b.Currency,
	}
	add := func(acc, mid string, d Direction, amt int64) {
		if amt > 0 {
			t.Entries = append(t.Entries, Entry{acc, mid, d, amt})
		}
	}
	add(CashDokuClearing, "", Debit, b.Gross-b.MDRFee)
	add(MerchantPayable, merchantID, Credit, b.MerchantNet)
	add(PlatformFeeRevenue, "", Credit, b.PlatformFee)
	return t, t.validate()
}

// Reverse returns the mirror journal of an already posted transaction (corrections
// are reversals, never edits). Its dedup key makes the reversal happen once.
func (s *Service) Reverse(ctx context.Context, originalDedupKey, reason string) (Result, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var id, st, sid, cur string
	err = tx.QueryRow(ctx,
		`SELECT id::text, source_type, source_id, currency FROM ledger_transactions WHERE dedup_key = $1`,
		originalDedupKey).Scan(&id, &st, &sid, &cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, fmt.Errorf("%w: original %q not found", ErrInvalid, originalDedupKey)
	}
	if err != nil {
		return Result{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT a.code, COALESCE(a.merchant_id::text,''), e.direction, e.amount
		FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id WHERE e.txn_id = $1`, id)
	if err != nil {
		return Result{}, err
	}
	orig, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Entry, error) {
		var e Entry
		var d string
		err := r.Scan(&e.Account, &e.MerchantID, &d, &e.Amount)
		e.Direction = Direction(d)
		return e, err
	})
	if err != nil {
		return Result{}, err
	}
	rev := Txn{DedupKey: "reversal:" + originalDedupKey, SourceType: "reversal:" + st, SourceID: sid, Currency: cur}
	for _, e := range orig {
		if e.Direction == Debit {
			e.Direction = Credit
		} else {
			e.Direction = Debit
		}
		rev.Entries = append(rev.Entries, e)
	}
	_ = reason // recorded by the caller in audit_logs; the journal itself stays minimal
	r, err := PostTx(ctx, tx, rev)
	if err != nil {
		return Result{}, err
	}
	return r, tx.Commit(ctx)
}

// MerchantBalance is derived from entries only (PRD §0 #14b). Payable is what
// EraPay owes the merchant. The pending/available/in_payout split needs settlement
// status and payout semantics (steps 9b/11) and is intentionally NOT guessed here.
type MerchantBalance struct {
	Currency string
	Payable  int64
}

func (s *Service) MerchantBalance(ctx context.Context, merchantID, currency string) (MerchantBalance, error) {
	var dmc int64
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(debit_minus_credit), 0) FROM ledger_account_balances
		WHERE merchant_id = $1 AND currency = $2 AND code = 'merchant_payable'`,
		merchantID, currency).Scan(&dmc)
	return MerchantBalance{Currency: currency, Payable: -dmc}, err // liability: credits increase it
}
