package pricing

import (
	"errors"
	"math/rand"
	"testing"
)

var defaultPlan = Plan{
	Version:  1,
	MDR:      Rate{BPS: 70, Rounding: RoundHalfUp},
	Platform: Rate{BPS: 100, Rounding: RoundHalfUp},
}

func TestComputeAcceptanceExample(t *testing.T) { // roadmap acceptance #17
	b, err := Compute(1000, "IDR", defaultPlan)
	if err != nil {
		t.Fatal(err)
	}
	if b.MDRFee != 7 || b.PlatformFee != 10 || b.MerchantNet != 983 || b.PlanVersion != 1 {
		t.Fatalf("got %+v", b)
	}
}

func TestRoundingModes(t *testing.T) {
	// gross 1234 * 70bps = 8.638
	for mode, want := range map[Rounding]int64{RoundFloor: 8, RoundHalfUp: 9, RoundCeil: 9} {
		p := defaultPlan
		p.MDR.Rounding = mode
		b, err := Compute(1234, "IDR", p)
		if err != nil || b.MDRFee != want {
			t.Fatalf("%s: got %+v err=%v want mdr=%d", mode, b, err, want)
		}
	}
}

func TestFixedComponent(t *testing.T) {
	p := defaultPlan
	p.Platform = Rate{BPS: 0, Fixed: 500, Rounding: RoundFloor}
	b, err := Compute(10000, "IDR", p)
	if err != nil || b.PlatformFee != 500 || b.MerchantNet != 10000-70-500 {
		t.Fatalf("got %+v err=%v", b, err)
	}
}

func TestInvalid(t *testing.T) {
	cases := []struct {
		name  string
		gross int64
		cur   string
		plan  Plan
		want  error
	}{
		{"zero", 0, "IDR", defaultPlan, ErrInvalidAmount},
		{"negative", -1, "IDR", defaultPlan, ErrInvalidAmount},
		{"too large", MaxGross + 1, "IDR", defaultPlan, ErrInvalidAmount},
		{"bad bps", 100, "IDR", Plan{MDR: Rate{BPS: 10001, Rounding: RoundFloor}, Platform: defaultPlan.Platform}, ErrInvalidRate},
		{"bad rounding", 100, "IDR", Plan{MDR: Rate{BPS: 1}, Platform: defaultPlan.Platform}, ErrInvalidRate},
		{"no currency", 100, "", defaultPlan, ErrInvalidRate},
		{"fees exceed", 100, "IDR", Plan{MDR: Rate{Fixed: 80, Rounding: RoundFloor}, Platform: Rate{Fixed: 80, Rounding: RoundFloor}}, ErrFeesExceed},
	}
	for _, c := range cases {
		if _, err := Compute(c.gross, c.cur, c.plan); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", c.name, err, c.want)
		}
	}
}

// Property: gross == mdr + platform + net for all valid inputs; no negatives.
func TestSumInvariantProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	modes := []Rounding{RoundFloor, RoundHalfUp, RoundCeil}
	for i := 0; i < 200000; i++ {
		gross := rng.Int63n(MaxGross) + 1
		if i%3 == 0 {
			gross = rng.Int63n(100000) + 1
		}
		p := Plan{
			MDR:      Rate{BPS: rng.Int63n(500), Rounding: modes[rng.Intn(3)]},
			Platform: Rate{BPS: rng.Int63n(500), Rounding: modes[rng.Intn(3)]},
		}
		b, err := Compute(gross, "IDR", p)
		if err != nil {
			t.Fatalf("gross=%d plan=%+v: %v", gross, p, err)
		}
		if b.MDRFee < 0 || b.PlatformFee < 0 || b.MerchantNet < 0 ||
			b.Gross != b.MDRFee+b.PlatformFee+b.MerchantNet {
			t.Fatalf("invariant broken: %+v", b)
		}
	}
}
