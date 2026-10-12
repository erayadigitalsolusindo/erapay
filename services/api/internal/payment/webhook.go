package payment

import (
	"context"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ApplyWebhook applies an AUTHENTICATED notification (the caller has verified the signature).
// It is safe to call any number of times, concurrently, with the same event: Transition
// serializes on the payment row and the dedup key makes a repeat a no-op (PRD §0 #5).
//
// Nothing changes when: the reference is not one of our payments of this gateway
// (ErrNotFound), the status has no confirmed mapping, or a "success" reports a different
// amount/currency (ErrAmountMismatch, logged for review).
func (o *Orchestrator) ApplyWebhook(ctx context.Context, gw string, ev gateway.WebhookEvent) (Result, error) {
	if !uuidRe.MatchString(ev.PaymentRef) {
		return Result{}, ErrNotFound
	}
	var pgw, currency string
	var amount int64
	err := o.db.QueryRow(ctx, `SELECT gateway, currency, amount FROM payments WHERE id = $1`, ev.PaymentRef).
		Scan(&pgw, &currency, &amount)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && pgw != gw) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	if !ev.StatusOK {
		o.log.Warn("webhook status has no confirmed mapping; payment unchanged",
			"payment_id", ev.PaymentRef, "raw_status", ev.RawStatus)
		return Result{}, nil
	}
	if ev.Status == gateway.StatusSucceeded && (ev.Amount != amount || ev.Currency != currency) {
		o.log.Error("webhook reports success with a different amount; NOT applied",
			"payment_id", ev.PaymentRef, "expected", amount, "got", ev.Amount)
		return Result{}, ErrAmountMismatch
	}
	return o.svc.Transition(ctx, TransitionInput{
		PaymentID: ev.PaymentRef, To: Status(ev.Status), Source: SourceWebhook,
		RawGatewayStatus: ev.RawStatus, Reason: "webhook", DedupKey: ev.DedupKey})
}
