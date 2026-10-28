// Package settlement records gateway -> PT settlement batches and derives the merchant
// pending/available split (PRD reconciliation.md, ledger.md).
//
// A batch is booked only when the figure Finance read from the official report equals the
// figure the server computes from its own records (sum of gross - MDR). Anything else becomes
// a reconciliation exception and changes no balance. There is deliberately no report parser:
// the DOKU settlement report format is unconfirmed [KD] (PRD §0 #10, #16).
package settlement

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/ledger"
)

var (
	ErrValidation    = errors.New("settlement: validation failed")
	ErrIneligible    = errors.New("settlement: some payments are unknown, not succeeded, of another gateway/currency, or already settled")
	ErrMismatch      = errors.New("settlement: reported net differs from the expected net")
	ErrBatchConflict = errors.New("settlement: batch_ref already recorded with different content")
	ErrNotFound      = errors.New("settlement: not found")
)

const MaxBatchPayments = 1000

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

type Actor struct{ UserID, RequestID string }

type BatchInput struct {
	Gateway        string
	BatchRef       string
	Currency       string
	SettlementDate time.Time // date only
	ReportedNet    int64     // read by Finance from the official report / bank statement
	PaymentIDs     []string
	Note           string
}

type Batch struct {
	ID             string    `json:"id"`
	Gateway        string    `json:"gateway"`
	BatchRef       string    `json:"batch_ref"`
	Currency       string    `json:"currency"`
	SettlementDate string    `json:"settlement_date"`
	ReportedNet    int64     `json:"reported_net"`
	ExpectedNet    int64     `json:"expected_net"`
	PaymentCount   int       `json:"payment_count"`
	CreatedAt      time.Time `json:"created_at"`
	Replayed       bool      `json:"replayed,omitempty"`
}

// Mismatch carries the figures of a rejected batch (also stored as an exception).
type Mismatch struct{ Reported, Expected int64 }

func (m *Mismatch) Error() string {
	return fmt.Sprintf("%v: reported %d, expected %d", ErrMismatch, m.Reported, m.Expected)
}
func (m *Mismatch) Unwrap() error { return ErrMismatch }

func (in BatchInput) validate() error {
	switch {
	case in.Gateway == "" || len(in.BatchRef) < 1 || len(in.BatchRef) > 128 || in.Currency == "":
		return fmt.Errorf("%w: gateway, batch_ref (1-128) and currency are required", ErrValidation)
	case in.ReportedNet <= 0:
		return fmt.Errorf("%w: reported_net must be positive", ErrValidation)
	case in.SettlementDate.IsZero():
		return fmt.Errorf("%w: settlement_date is required", ErrValidation)
	case len(in.PaymentIDs) == 0 || len(in.PaymentIDs) > MaxBatchPayments:
		return fmt.Errorf("%w: payment_ids must hold 1..%d ids", ErrValidation, MaxBatchPayments)
	case len(in.Note) > 500:
		return fmt.Errorf("%w: note too long", ErrValidation)
	}
	seen := map[string]bool{}
	for _, id := range in.PaymentIDs {
		if seen[id] {
			return fmt.Errorf("%w: duplicate payment id", ErrValidation)
		}
		seen[id] = true
	}
	return nil
}

// RecordBatch books one settlement batch atomically:
//
//	Dr cash_bank_pt        reported_net
//	Cr <gateway clearing>  expected_net      (equal: a mismatch is refused, never booked;
//	                                          cash_ids_clearing for IDS, else cash_doku_clearing)
//
// It is idempotent per (gateway, batch_ref): the identical batch replays the stored result; a
// different batch under the same reference is ErrBatchConflict. A payment can settle once
// (unique settlement_items.payment_id), so overlapping concurrent batches cannot both win.
func (s *Service) RecordBatch(ctx context.Context, in BatchInput, a Actor) (Batch, error) {
	if err := in.validate(); err != nil {
		return Batch{}, err
	}
	b, err := s.recordTx(ctx, in, a)
	var mm *Mismatch
	if errors.As(err, &mm) {
		// The transaction is rolled back; the exception must survive it.
		if xerr := s.openException(ctx, in, mm, a); xerr != nil {
			return Batch{}, xerr
		}
	}
	return b, err
}

