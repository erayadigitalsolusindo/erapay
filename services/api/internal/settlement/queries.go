package settlement

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// Balance is the merchant split derived from records, never stored (PRD §0 #14b):
//
//	payable   = ledger merchant_payable (what EraPay owes the merchant)
//	pending   = merchant_net of succeeded payments not yet settled by the gateway
//	available = payable - pending  (payout-eligible; only settlement-confirmed funds)
//
// in_payout = funds reserved for payouts not yet paid out (ledger payout_in_transit);
// on_hold = funds reserved for refunds not yet returned to the customer (ledger refund_payable).
type Balance struct {
	Currency  string `json:"currency"`
	Payable   int64  `json:"payable"`
	Pending   int64  `json:"pending"`
	Available int64  `json:"available"`
	InPayout  int64  `json:"in_payout"`
	OnHold    int64  `json:"on_hold"`
}

// Querier is satisfied by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Balance reads all figures in one statement so they come from a single snapshot.
func (s *Service) Balance(ctx context.Context, merchantID, currency string) (Balance, error) {
	return BalanceOf(ctx, s.db, merchantID, currency)
}

// BalanceOf is Balance on any Querier, so a payout request can read it inside its own transaction.
func BalanceOf(ctx context.Context, q Querier, merchantID, currency string) (Balance, error) {
	b := Balance{Currency: currency}
	err := q.QueryRow(ctx, `
		SELECT
		  (SELECT -COALESCE(SUM(debit_minus_credit), 0) FROM ledger_account_balances
		    WHERE merchant_id = $1 AND currency = $2 AND code = 'merchant_payable'),
		  (SELECT COALESCE(SUM(p.merchant_net), 0) FROM payments p
		    WHERE p.merchant_id = $1 AND p.currency = $2 AND p.status = 'succeeded'
		      AND NOT EXISTS (SELECT 1 FROM settlement_items si WHERE si.payment_id = p.id)),
		  (SELECT -COALESCE(SUM(debit_minus_credit), 0) FROM ledger_account_balances
		    WHERE merchant_id = $1 AND currency = $2 AND code = 'payout_in_transit'),
		  (SELECT -COALESCE(SUM(debit_minus_credit), 0) FROM ledger_account_balances
		    WHERE merchant_id = $1 AND currency = $2 AND code = 'refund_payable')`,
		merchantID, currency).Scan(&b.Payable, &b.Pending, &b.InPayout, &b.OnHold)
	b.Available = b.Payable - b.Pending
	return b, err
}

type Unsettled struct {
	PaymentID        string    `json:"payment_id"`
	MerchantID       string    `json:"merchant_id"`
	Gateway          string    `json:"gateway"`
	GatewayReference string    `json:"gateway_reference"`
	Currency         string    `json:"currency"`
	Gross            int64     `json:"gross"`
	MDRFee           int64     `json:"mdr_fee"`
	ExpectedNet      int64     `json:"expected_net"`
	FinalAt          time.Time `json:"final_at"`
}

// ListUnsettled shows Finance which succeeded payments still await a settlement batch.
func (s *Service) ListUnsettled(ctx context.Context, gateway, currency string, limit int) ([]Unsettled, error) {
	if limit < 1 || limit > MaxBatchPayments {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `
		SELECT p.id::text, p.merchant_id::text, p.gateway, COALESCE(p.gateway_reference,''), p.currency,
		       p.amount, p.mdr_fee, p.amount - p.mdr_fee, p.final_at
		FROM payments p
		WHERE p.status = 'succeeded' AND ($1 = '' OR p.gateway = $1) AND ($2 = '' OR p.currency = $2)
		  AND NOT EXISTS (SELECT 1 FROM settlement_items si WHERE si.payment_id = p.id)
		ORDER BY p.final_at, p.id LIMIT $3`, gateway, currency, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Unsettled, error) {
		var u Unsettled
		return u, r.Scan(&u.PaymentID, &u.MerchantID, &u.Gateway, &u.GatewayReference, &u.Currency, &u.Gross, &u.MDRFee, &u.ExpectedNet, &u.FinalAt)
	})
}

func (s *Service) ListBatches(ctx context.Context, gateway string, limit int) ([]Batch, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, gateway, batch_ref, currency, to_char(settlement_date,'YYYY-MM-DD'), reported_net, expected_net, payment_count, created_at
		FROM settlement_batches WHERE ($1 = '' OR gateway = $1) ORDER BY created_at DESC, id LIMIT $2`, gateway, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Batch, error) {
		var b Batch
		return b, r.Scan(&b.ID, &b.Gateway, &b.BatchRef, &b.Currency, &b.SettlementDate, &b.ReportedNet, &b.ExpectedNet, &b.PaymentCount, &b.CreatedAt)
	})
}

type Exception struct {
	ID             string         `json:"id"`
	Kind           string         `json:"kind"`
	Gateway        string         `json:"gateway"`
	Reference      string         `json:"reference"`
	Details        map[string]any `json:"details"`
	Status         string         `json:"status"`
	ResolutionNote *string        `json:"resolution_note"`
	CreatedAt      time.Time      `json:"created_at"`
	ResolvedAt     *time.Time     `json:"resolved_at"`
}

func (s *Service) ListExceptions(ctx context.Context, status string, limit int) ([]Exception, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, kind, gateway, reference, details, status, resolution_note, created_at, resolved_at
		FROM reconciliation_exceptions WHERE ($1 = '' OR status = $1) ORDER BY created_at DESC, id LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Exception, error) {
		var e Exception
		return e, r.Scan(&e.ID, &e.Kind, &e.Gateway, &e.Reference, &e.Details, &e.Status, &e.ResolutionNote, &e.CreatedAt, &e.ResolvedAt)
	})
}

// ResolveException closes an exception with a mandatory note. It changes no balance: any money
// correction is a separate, audited ledger action (reversal/adjustment).
func (s *Service) ResolveException(ctx context.Context, id, note string, a Actor) error {
	if l := len(note); l < 3 || l > 1000 {
		return ErrValidation
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, `
		UPDATE reconciliation_exceptions SET status = 'resolved', resolution_note = $2, resolved_by = NULLIF($3,'')::uuid, resolved_at = now()
		WHERE id = $1 AND status = 'open'`, id, note, a.UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "reconciliation_exception.resolve",
		EntityType: "reconciliation_exception", EntityID: id, RequestID: a.RequestID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
