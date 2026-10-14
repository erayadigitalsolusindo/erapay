package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/ledger"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/outbound"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/pricing"
)

var (
	ErrNotFound          = errors.New("payment: not found")
	ErrValidation        = errors.New("payment: validation failed")
	ErrMerchantNotActive = errors.New("payment: merchant not active")
	ErrNoPricing         = errors.New("payment: no pricing plan for merchant/gateway/method")
)

// SupportedCurrencies is the allowlist; IDR is the only currency at launch.
var SupportedCurrencies = map[string]bool{"IDR": true}

type Payment struct {
	ID                string     `json:"id"`
	MerchantID        string     `json:"-"`
	OutletID          string     `json:"outlet_id,omitempty"`
	MerchantReference string     `json:"merchant_reference"`
	Description       string     `json:"description,omitempty"`
	Amount            int64      `json:"amount"`
	Currency          string     `json:"currency"`
	PaymentMethod     string     `json:"payment_method"`
	Status            Status     `json:"status"`
	SettlementStatus  string     `json:"settlement_status"` // not_expected | expected | settled; separate from status (PRD §0 #9)
	RefundedAmount    int64      `json:"refunded_amount"`   // refunds already returned to the customer; separate from status
	RefundingAmount   int64      `json:"refunding_amount"`  // refunds requested or approved, not yet returned
	Gateway           string     `json:"gateway"`
	GatewayReference  string     `json:"-"`
	QRPayload         string     `json:"-"`
	QRCode            *QRCode    `json:"qr_code,omitempty"`
	Fee               Fee        `json:"fee"`
	ExpiresAt         *time.Time `json:"expires_at"`
	FinalAt           *time.Time `json:"final_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

type Fee struct {
	Bearer      string `json:"bearer"`
	MDRFee      int64  `json:"mdr_fee"`
	PlatformFee int64  `json:"platform_fee"`
	MerchantNet int64  `json:"merchant_net"`
}

// CreateInput: MerchantID must come from the authenticated principal. Gateway is the
// routing decision made by the caller (merchant gateway config, step 6); fees and
// status are never accepted from the client (PRD §0 #8).
type CreateInput struct {
	MerchantID        string
	OutletID          string
	MerchantReference string
	Description       string
	Amount            int64
	Currency          string
	PaymentMethod     string
	Gateway           string
	ExpiresAt         *time.Time
}

func (in CreateInput) validate(now time.Time) error {
	switch {
	case in.MerchantID == "" || in.Gateway == "" || in.PaymentMethod == "":
		return fmt.Errorf("%w: merchant, gateway and payment_method are required", ErrValidation)
	case in.MerchantReference == "" || len(in.MerchantReference) > 64:
		return fmt.Errorf("%w: merchant_reference must be 1-64 chars", ErrValidation)
	case len(in.Description) > 255:
		return fmt.Errorf("%w: description too long", ErrValidation)
	case !SupportedCurrencies[in.Currency]:
		return fmt.Errorf("%w: unsupported currency", ErrValidation)
	case in.Amount <= 0 || in.Amount > pricing.MaxGross:
		return fmt.Errorf("%w: amount out of range", ErrValidation)
	case in.ExpiresAt != nil && !in.ExpiresAt.After(now):
		return fmt.Errorf("%w: expires_at must be in the future", ErrValidation)
	}
	return nil
}

type Service struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db, now: time.Now} }

const cols = `id::text, merchant_id::text, COALESCE(outlet_id::text,''), merchant_reference, COALESCE(description,''),
	amount, currency, payment_method, status, gateway, COALESCE(gateway_reference,''),
	mdr_fee, platform_fee, merchant_net, expires_at, final_at, created_at, COALESCE(qr_payload,''),
		CASE WHEN status <> 'succeeded' THEN 'not_expected'
		     WHEN EXISTS (SELECT 1 FROM settlement_items si WHERE si.payment_id = payments.id) THEN 'settled'
		     ELSE 'expected' END,
		COALESCE((SELECT SUM(r.amount) FROM refunds r WHERE r.payment_id = payments.id AND r.status = 'refunded'), 0),
		COALESCE((SELECT SUM(r.amount) FROM refunds r WHERE r.payment_id = payments.id AND r.status IN ('pending_approval','approved')), 0)`

func scan(row pgx.Row) (Payment, error) {
	var p Payment
	var st string
	err := row.Scan(&p.ID, &p.MerchantID, &p.OutletID, &p.MerchantReference, &p.Description,
		&p.Amount, &p.Currency, &p.PaymentMethod, &st, &p.Gateway, &p.GatewayReference,
		&p.Fee.MDRFee, &p.Fee.PlatformFee, &p.Fee.MerchantNet, &p.ExpiresAt, &p.FinalAt, &p.CreatedAt, &p.QRPayload, &p.SettlementStatus, &p.RefundedAmount, &p.RefundingAmount)
	p.Status = Status(st)
	p.Fee.Bearer = "merchant"
	if p.Status == Pending && p.QRPayload != "" { // a dynamic QR is only exposed while it can still be paid
		p.QRCode = &QRCode{Type: "provider_defined", Value: p.QRPayload}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// CreateTx inserts a payment in status `created` with its immutable fee snapshot,
// inside the caller's transaction (compose with idempotency.Store.Do so a retried
// request yields the same payment).
func (s *Service) CreateTx(ctx context.Context, tx pgx.Tx, in CreateInput) (Payment, error) {
	if err := in.validate(s.now()); err != nil {
		return Payment{}, err
	}
	var onboarding string
	err := tx.QueryRow(ctx, `SELECT onboarding_status FROM merchants WHERE id = $1`, in.MerchantID).Scan(&onboarding)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && onboarding != "approved") {
		return Payment{}, ErrMerchantNotActive
	}
	if err != nil {
		return Payment{}, err
	}
	if in.OutletID != "" {
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM merchant_outlets WHERE id = $1 AND merchant_id = $2)`,
			in.OutletID, in.MerchantID).Scan(&ok); err != nil {
			return Payment{}, err
		}
		if !ok {
			return Payment{}, fmt.Errorf("%w: unknown outlet", ErrValidation)
		}
	}

	var planID string
	var plan pricing.Plan
	var mdrR, platR string
	err = tx.QueryRow(ctx, `
		SELECT p.id::text, p.version, p.mdr_bps, p.mdr_fixed, p.mdr_rounding,
		       p.platform_bps, p.platform_fixed, p.platform_rounding,
		       p.mdr_cap, p.mdr_cap_up_to
		FROM merchant_pricing_assignments a JOIN pricing_plans p ON p.id = a.plan_id
		WHERE a.merchant_id = $1 AND p.gateway = $2 AND p.payment_method = $3 AND a.effective_from <= $4
		ORDER BY a.effective_from DESC LIMIT 1`,
		in.MerchantID, in.Gateway, in.PaymentMethod, s.now()).
		Scan(&planID, &plan.Version, &plan.MDR.BPS, &plan.MDR.Fixed, &mdrR,
			&plan.Platform.BPS, &plan.Platform.Fixed, &platR, &plan.MDR.Cap, &plan.MDR.CapUpTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, ErrNoPricing
	}
	if err != nil {
		return Payment{}, err
	}
	plan.MDR.Rounding, plan.Platform.Rounding = pricing.Rounding(mdrR), pricing.Rounding(platR)
	b, err := pricing.Compute(in.Amount, in.Currency, plan)
	if err != nil {
		return Payment{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	return scan(tx.QueryRow(ctx, `
		INSERT INTO payments (merchant_id, outlet_id, merchant_reference, description, amount, currency,
		    payment_method, gateway, mdr_fee, platform_fee, merchant_net, pricing_plan_id, plan_version, expires_at)
		VALUES ($1, NULLIF($2,'')::uuid, $3, NULLIF($4,''), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING `+cols,
		in.MerchantID, in.OutletID, in.MerchantReference, in.Description, in.Amount, in.Currency,
		in.PaymentMethod, in.Gateway, b.MDRFee, b.PlatformFee, b.MerchantNet, planID, b.PlanVersion, in.ExpiresAt))
}

// Get is tenant-scoped: a payment of another merchant is indistinguishable from a missing one.
func (s *Service) Get(ctx context.Context, merchantID, id string) (Payment, error) {
	return scan(s.db.QueryRow(ctx, `SELECT `+cols+` FROM payments WHERE id = $1 AND merchant_id = $2`, id, merchantID))
}

type TransitionInput struct {
	PaymentID        string
	To               Status
	Source           Source
	Reason           string
	RawGatewayStatus string // gateway's own status string, stored apart from the normalized one
	GatewayReference string // set once; later values are ignored
	DedupKey         string // e.g. webhook event id: the same report is applied once
	QRPayload        string // provider_defined; stored when first provided
}

type Result struct {
	Status    Status // status after the call
	Applied   bool   // the transition happened
	NoOp      bool   // already in the requested status
	Duplicate bool   // dedup key already processed
	Conflict  bool   // payment already final with a different status; recorded, NOT applied
	From      Status
}

// Transition moves a payment under a row lock, so concurrent reports serialize:
// exactly one final status wins and later conflicting reports are recorded as
// applied=false events for reconciliation (never flipping a terminal status).
// When the payment becomes `succeeded`, the Model B journal is posted in the
// SAME transaction (status change and money movement commit together).
func (s *Service) Transition(ctx context.Context, in TransitionInput) (Result, error) {
	if !SourceMaySet(in.Source, in.To) {
		return Result{}, ErrSourceNotAllowed
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var cur string
	var mid, currency, merchantRef, method string
	var amount, mdr, plat, net int64
	var planVer int
	err = tx.QueryRow(ctx, `
		SELECT status, merchant_id::text, currency, amount, mdr_fee, platform_fee, merchant_net, plan_version,
		       merchant_reference, payment_method
		FROM payments WHERE id = $1 FOR NO KEY UPDATE`, in.PaymentID).
		Scan(&cur, &mid, &currency, &amount, &mdr, &plat, &net, &planVer, &merchantRef, &method)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	from := Status(cur)
	res := Result{Status: from, From: from}

	if in.DedupKey != "" {
		var seen bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_events WHERE dedup_key = $1)`, in.DedupKey).Scan(&seen); err != nil {
			return Result{}, err
		}
		if seen {
			res.Duplicate = true
			return res, nil
		}
	}
	if from == in.To {
		res.NoOp = true
		return res, nil
	}

	event := func(applied bool) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO payment_events (payment_id, from_status, to_status, applied, source, reason, raw_gateway_status, dedup_key)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''))`,
			in.PaymentID, string(from), string(in.To), applied, string(in.Source), in.Reason, in.RawGatewayStatus, in.DedupKey)
		return err
	}

	if from.IsFinal() {
		// Late/conflicting report on a terminal payment: keep the evidence, change nothing.
		if err := event(false); err != nil {
			return Result{}, err
		}
		res.Conflict = true
		return res, tx.Commit(ctx)
	}
	if !CanTransition(from, in.To) {
		return Result{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, in.To)
	}

	var finalAt *time.Time
	if err := tx.QueryRow(ctx, `
		UPDATE payments SET status = $2,
		    gateway_raw_status = COALESCE(NULLIF($3,''), gateway_raw_status),
		    gateway_reference = COALESCE(gateway_reference, NULLIF($4,'')),
		    qr_payload = COALESCE(qr_payload, NULLIF($6,'')),
		    next_inquiry_at = CASE WHEN $2 IN ('pending','unknown') THEN now() + interval '60 seconds' ELSE NULL END,
		    final_at = CASE WHEN $5 THEN now() ELSE final_at END,
		    updated_at = now(), version = version + 1
		WHERE id = $1 RETURNING final_at`,
		in.PaymentID, string(in.To), in.RawGatewayStatus, in.GatewayReference, in.To.IsFinal(), in.QRPayload).Scan(&finalAt); err != nil {
		return Result{}, err
	}
	if err := event(true); err != nil {
		return Result{}, err
	}
	if in.To == Succeeded {
		j, err := ledger.PaymentSucceeded(mid, in.PaymentID, pricing.Breakdown{
			Currency: currency, Gross: amount, MDRFee: mdr, PlatformFee: plat, MerchantNet: net, PlanVersion: planVer})
		if err != nil {
			return Result{}, err
		}
		if _, err := ledger.PostTx(ctx, tx, j); err != nil {
			return Result{}, err
		}
	}
	if in.To.IsFinal() {
		// Same transaction as the status change: a committed final status always has its merchant
		// notification queued, and a rolled-back one never leaks an event (transactional outbox).
		if err := outbound.EnqueueTx(ctx, tx, mid, "payment."+string(in.To), in.PaymentID, map[string]any{
			"payment": map[string]any{
				"id": in.PaymentID, "merchant_reference": merchantRef, "amount": amount, "currency": currency,
				"payment_method": method, "status": string(in.To), "final_at": finalAt,
				"fee": map[string]any{"bearer": "merchant", "mdr_fee": mdr, "platform_fee": plat, "merchant_net": net},
			}}); err != nil {
			return Result{}, err
		}
	}
	res.Status, res.Applied = in.To, true
	return res, tx.Commit(ctx)
}

// QRCode is the gateway-defined payload; the format is opaque to EraPay.
type QRCode struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
