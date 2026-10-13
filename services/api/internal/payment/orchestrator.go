package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
)

var (
	ErrMethodUnavailable = errors.New("payment: payment method unavailable for this merchant")
	ErrAmountMismatch    = errors.New("payment: gateway amount differs from the payment amount")
)

const (
	OpCreate        = "payment.create"
	DefaultQRTTL    = 15 * time.Minute
	MaxQRTTL        = 24 * time.Hour
	staleCreatedAge = 2 * time.Minute
)

// Gateways is satisfied by *gateway.Registry; the domain never sees a concrete gateway.
type Gateways interface {
	Get(code string) (gateway.Adapter, bool)
}

// Orchestrator runs the payment flows that touch a gateway. Money rules it enforces:
//   - a gateway timeout/ambiguity leaves the payment `unknown` (never `failed`) and
//     create is never retried blindly (PRD §0 #3);
//   - final status comes only from the gateway's own answer (create reject, inquiry) (#2);
//   - the Idempotency-Key makes a retried POST return the same payment (#4).
type Orchestrator struct {
	svc  *Service
	db   *pgxpool.Pool
	idem *idempotency.Store
	gws  Gateways
	env  string // "sandbox" | "production": selects merchant_gateway_accounts rows
	log  *slog.Logger
}

func NewOrchestrator(svc *Service, db *pgxpool.Pool, idem *idempotency.Store, gws Gateways, env string, log *slog.Logger) *Orchestrator {
	return &Orchestrator{svc: svc, db: db, idem: idem, gws: gws, env: env, log: log}
}

type CreateRequest struct {
	MerchantID        string // from the authenticated principal only
	OutletID          string
	MerchantReference string
	Description       string
	Amount            int64
	Currency          string
	PaymentMethod     string
	ExpiresAt         *time.Time
	IdempotencyKey    string
	RequestHash       []byte // idempotency.HashJSON of the canonical request
}

type Outcome struct {
	Payment  Payment
	Replayed bool
}

// route picks the merchant's active gateway account (MVP: one gateway per merchant+method).
func (o *Orchestrator) route(ctx context.Context, merchantID, method string) (gateway.Adapter, string, error) {
	var code, ref string
	err := o.db.QueryRow(ctx, `
		SELECT gateway, gateway_ref FROM merchant_gateway_accounts
		WHERE merchant_id = $1 AND environment = $2 AND activation_status = 'active' AND gateway_ref IS NOT NULL
		ORDER BY created_at LIMIT 1`, merchantID, o.env).Scan(&code, &ref)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrMethodUnavailable
	}
	if err != nil {
		return nil, "", err
	}
	ad, ok := o.gws.Get(code)
	if !ok || !ad.Capabilities().SupportsMethod(method) {
		return nil, "", ErrMethodUnavailable
	}
	return ad, ref, nil
}

