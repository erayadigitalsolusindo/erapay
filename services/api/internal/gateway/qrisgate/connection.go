package qrisgate

import (
	"context"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var _ gateway.ConnectionTester = (*Adapter)(nil)

// TestConnection implements gateway.ConnectionTester. QRISGate is EraPay's own gateway: there is no upstream and no
// server-side credential, so deep=false only confirms that. With deep=true accountRef (= the merchant id) is checked the
// way a real payment depends on it: static QRIS saved, at least one listener package, listener secret that signs and
// verifies, and the gateway account activated by the platform. Nothing is written and no QR is claimed.
func (a *Adapter) TestConnection(ctx context.Context, accountRef string, deep bool) []gateway.ConnectionStep {
	if !deep {
		return []gateway.ConnectionStep{{Name: "credentials", OK: true, Message: "own gateway: no external credentials to check"}}
	}
	if !uuidRe.MatchString(accountRef) {
		return []gateway.ConnectionStep{{Name: "account", Code: "bad_merchant", Message: "invalid merchant id"}}
	}
	acc, err := a.cfg.Port.Account(ctx, accountRef)
	if err != nil {
		return []gateway.ConnectionStep{{Name: "account", Code: "unavailable", Message: "could not read the merchant's QRISGate account"}}
	}
	steps := []gateway.ConnectionStep{
		step("qris", acc.HasQRIS, "no_qris", "the merchant has not saved a valid static QRIS"),
		step("packages", len(acc.Packages) > 0, "no_packages", "no listener app package is registered"),
	}
	steps = append(steps, a.TestWebhook(ctx, accountRef)...)
	return append(steps, step("activation", acc.ActivationStatus == "active", "not_active", "the gateway is not activated for this merchant (status: "+orNone(acc.ActivationStatus)+")"))
}

func step(name string, ok bool, code, msg string) gateway.ConnectionStep {
	if ok {
		return gateway.ConnectionStep{Name: name, OK: true}
	}
	return gateway.ConnectionStep{Name: name, Code: code, Message: msg}
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
