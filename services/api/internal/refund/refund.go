// Package refund returns money for a succeeded payment (PRD refund.md, §0 #9, #14).
//
// A refund is its own entity and state machine: it never changes the payment's status. The merchant's
// funds are reserved in the ledger when the refund is requested (merchant_payable -> refund_payable), the
// total of live refunds can never exceed the payment amount (checked under the payment row lock), and the
// reservation can only come out of settled `available` money, so a balance never goes negative.
// There is no gateway call: DOKU refund is unconfirmed [KD]. Finance returns the money out of band and
// records it, exactly as for payouts.
package refund

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/ledger"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/outbound"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/settlement"
)

var (
	ErrValidation    = errors.New("refund: validation failed")
	ErrNotFound      = errors.New("refund: not found")
	ErrNotRefundable = errors.New("refund: only a succeeded payment can be refunded")
	ErrTooMuch       = errors.New("refund: amount exceeds what is left to refund on this payment")
	ErrInsufficient  = errors.New("refund: merchant's available (settled) balance cannot cover this refund")
	ErrState         = errors.New("refund: not allowed in the current status")
	ErrSelfApproval  = errors.New("refund: the requester cannot approve their own refund")
	ErrRefConflict   = errors.New("refund: already refunded with a different reference")
)

type Service struct {
	db   *pgxpool.Pool
	idem *idempotency.Store
}

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db, idem: idempotency.NewStore(db)} }

type Actor struct{ UserID, RequestID string }

type Refund struct {
	ID            string     `json:"id"`
	MerchantID    string     `json:"merchant_id,omitempty"`
	PaymentID     string     `json:"payment_id"`
	Amount        int64      `json:"amount"`
	Currency      string     `json:"currency"`
	Reason        string     `json:"reason"`
	Status        string     `json:"status"`
	RequestedBy   string     `json:"requested_by,omitempty"`
	ApprovedAt    *time.Time `json:"approved_at"`
	DecisionNote  *string    `json:"decision_note"`
	BankReference *string    `json:"bank_reference"`
	RefundedAt    *time.Time `json:"refunded_at"`
	FailureReason *string    `json:"failure_reason"`
	CreatedAt     time.Time  `json:"created_at"`
}

const cols = `id::text, merchant_id::text, payment_id::text, amount, currency, reason, status, requested_by::text,
	approved_at, decision_note, bank_reference, refunded_at, failure_reason, created_at`