// Create implements POST /v1/payments. See the type comment for the guarantees.
func (o *Orchestrator) Create(ctx context.Context, r CreateRequest) (Outcome, error) {
	if !idempotency.ValidKey(r.IdempotencyKey) {
		return Outcome{}, fmt.Errorf("%w: Idempotency-Key header is required (8-128 chars)", ErrValidation)
	}
	ad, accountRef, err := o.route(ctx, r.MerchantID, r.PaymentMethod)
	if err != nil {
		return Outcome{}, err
	}
	sc := idempotency.Scope{MerchantID: r.MerchantID, Operation: OpCreate, Key: r.IdempotencyKey}
	prev, err := o.idem.Reserve(ctx, sc, r.RequestHash)
	if err != nil {
		return Outcome{}, err // ErrInProgress / ErrConflict / infrastructure
	}
	if prev != nil {
		return o.replay(ctx, r.MerchantID, prev)
	}

	// From here the claim is ours. Everything after the gateway call must survive client
	// disconnects, so it runs on a context that is not cancelled with the request.
	work := context.WithoutCancel(ctx)

	expires := time.Now().Add(DefaultQRTTL)
	if r.ExpiresAt != nil {
		expires = *r.ExpiresAt
	}
	if expires.After(time.Now().Add(MaxQRTTL)) {
		o.fail(work, sc, ErrValidation)
		return Outcome{}, fmt.Errorf("%w: expires_at too far in the future", ErrValidation)
	}

	p, err := o.createRow(work, CreateInput{MerchantID: r.MerchantID, OutletID: r.OutletID, MerchantReference: r.MerchantReference,
		Description: r.Description, Amount: r.Amount, Currency: r.Currency, PaymentMethod: r.PaymentMethod,
		Gateway: ad.Code(), ExpiresAt: &expires})
	if err != nil {
		o.fail(work, sc, err)
		return Outcome{}, err
	}

	gctx, cancel := context.WithTimeout(work, 30*time.Second)
	defer cancel()
	start := time.Now()
	res, gerr := ad.CreatePayment(gctx, gateway.CreateRequest{Reference: p.ID, AccountRef: accountRef,
		Method: r.PaymentMethod, Amount: p.Amount, Currency: p.Currency, ExpiresAt: expires, Description: p.Description})
	lat := int(time.Since(start).Milliseconds())

	switch {
	case gerr == nil:
		o.attempt(work, p.ID, ad.Code(), "create", "ok", 0, res.ResponseCode, res.RawStatus, lat)
		o.mustTransition(work, TransitionInput{PaymentID: p.ID, To: Pending, Source: SourceGatewayResponse,
			GatewayReference: res.GatewayReference, QRPayload: res.QRPayload, RawGatewayStatus: res.RawStatus, Reason: "gateway accepted"})
	case gateway.IsRejected(gerr):
		var ge *gateway.Error
		errors.As(gerr, &ge)
		o.attempt(work, p.ID, ad.Code(), "create", "rejected", ge.HTTPStatus, ge.Code, "", lat)
		o.log.Warn("gateway rejected create", "payment_id", p.ID, "http", ge.HTTPStatus, "code", ge.Code, "message", ge.Message)
		o.mustTransition(work, TransitionInput{PaymentID: p.ID, To: Failed, Source: SourceGatewayResponse, Reason: "gateway rejected: " + ge.Code})
	default:
		// Timeout, 5xx, 409, pending... the gateway may hold a QR we never saw: UNKNOWN, not failed.
		var ge gateway.Error
		if e := new(gateway.Error); errors.As(gerr, &e) {
			ge = *e
		}
		o.attempt(work, p.ID, ad.Code(), "create", "ambiguous", ge.HTTPStatus, ge.Code, "", lat)
		o.mustTransition(work, TransitionInput{PaymentID: p.ID, To: Unknown, Source: SourceInternal, Reason: "gateway outcome ambiguous on create"})
	}

	final, err := o.svc.Get(work, r.MerchantID, p.ID)
	if err != nil {
		return Outcome{}, err
	}
	resp := idempotency.Response{Status: 201, ResourceType: "payment", ResourceID: p.ID}
	for i := 0; i < 3; i++ {
		if err = o.idem.Complete(work, sc, resp); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		o.log.Error("idempotency complete failed; record stays in_progress for recovery", "payment_id", p.ID, "err", err)
	}
	return Outcome{Payment: final}, nil
}

