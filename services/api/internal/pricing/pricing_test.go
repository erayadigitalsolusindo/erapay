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

func TestMDRCap(t *testing.T) {
	// Bank Indonesia QRIS rule (lisan DOKU 14 Okt 2026, efektif 1 Nov 2026): MDR 0,7%
	// capped at Rp500 for gross Rp0-Rp100.000. Above the threshold the cap no longer applies.
	capped := defaultPlan
	capped.MDR.Cap = 500
	capped.MDR.CapUpTo = 100_000
	for _, c := range []struct {
		name  string
		gross int64
		plan  Plan
		want  int64
	}{
		{"below cap, untouched", 50_000, capped, 350},
		{"just below threshold, capped", 99_999, capped, 500},
		{"at threshold, capped", 100_000, capped, 500},
		{"just above threshold, uncapped", 100_001, capped, 700},
		{"far above threshold", 200_000, capped, 1400},
		{"small payment, cap not reached", 1_000, capped, 7},
		{"no cap, old behaviour", 100_000, defaultPlan, 700},
		{"cap without threshold, applies everywhere", 200_000, func() Plan {
			p := defaultPlan
			p.MDR.Cap = 500
			return p
		}(), 500},
	} {
		b, err := Compute(c.gross, "IDR", c.plan)
		if err != nil || b.MDRFee != c.want {
			t.Errorf("%s: gross=%d got mdr=%d err=%v want=%d", c.name, c.gross, b.MDRFee, err, c.want)
		}
		if b.Gross != b.MDRFee+b.PlatformFee+b.MerchantNet {
			t.Errorf("%s: gross != mdr + platform + net: %+v", c.name, b)
		}
	}
	b, err := Compute(100_000, "IDR", capped)
	if err != nil || b.MerchantNet != 100_000-500-1000 {
		t.Fatalf("net with capped MDR: got %+v", b)
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
		{"negative cap", 100, "IDR", Plan{MDR: Rate{BPS: 70, Cap: -1, Rounding: RoundHalfUp}, Platform: defaultPlan.Platform}, ErrInvalidRate},
		{"negative cap_up_to", 100, "IDR", Plan{MDR: Rate{BPS: 70, Cap: 500, CapUpTo: -1, Rounding: RoundHalfUp}, Platform: defaultPlan.Platform}, ErrInvalidRate},
		{"cap_up_to without cap", 100, "IDR", Plan{MDR: Rate{BPS: 70, CapUpTo: 100_000, Rounding: RoundHalfUp}, Platform: defaultPlan.Platform}, ErrInvalidRate},
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
		mdr := Rate{BPS: rng.Int63n(500), Cap: rng.Int63n(500), Rounding: modes[rng.Intn(3)]}
		if mdr.Cap > 0 {
			mdr.CapUpTo = rng.Int63n(100000)
		}
		p := Plan{
			MDR:      mdr,
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

// Aturan transaksi kecil (BI): sampai batas (inklusif) MDR flat, di atasnya kembali ke rumus biasa.
func TestMDRFlatSmallTransaction(t *testing.T) {
	plan := Plan{Version: 1,
		MDR:      Rate{BPS: 70, FlatUpTo: 100_000, FlatFee: 500, Rounding: RoundHalfUp},
		Platform: Rate{BPS: 100, Rounding: RoundHalfUp}}
	cases := []struct{ gross, wantMDR int64 }{
		{1_000, 500},       // jauh di bawah batas
		{99_999, 500},      // tepat di bawah batas
		{100_000, 500},     // batas bersifat inklusif
		{100_001, 700},     // di atas batas: 0,7% (half_up) = 700,007 -> 700
		{1_000_000, 7_000}, // rumus biasa
	}
	for _, c := range cases {
		b, err := Compute(c.gross, "IDR", plan)
		if err != nil {
			t.Fatalf("gross %d: %v", c.gross, err)
		}
		if b.MDRFee != c.wantMDR {
			t.Errorf("gross %d: mdr = %d, want %d", c.gross, b.MDRFee, c.wantMDR)
		}
		if b.Gross != b.MDRFee+b.PlatformFee+b.MerchantNet {
			t.Errorf("gross %d: invarian jumlah rusak: %+v", c.gross, b)
		}
	}
	// Flat 0 = gratis di bawah batas (perilaku lama sebelum aturan baru).
	free := plan
	free.MDR.FlatFee = 0
	if b, _ := Compute(50_000, "IDR", free); b.MDRFee != 0 {
		t.Errorf("flat 0: mdr = %d, want 0", b.MDRFee)
	}
	// Biaya flat lebih besar dari nominal transaksi ditolak (net tidak boleh negatif).
	tiny := plan
	tiny.Platform = Rate{Rounding: RoundHalfUp}
	if _, err := Compute(300, "IDR", tiny); err == nil {
		t.Error("flat 500 pada transaksi 300 harus ditolak")
	}
	bad := plan
	bad.MDR.FlatFee = -1
	if _, err := Compute(1_000, "IDR", bad); err == nil {
		t.Error("flat negatif harus ditolak")
	}
}