func scan(row pgx.Row) (Refund, error) {
	var r Refund
	err := row.Scan(&r.ID, &r.MerchantID, &r.PaymentID, &r.Amount, &r.Currency, &r.Reason, &r.Status, &r.RequestedBy,
		&r.ApprovedAt, &r.DecisionNote, &r.BankReference, &r.RefundedAt, &r.FailureReason, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

type RequestInput struct {
	MerchantID     string // from the authenticated principal only
	RequestedBy    string
	PaymentID      string
	Amount         int64
	Reason         string
	IdempotencyKey string
	RequestHash    []byte
	RequestID      string
}

// Request creates a refund and reserves the money. Idempotent per (merchant, Idempotency-Key).
func (s *Service) Request(ctx context.Context, in RequestInput) (Refund, bool, error) {
	if !idempotency.ValidKey(in.IdempotencyKey) {
		return Refund{}, false, fmt.Errorf("%w: Idempotency-Key header is required (8-128 chars)", ErrValidation)
	}
	if in.Amount <= 0 || len(in.Reason) < 3 || len(in.Reason) > 255 {
		return Refund{}, false, fmt.Errorf("%w: amount > 0 and a reason (3-255 chars) are required", ErrValidation)
	}
	sc := idempotency.Scope{MerchantID: in.MerchantID, Operation: "refund.request", Key: in.IdempotencyKey}
	resp, replayed, err := s.idem.Do(ctx, sc, in.RequestHash, func(ctx context.Context, tx pgx.Tx) (idempotency.Response, error) {
		id, err := s.createTx(ctx, tx, in)
		return idempotency.Response{Status: 201, ResourceType: "refund", ResourceID: id}, err
	})
	if err != nil {
		return Refund{}, false, err
	}
	r, err := s.Get(ctx, in.MerchantID, resp.ResourceID)
	return r, replayed, err
}

func (s *Service) createTx(ctx context.Context, tx pgx.Tx, in RequestInput) (string, error) {
	// Lock order is always merchant, then payment. The merchant lock serializes balance checks; the payment
	// lock serializes the "total refunds <= amount" check of one payment.
	var onboarding string
	if err := tx.QueryRow(ctx, `SELECT onboarding_status FROM merchants WHERE id = $1 FOR NO KEY UPDATE`, in.MerchantID).Scan(&onboarding); err != nil {
		return "", err
	}
	var amount int64
	var currency, status string
	err := tx.QueryRow(ctx, `SELECT amount, currency, status FROM payments WHERE id::text = $1 AND merchant_id = $2 FOR NO KEY UPDATE`,
		in.PaymentID, in.MerchantID).Scan(&amount, &currency, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound // another tenant's payment is indistinguishable from a missing one
	}
	if err != nil {
		return "", err
	}
	if status != "succeeded" {
		return "", ErrNotRefundable
	}
	var live int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount),0) FROM refunds WHERE payment_id::text = $1
		AND status IN ('pending_approval','approved','refunded')`, in.PaymentID).Scan(&live); err != nil {
		return "", err
	}
	if in.Amount > amount-live {
		return "", ErrTooMuch
	}
	bal, err := settlement.BalanceOf(ctx, tx, in.MerchantID, currency)
	if err != nil {
		return "", err
	}
	if in.Amount > bal.Available {
		return "", ErrInsufficient
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO refunds (merchant_id, payment_id, amount, currency, reason, requested_by)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		in.MerchantID, in.PaymentID, in.Amount, currency, in.Reason, in.RequestedBy).Scan(&id); err != nil {
		return "", err
	}
	if _, err := ledger.PostTx(ctx, tx, journal("refund_reserved", id, in.MerchantID, currency, in.Amount,
		ledger.MerchantPayable, ledger.RefundPayable)); err != nil {
		return "", err
	}
	return id, audit.Record(ctx, tx, audit.Entry{ActorID: in.RequestedBy, Action: "refund.request", EntityType: "refund", EntityID: id,
		RequestID: in.RequestID, Metadata: map[string]any{"merchant_id": in.MerchantID, "payment_id": in.PaymentID, "amount": in.Amount}})
}

// journal moves amt from the debit account to the credit account; the refund_payable leg is merchant scoped.
func journal(kind, id, merchantID, cur string, amt int64, debit, credit string) ledger.Txn {
	leg := func(acc string, d ledger.Direction) ledger.Entry {
		e := ledger.Entry{Account: acc, Direction: d, Amount: amt}
		if acc == ledger.MerchantPayable || acc == ledger.RefundPayable {
			e.MerchantID = merchantID
		}
		return e
	}
	return ledger.Txn{DedupKey: kind + ":" + id, SourceType: kind, SourceID: id, Currency: cur,
		Entries: []ledger.Entry{leg(debit, ledger.Debit), leg(credit, ledger.Credit)}}
}

func (s *Service) Get(ctx context.Context, merchantID, id string) (Refund, error) {
	return scan(s.db.QueryRow(ctx, `SELECT `+cols+` FROM refunds WHERE id::text = $1 AND merchant_id::text = $2`, id, merchantID))
}

func (s *Service) GetAny(ctx context.Context, id string) (Refund, error) {
	return scan(s.db.QueryRow(ctx, `SELECT `+cols+` FROM refunds WHERE id::text = $1`, id))
}

// List filters by merchant ("" = all, platform only), status and payment ("" = any).
func (s *Service) List(ctx context.Context, merchantID, status, paymentID string, limit int) ([]Refund, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT `+cols+` FROM refunds WHERE ($1 = '' OR merchant_id::text = $1) AND ($2 = '' OR status = $2)
		AND ($3 = '' OR payment_id::text = $3) ORDER BY created_at DESC, id LIMIT $4`, merchantID, status, paymentID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Refund, error) { return scan(r) })
}

type locked struct {
	Refund
	approvedBy string
}