func (o *Orchestrator) createRow(ctx context.Context, in CreateInput) (Payment, error) {
	tx, err := o.db.Begin(ctx)
	if err != nil {
		return Payment{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	p, err := o.svc.CreateTx(ctx, tx, in)
	if err != nil {
		return Payment{}, err
	}
	return p, tx.Commit(ctx)
}

// fail resolves the idempotency claim after a failure that happened BEFORE any gateway
// call: deterministic rejections are stored (so a retry gets the same answer),
// infrastructure errors release the key so the client can retry safely.
func (o *Orchestrator) fail(ctx context.Context, sc idempotency.Scope, err error) {
	var code string
	switch {
	case errors.Is(err, ErrValidation):
		code = "validation"
	case errors.Is(err, ErrNoPricing):
		code = "no_pricing"
	case errors.Is(err, ErrMerchantNotActive):
		code = "merchant_not_active"
	default:
		if rerr := o.idem.Release(ctx, sc); rerr != nil {
			o.log.Error("idempotency release failed", "err", rerr)
		}
		return
	}
	body, _ := json.Marshal(map[string]string{"code": code})
	if cerr := o.idem.Complete(ctx, sc, idempotency.Response{Status: 422, Body: body}); cerr != nil {
		o.log.Error("idempotency complete (failure) failed", "err", cerr)
	}
}

func (o *Orchestrator) replay(ctx context.Context, merchantID string, prev *idempotency.Response) (Outcome, error) {
	if prev.ResourceID == "" { // a stored pre-gateway failure
		var b struct{ Code string }
		_ = json.Unmarshal(prev.Body, &b)
		switch b.Code {
		case "no_pricing":
			return Outcome{}, ErrNoPricing
		case "merchant_not_active":
			return Outcome{}, ErrMerchantNotActive
		}
		return Outcome{}, fmt.Errorf("%w: the original request was rejected", ErrValidation)
	}
	p, err := o.svc.Get(ctx, merchantID, prev.ResourceID)
	return Outcome{Payment: p, Replayed: true}, err
}

// mustTransition applies a status change that mirrors an already-happened gateway event.
// It retries transient DB errors; if it still fails the payment stays `created` and is
// picked up by SweepStaleCreated (-> unknown) rather than being lost silently.
func (o *Orchestrator) mustTransition(ctx context.Context, in TransitionInput) {
	var err error
	for i := 0; i < 3; i++ {
		if _, err = o.svc.Transition(ctx, in); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	o.log.Error("could not record gateway outcome", "payment_id", in.PaymentID, "to", in.To, "err", err)
}

func (o *Orchestrator) attempt(ctx context.Context, paymentID, gw, op, outcome string, httpStatus int, code, raw string, latencyMS int) {
	if _, err := o.db.Exec(ctx, `
		INSERT INTO payment_attempts (payment_id, gateway, operation, outcome, http_status, gateway_code, raw_status, latency_ms)
		VALUES ($1,$2,$3,$4,NULLIF($5,0),NULLIF($6,''),NULLIF($7,''),$8)`,
		paymentID, gw, op, outcome, httpStatus, code, raw, latencyMS); err != nil {
		o.log.Error("record payment attempt", "payment_id", paymentID, "err", err)
	}
}

// Inquire asks the gateway for the real status of a pending/unknown payment and applies
// the result with source=inquiry. If the status has no confirmed mapping, or a "success"
// reports a different amount, nothing changes and the attempt is recorded for review.
func (o *Orchestrator) Inquire(ctx context.Context, paymentID string) error {
	var merchantID, gw, gref, currency, status, acct string
	var amount int64
	err := o.db.QueryRow(ctx, `
		SELECT p.merchant_id::text, p.gateway, COALESCE(p.gateway_reference,''), p.currency, p.status, p.amount, a.gateway_ref
		FROM payments p JOIN merchant_gateway_accounts a
		  ON a.merchant_id = p.merchant_id AND a.gateway = p.gateway AND a.environment = $2
		WHERE p.id = $1`, paymentID, o.env).Scan(&merchantID, &gw, &gref, &currency, &status, &amount, &acct)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if (Status(status) != Pending && Status(status) != Unknown) || gref == "" {
		return nil
	}
	ad, ok := o.gws.Get(gw)
	if !ok || !ad.Capabilities().Inquiry {
		return gateway.ErrNotSupported
	}

	start := time.Now()
	res, gerr := ad.GetPayment(gctx, gateway.QueryRequest{Reference: paymentID, GatewayReference: gref, AccountRef: acct})
	lat := time.Since(start)
	if gerr != nil {
		var ge gateway.Error
		if e := new(gateway.Error); errors.As(gerr, &e) {
			ge = *e
		}
		out := "rejected"
		if gateway.IsAmbiguous(gerr) {
			out = "ambiguous"
		}
		observability.ObserveGateway(gw, "inquiry", out, lat)
		o.attempt(ctx, paymentID, gw, "inquiry", out, ge.HTTPStatus, ge.Code, "", int(lat.Milliseconds()))
		if gateway.IsAmbiguous(gerr) {
			return nil // try again later; the poller backs off
		}
		return gerr
	}

	to, mapped := ad.MapStatus(res.RawStatus)
	if !mapped {
		observability.ObserveGateway(gw, "inquiry", "unmapped", lat)
		o.attempt(ctx, paymentID, gw, "inquiry", "unmapped", 0, res.ResponseCode, res.RawStatus, int(lat.Milliseconds()))
		o.log.Warn("inquiry status has no confirmed mapping; payment unchanged", "payment_id", paymentID, "raw_status", res.RawStatus)
		return nil
	}
	if to == gateway.StatusSucceeded && (res.Amount != amount || (res.Currency != "" && res.Currency != currency)) {
		observability.ObserveGateway(gw, "inquiry", "amount_mismatch", lat)
		o.attempt(ctx, paymentID, gw, "inquiry", "amount_mismatch", 0, res.ResponseCode, res.RawStatus, int(lat.Milliseconds()))
		o.log.Error("gateway reports success with a different amount; NOT applied", "payment_id", paymentID,
			"expected", amount, "got", res.Amount)
		return ErrAmountMismatch
	}
	observability.ObserveGateway(gw, "inquiry", "ok", lat)
	o.attempt(ctx, paymentID, gw, "inquiry", "ok", 0, res.ResponseCode, res.RawStatus, int(lat.Milliseconds()))
	_, err = o.svc.Transition(ctx, TransitionInput{PaymentID: paymentID, To: Status(to), Source: SourceInquiry,
		RawGatewayStatus: res.RawStatus, Reason: "inquiry"})
	return err
}

// PollOnce claims due pending/unknown payments (SKIP LOCKED: safe with several instances),
// schedules their next inquiry with exponential backoff, and inquires each.
// The first inquiry is due 60s after the payment becomes pending (doku.md §5).
func (o *Orchestrator) PollOnce(ctx context.Context, limit int) (int, error) {
	rows, err := o.db.Query(ctx, `
		UPDATE payments SET inquiry_attempts = inquiry_attempts + 1,
		    next_inquiry_at = now() + LEAST(interval '30 seconds' * power(2, LEAST(inquiry_attempts, 10)), interval '10 minutes')
		WHERE id IN (
		    SELECT id FROM payments
		    WHERE status IN ('pending','unknown') AND gateway_reference IS NOT NULL AND next_inquiry_at <= now()
		    ORDER BY next_inquiry_at LIMIT $1 FOR UPDATE SKIP LOCKED)
		RETURNING id::text`, limit)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := o.Inquire(ctx, id); err != nil {
			o.log.Warn("inquiry failed", "payment_id", id, "err", err)
		}
	}
	return len(ids), nil
}

// SweepStaleCreated moves payments stuck in `created` (a crash between insert and the
// gateway result) to `unknown`. They have no gateway reference, so they cannot be queried:
// they surface for manual/reconciliation follow-up instead of being retried blindly.
func (o *Orchestrator) SweepStaleCreated(ctx context.Context, limit int) (int, error) {
	rows, err := o.db.Query(ctx, `SELECT id::text FROM payments
		WHERE status = 'created' AND created_at < now() - $1::interval ORDER BY created_at LIMIT $2`,
		fmt.Sprintf("%d seconds", int(staleCreatedAge.Seconds())), limit)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if _, err := o.svc.Transition(ctx, TransitionInput{PaymentID: id, To: Unknown, Source: SourceInternal,
			Reason: "stale in created: gateway outcome unknown"}); err == nil {
			n++
		}
	}
	return n, nil
}

// RunPoller runs PollOnce and SweepStaleCreated until ctx is cancelled.
func (o *Orchestrator) RunPoller(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := o.PollOnce(ctx, 20); err != nil && ctx.Err() == nil {
				o.log.Error("poll", "err", err)
			}
			if _, err := o.SweepStaleCreated(ctx, 20); err != nil && ctx.Err() == nil {
				o.log.Error("sweep", "err", err)
			}
		}
	}
}
