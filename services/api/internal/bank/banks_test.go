package bank

import "testing"

// TestBankList memeriksa keanggotaan daftar terhadap kasus kritis: kode BI
// konvensional, unit syariah berakhiran SY, bank tanpa BI-FAST, e-wallet
// (bukan rekening bank), dan nama bank yang sering tertukar dengan kodenya.
func TestBankList(t *testing.T) {
	cases := []struct {
		code       string
		wantValid  bool
		wantBIFast bool
	}{
		{"014", true, true},   // BANK BCA
		{"002", true, true},   // BANK BRI
		{"542SY", true, true}, // PT. BANK JAGO SYARIAH
		{"146", true, false},  // BANK OF INDIA INDONESIA: satu-satunya tanpa BI-FAST
		{"911", false, false}, // LinkAja: e-wallet, bukan rekening bank
		{"SHOPEEPAY", false, false},
		{"BCA", false, false}, // nama bank, bukan kode BI
		{"999", false, false}, // well-formed tapi tidak didukung
		{"", false, false},
	}
	for _, c := range cases {
		if got := Valid(c.code); got != c.wantValid {
			t.Errorf("Valid(%q) = %v, want %v", c.code, got, c.wantValid)
		}
		b, ok := Lookup(c.code)
		if ok != c.wantValid {
			t.Errorf("Lookup(%q) ok = %v, want %v", c.code, ok, c.wantValid)
		}
		if ok && b.BIFast != c.wantBIFast {
			t.Errorf("Lookup(%q).BIFast = %v, want %v", c.code, b.BIFast, c.wantBIFast)
		}
	}
	if len(Banks) != 125 {
		t.Errorf("len(Banks) = %d, want 125", len(Banks))
	}
	if len(byCode) != len(Banks) {
		t.Errorf("duplicate codes: byCode has %d entries for %d banks", len(byCode), len(Banks))
	}
	if got := Name("014"); got != "BANK BCA" {
		t.Errorf("Name(014) = %q", got)
	}
	if got := Name("999"); got != "" {
		t.Errorf("Name(999) = %q, want empty", got)
	}
}
