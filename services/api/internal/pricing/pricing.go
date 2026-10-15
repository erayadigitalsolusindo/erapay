// Package pricing computes the fee breakdown of a payment (PRD pricing.md).
// All money is integer minor units; rates are basis points (1 bps = 0.01%).
package pricing

import (
	"errors"
	"fmt"
)

// MaxGross bounds gross so gross*bps cannot overflow int64.
const MaxGross int64 = 1e13

type Rounding string

const (
	RoundHalfUp Rounding = "half_up"
	RoundFloor  Rounding = "floor"
	RoundCeil   Rounding = "ceil"
)

// Rate is bps of gross plus an optional fixed amount. Both come from the
// versioned plan, never from constants (PRD pricing.md).
type Rate struct {
	BPS     int64
	Fixed   int64
	Cap     int64 // 0 = no cap; otherwise the fee never exceeds this total
	CapUpTo int64 // 0 = the cap applies at any amount; >0 = the cap applies only when gross <= CapUpTo
	// Small-transaction rule (BI): when FlatUpTo > 0 and gross <= FlatUpTo the fee is exactly FlatFee,
	// replacing the percentage, fixed part and cap. FlatUpTo == 0 disables it; FlatFee may be 0 (free).
	FlatUpTo int64
	FlatFee  int64
	Rounding Rounding
}

// Plan is an immutable, versioned tariff. Rounding of each component is
// configurable until DOKU confirms its own rule (OPEN QUESTION, [KD]).
type Plan struct {
	Version  int
	MDR      Rate // gateway fee charged to the merchant
	Platform Rate // EraPay service fee
}

// Breakdown is the immutable snapshot stored on the payment.
// Invariant: Gross == MDRFee + PlatformFee + MerchantNet.
type Breakdown struct {
	Currency    string
	Gross       int64
	MDRFee      int64
	PlatformFee int64
	MerchantNet int64
	PlanVersion int
}

var (
	ErrInvalidAmount = errors.New("pricing: gross must be > 0 and <= MaxGross")
	ErrInvalidRate   = errors.New("pricing: invalid rate")
	ErrFeesExceed    = errors.New("pricing: fees exceed gross")
)

// Compute derives the fees from gross. Any rounding remainder lands in
// MerchantNet (net = gross - mdr - platform), so nothing is ever lost.
func Compute(gross int64, currency string, p Plan) (Breakdown, error) {
	if gross <= 0 || gross > MaxGross {
		return Breakdown{}, ErrInvalidAmount
	}
	if currency == "" {
		return Breakdown{}, fmt.Errorf("%w: currency required", ErrInvalidRate)
	}
	mdr, err := fee(gross, p.MDR)
	if err != nil {
		return Breakdown{}, fmt.Errorf("mdr: %w", err)
	}
	plat, err := fee(gross, p.Platform)
	if err != nil {
		return Breakdown{}, fmt.Errorf("platform: %w", err)
	}
	net := gross - mdr - plat
	if net < 0 {
		return Breakdown{}, ErrFeesExceed
	}
	return Breakdown{
		Currency: currency, Gross: gross, MDRFee: mdr, PlatformFee: plat,
		MerchantNet: net, PlanVersion: p.Version,
	}, nil
}

func fee(gross int64, r Rate) (int64, error) {
	if r.BPS < 0 || r.BPS > 10000 || r.Fixed < 0 || r.Fixed > MaxGross {
		return 0, ErrInvalidRate
	}
	if r.Cap < 0 || r.Cap > MaxGross || r.CapUpTo < 0 || r.CapUpTo > MaxGross {
		return 0, ErrInvalidRate
	}
	if r.CapUpTo > 0 && r.Cap == 0 {
		return 0, fmt.Errorf("%w: cap_up_to %d without a cap", ErrInvalidRate, r.CapUpTo)
	}
	if r.FlatUpTo < 0 || r.FlatUpTo > MaxGross || r.FlatFee < 0 || r.FlatFee > MaxGross {
		return 0, ErrInvalidRate
	}
	if r.FlatUpTo > 0 && gross <= r.FlatUpTo {
		return r.FlatFee, nil
	}
	num := gross * r.BPS // safe: gross <= 1e13, bps <= 1e4
	var q int64
	switch r.Rounding {
	case RoundFloor:
		q = num / 10000
	case RoundCeil:
		q = (num + 9999) / 10000
	case RoundHalfUp:
		q = (num + 5000) / 10000
	default:
		return 0, fmt.Errorf("%w: rounding %q", ErrInvalidRate, r.Rounding)
	}
	f := q + r.Fixed
	// The cap bounds the total fee of this component and only ever lowers it, so the
	// gross == mdr + platform + net invariant (net absorbs the difference) still holds.
	if r.Cap > 0 && (r.CapUpTo == 0 || gross <= r.CapUpTo) && f > r.Cap {
		f = r.Cap
	}
	return f, nil
}