func (s *Service) recordTx(ctx context.Context, in BatchInput, a Actor) (Batch, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	ids := slices.Clone(in.PaymentIDs)
	slices.Sort(ids) // consistent lock order across concurrent batches

	var count int
	var expected int64
	err = tx.QueryRow(ctx, `
		SELECT count(*), COALESCE(SUM(p.amount - p.mdr_fee), 0)
		FROM payments p
		WHERE p.id = ANY($1::uuid[]) AND p.status = 'succeeded' AND p.gateway = $2 AND p.currency = $3
		  AND NOT EXISTS (SELECT 1 FROM settlement_items si WHERE si.payment_id = p.id)`,
		ids, in.Gateway, in.Currency).Scan(&count, &expected)
	if err != nil {
		if isInvalidText(err) {
			return Batch{}, fmt.Errorf("%w: payment_ids must be uuids", ErrValidation)
		}
		return Batch{}, err
	}

	var batchID string
	var created time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO settlement_batches (gateway, batch_ref, currency, settlement_date, reported_net, expected_net,
		                                payment_count, note, created_by)
		SELECT $1::text,$2::text,$3::text,$4::date,$5::bigint,$6::bigint,$7::int,NULLIF($8::text,''),NULLIF($9::text,'')::uuid
		WHERE $5::bigint = $6::bigint AND $7::int = $10::int
		ON CONFLICT (gateway, batch_ref) DO NOTHING RETURNING id::text, created_at`,
		in.Gateway, in.BatchRef, in.Currency, in.SettlementDate, in.ReportedNet, expected, count, in.Note, a.UserID, len(ids)).
		Scan(&batchID, &created)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Either the batch exists already (replay / conflict) or it did not reconcile.
		if ex, ok, rerr := s.existing(ctx, tx, in, ids); rerr != nil {
			return Batch{}, rerr
		} else if ok {
			return ex, nil
		}
		if count != len(ids) {
			return Batch{}, ErrIneligible
		}
		return Batch{}, &Mismatch{Reported: in.ReportedNet, Expected: expected}
	case err != nil:
		return Batch{}, err
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO settlement_items (batch_id, payment_id, merchant_id, gross, mdr_fee, expected_net)
		SELECT $1, p.id, p.merchant_id, p.amount, p.mdr_fee, p.amount - p.mdr_fee
		FROM payments p WHERE p.id = ANY($2::uuid[]) AND p.status = 'succeeded'
		ORDER BY p.id ON CONFLICT (payment_id) DO NOTHING`, batchID, ids)
	if err != nil {
		return Batch{}, err
	}
	if int(tag.RowsAffected()) != len(ids) {
		return Batch{}, ErrIneligible // a concurrent batch settled one of them first
	}

	j := ledger.Txn{DedupKey: "settlement:" + in.Gateway + ":" + in.BatchRef, SourceType: "settlement_batch", SourceID: batchID,
		Currency: in.Currency, Entries: []ledger.Entry{
			{Account: ledger.CashBankPT, Direction: ledger.Debit, Amount: in.ReportedNet},
			{Account: ledger.ClearingAccount(in.Gateway), Direction: ledger.Credit, Amount: expected},
		}}
	if _, err := ledger.PostTx(ctx, tx, j); err != nil {
		return Batch{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "settlement_batch.record",
		EntityType: "settlement_batch", EntityID: batchID, RequestID: a.RequestID,
		Metadata: map[string]any{"gateway": in.Gateway, "batch_ref": in.BatchRef, "net": expected, "payments": len(ids)}}); err != nil {
		return Batch{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Batch{}, err
	}
	return Batch{ID: batchID, Gateway: in.Gateway, BatchRef: in.BatchRef, Currency: in.Currency,
		SettlementDate: in.SettlementDate.Format("2006-01-02"), ReportedNet: in.ReportedNet, ExpectedNet: expected,
		PaymentCount: len(ids), CreatedAt: created}, nil
}

// existing returns the stored batch when (gateway, batch_ref) is taken and the submission is identical.
func (s *Service) existing(ctx context.Context, tx pgx.Tx, in BatchInput, sortedIDs []string) (Batch, bool, error) {
	var b Batch
	var date time.Time
	err := tx.QueryRow(ctx, `
		SELECT id::text, gateway, batch_ref, currency, settlement_date, reported_net, expected_net, payment_count, created_at
		FROM settlement_batches WHERE gateway = $1 AND batch_ref = $2`, in.Gateway, in.BatchRef).
		Scan(&b.ID, &b.Gateway, &b.BatchRef, &b.Currency, &date, &b.ReportedNet, &b.ExpectedNet, &b.PaymentCount, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Batch{}, false, nil
	}
	if err != nil {
		return Batch{}, false, err
	}
	b.SettlementDate = date.Format("2006-01-02")
	rows, err := tx.Query(ctx, `SELECT payment_id::text FROM settlement_items WHERE batch_id = $1 ORDER BY payment_id::text`, b.ID)
	if err != nil {
		return Batch{}, false, err
	}
	have, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return Batch{}, false, err
	}
	want := make([]string, len(sortedIDs))
	for i, id := range sortedIDs {
		want[i] = lowerUUID(id)
	}
	slices.Sort(want)
	if b.Currency != in.Currency || b.ReportedNet != in.ReportedNet || !slices.Equal(have, want) {
		return Batch{}, false, ErrBatchConflict
	}
	b.Replayed = true
	return b, true, nil
}

func lowerUUID(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'F' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func (s *Service) openException(ctx context.Context, in BatchInput, m *Mismatch, a Actor) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO reconciliation_exceptions (kind, gateway, reference, details, created_by)
		VALUES ('settlement_mismatch', $1, $2, jsonb_build_object('reported_net', $3::bigint, 'expected_net', $4::bigint,
		        'payment_count', $5::int, 'currency', $6::text, 'difference', $3::bigint - $4::bigint), NULLIF($7,'')::uuid)
		ON CONFLICT (kind, gateway, reference) WHERE status = 'open' DO NOTHING RETURNING id::text`,
		in.Gateway, in.BatchRef, m.Reported, m.Expected, len(in.PaymentIDs), in.Currency, a.UserID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already open
	}
	if err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "reconciliation_exception.open",
		EntityType: "reconciliation_exception", EntityID: id, RequestID: a.RequestID,
		Metadata: map[string]any{"gateway": in.Gateway, "batch_ref": in.BatchRef}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func isInvalidText(err error) bool {
	var pe interface{ SQLState() string }
	return errors.As(err, &pe) && pe.SQLState() == "22P02"
}
