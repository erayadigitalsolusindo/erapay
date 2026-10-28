package ledger

import "testing"

func TestClearingAccountPerGateway(t *testing.T) {
	for gw, want := range map[string]string{"ids": CashIDSClearing, "doku": CashDokuClearing, "gw1": CashDokuClearing, "": CashDokuClearing} {
		if got := ClearingAccount(gw); got != want {
			t.Errorf("ClearingAccount(%q) = %q, want %q", gw, got, want)
		}
	}
}

func TestPaymentSucceededViaUsesTheGivenClearingAccount(t *testing.T) {
	bd := breakdown(t, 100000)
	for name, tc := range map[string]struct {
		build func() (Txn, error)
		want  string
		other string
	}{
		"ids":     {func() (Txn, error) { return PaymentSucceededVia(CashIDSClearing, "m1", "p1", bd) }, CashIDSClearing, CashDokuClearing},
		"default": {func() (Txn, error) { return PaymentSucceeded("m1", "p1", bd) }, CashDokuClearing, CashIDSClearing},
	} {
		tx, err := tc.build()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var debit int64
		for _, e := range tx.Entries {
			if e.Account == tc.other {
				t.Errorf("%s: touched %s", name, tc.other)
			}
			if e.Account == tc.want && e.Direction == Debit {
				debit += e.Amount
			}
		}
		if debit != bd.Gross-bd.MDRFee {
			t.Errorf("%s: clearing debit %d, want %d", name, debit, bd.Gross-bd.MDRFee)
		}
		if tx.DedupKey != "payment_succeeded:p1" {
			t.Errorf("%s: dedup key %q", name, tx.DedupKey)
		}
	}
}