func lock(ctx context.Context, tx pgx.Tx, id, merchantID string) (locked, error) {
	var l locked
	err := tx.QueryRow(ctx, `SELECT `+cols+`, COALESCE(approved_by::text,'') FROM refunds
		WHERE id::text = $1 AND ($2 = '' OR merchant_id::text = $2) FOR NO KEY UPDATE`, id, merchantID).
		Scan(&l.ID, &l.MerchantID, &l.PaymentID, &l.Amount, &l.Currency, &l.Reason, &l.Status, &l.RequestedBy,
			&l.ApprovedAt, &l.DecisionNote, &l.BankReference, &l.RefundedAt, &l.FailureReason, &l.CreatedAt, &l.approvedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// notify queues the merchant webhook for a final refund status, in the same transaction.
func notify(ctx context.Context, tx pgx.Tx, r Refund, to string) error {
	return outbound.EnqueueTx(ctx, tx, r.MerchantID, "refund."+to, r.ID, map[string]any{"refund": map[string]any{
		"id": r.ID, "payment_id": r.PaymentID, "amount": r.Amount, "currency": r.Currency, "status": to}})
}

// end finishes a refund that did not happen (cancel / reject / failed) and releases the reservation.
func (s *Service) end(ctx context.Context, id, merchantID string, from []string, to, note string, a Actor, action string) (Refund, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Refund{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	l, err := lock(ctx, tx, id, merchantID)
	if err != nil {
		return Refund{}, err
	}
	if l.Status == to {
		return l.Refund, nil
	}
	if !contains(from, l.Status) {
		return Refund{}, ErrState
	}
	col := "decision_note"
	if to == "failed" {
		col = "failure_reason"
	}
	if _, err := tx.Exec(ctx, `UPDATE refunds SET status = $2, `+col+` = NULLIF($3,''), updated_at = now(), version = version + 1 WHERE id = $1`, id, to, note); err != nil {
		return Refund{}, err
	}
	if _, err := ledger.PostTx(ctx, tx, journal("refund_released", id, l.MerchantID, l.Currency, l.Amount,
		ledger.RefundPayable, ledger.MerchantPayable)); err != nil {
		return Refund{}, err
	}
	if err := notify(ctx, tx, l.Refund, to); err != nil {
		return Refund{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: action, EntityType: "refund", EntityID: id, RequestID: a.RequestID,
		Metadata: map[string]any{"merchant_id": l.MerchantID, "amount": l.Amount, "from": l.Status}}); err != nil {
		return Refund{}, err
	}
	out, err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM refunds WHERE id = $1`, id))
	if err != nil {
		return Refund{}, err
	}
	return out, tx.Commit(ctx)
}

func (s *Service) Cancel(ctx context.Context, merchantID, id string, a Actor) (Refund, error) {
	return s.end(ctx, id, merchantID, []string{"pending_approval"}, "cancelled", "cancelled by merchant", a, "refund.cancel")
}

func (s *Service) Reject(ctx context.Context, id, note string, a Actor) (Refund, error) {
	if len(note) < 3 || len(note) > 500 {
		return Refund{}, fmt.Errorf("%w: note (3-500 chars) is required", ErrValidation)
	}
	return s.end(ctx, id, "", []string{"pending_approval"}, "cancelled", note, a, "refund.reject")
}

func (s *Service) MarkFailed(ctx context.Context, id, reason string, a Actor) (Refund, error) {
	if len(reason) < 3 || len(reason) > 500 {
		return Refund{}, fmt.Errorf("%w: reason (3-500 chars) is required", ErrValidation)
	}
	return s.end(ctx, id, "", []string{"approved"}, "failed", reason, a, "refund.mark_failed")
}

func (s *Service) Approve(ctx context.Context, id string, a Actor) (Refund, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Refund{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	l, err := lock(ctx, tx, id, "")
	if err != nil {
		return Refund{}, err
	}
	if l.Status == "approved" && l.approvedBy == a.UserID {
		return l.Refund, nil
	}
	if l.Status != "pending_approval" {
		return Refund{}, ErrState
	}
	if l.RequestedBy == a.UserID {
		return Refund{}, ErrSelfApproval
	}
	if _, err := tx.Exec(ctx, `UPDATE refunds SET status = 'approved', approved_by = $2::uuid, approved_at = now(), updated_at = now(), version = version + 1
		WHERE id = $1`, id, a.UserID); err != nil {
		return Refund{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "refund.approve", EntityType: "refund", EntityID: id, RequestID: a.RequestID,
		Metadata: map[string]any{"merchant_id": l.MerchantID, "amount": l.Amount}}); err != nil {
		return Refund{}, err
	}
	out, err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM refunds WHERE id = $1`, id))
	if err != nil {
		return Refund{}, err
	}
	return out, tx.Commit(ctx)
}

// MarkRefunded records that Finance returned the money to the customer. The reserved liability leaves
// the books once (dedup key + status guard); repeating it with the same reference is a no-op.
func (s *Service) MarkRefunded(ctx context.Context, id, bankReference string, a Actor) (Refund, error) {
	if len(bankReference) < 3 || len(bankReference) > 128 {
		return Refund{}, fmt.Errorf("%w: bank_reference (3-128 chars) is required", ErrValidation)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Refund{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	l, err := lock(ctx, tx, id, "")
	if err != nil {
		return Refund{}, err
	}
	if l.Status == "refunded" {
		if l.BankReference != nil && *l.BankReference == bankReference {
			return l.Refund, nil
		}
		return Refund{}, ErrRefConflict
	}
	if l.Status != "approved" {
		return Refund{}, ErrState
	}
	if _, err := tx.Exec(ctx, `UPDATE refunds SET status = 'refunded', bank_reference = $2, executed_by = $3::uuid, refunded_at = now(),
		updated_at = now(), version = version + 1 WHERE id = $1`, id, bankReference, a.UserID); err != nil {
		return Refund{}, err
	}
	if _, err := ledger.PostTx(ctx, tx, journal("refund_paid", id, l.MerchantID, l.Currency, l.Amount,
		ledger.RefundPayable, ledger.CashBankPT)); err != nil {
		return Refund{}, err
	}
	if err := notify(ctx, tx, l.Refund, "refunded"); err != nil {
		return Refund{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "refund.mark_refunded", EntityType: "refund", EntityID: id, RequestID: a.RequestID,
		Metadata: map[string]any{"merchant_id": l.MerchantID, "amount": l.Amount}}); err != nil {
		return Refund{}, err
	}
	out, err := scan(tx.QueryRow(ctx, `SELECT `+cols+` FROM refunds WHERE id = $1`, id))
	if err != nil {
		return Refund{}, err
	}
	return out, tx.Commit(ctx)
}
