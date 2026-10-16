package doku

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// testAmount is the nominal of the throw-away QR used by the deep connection test. The QR is
// never shown to a payer and is expired right after it is created, so no money moves.
const testAmount = 1000

var _ gateway.ConnectionTester = (*Adapter)(nil)

// step turns a gateway error into an operator-safe connection step. gateway.Error messages are
// already sanitized (see gateway.Error); anything else is reported generically.
func step(name string, err error) gateway.ConnectionStep {
	if err == nil {
		return gateway.ConnectionStep{Name: name, OK: true}
	}
	var ge *gateway.Error
	if errors.As(err, &ge) {
		return gateway.ConnectionStep{Name: name, Code: ge.Code, Message: ge.Error()}
	}
	return gateway.ConnectionStep{Name: name, Message: "unexpected error"}
}

// TestConnection implements gateway.ConnectionTester.
//
//	credentials: a fresh B2B access token (checks base URL, client id, private key signature).
//	merchant (deep only): generate a QR for accountRef with this adapter's terminal id and postal
//	code, which is exactly what fails with 5004701 when the merchant id is wrong. The QR is
//	expired immediately; a cleanup failure is reported but does not fail the test.
func (a *Adapter) TestConnection(ctx context.Context, accountRef string, deep bool) []gateway.ConnectionStep {
	_, err := a.accessToken(ctx, true)
	steps := []gateway.ConnectionStep{step("credentials", err)}
	if err != nil || !deep {
		return steps
	}
	if accountRef == "" {
		return append(steps, gateway.ConnectionStep{Name: "merchant", Message: "account reference is empty"})
	}
	ref := "ERAPAYTEST" + strconv.FormatInt(a.cfg.Now().UnixMilli(), 10)
	res, err := a.CreatePayment(ctx, gateway.CreateRequest{
		Reference: ref, AccountRef: accountRef, Method: "QRIS", Amount: testAmount, Currency: "IDR",
		ExpiresAt: a.cfg.Now().Add(5 * time.Minute), Description: "connection test",
	})
	steps = append(steps, step("merchant", err))
	if err != nil {
		return steps
	}
	cerr := a.CancelPayment(ctx, gateway.CancelRequest{Reference: ref, GatewayReference: res.GatewayReference, AccountRef: accountRef})
	cs := step("cleanup", cerr)
	if cerr != nil {
		cs.Message = "test QR could not be expired and will lapse on its own in 5 minutes: " + cs.Message
		cs.OK = true // informational: the test itself passed
	}
	return append(steps, cs)
}
