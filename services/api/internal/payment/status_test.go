package payment

import (
	"errors"
	"testing"
)

func TestApply(t *testing.T) {
	all := []Status{Created, Pending, Succeeded, Failed, Expired, Cancelled, Unknown}
	ok := map[[2]Status]bool{
		{Created, Pending}: true, {Created, Failed}: true, {Created, Unknown}: true,
		{Pending, Succeeded}: true, {Pending, Failed}: true, {Pending, Expired}: true,
		{Pending, Cancelled}: true, {Pending, Unknown}: true,
		{Unknown, Pending}: true, {Unknown, Succeeded}: true, {Unknown, Failed}: true, {Unknown, Expired}: true,
	}
	for _, from := range all {
		for _, to := range all {
			changed, err := Apply(from, to)
			switch {
			case from == to:
				if changed || err != nil {
					t.Errorf("%s→%s harus no-op, dapat changed=%v err=%v", from, to, changed, err)
				}
			case ok[[2]Status{from, to}]:
				if !changed || err != nil {
					t.Errorf("%s→%s seharusnya diizinkan, err=%v", from, to, err)
				}
			default:
				if !errors.Is(err, ErrInvalidTransition) {
					t.Errorf("%s→%s seharusnya ditolak", from, to)
				}
			}
		}
	}
}

func TestTerminalStatusesNeverLeave(t *testing.T) {
	for _, term := range []Status{Succeeded, Failed, Expired, Cancelled} {
		if !term.IsTerminal() {
			t.Errorf("%s harus terminal", term)
		}
		for _, to := range []Status{Created, Pending, Succeeded, Failed, Expired, Cancelled, Unknown} {
			if to == term {
				continue
			}
			if _, err := Apply(term, to); err == nil {
				t.Errorf("status terminal %s tidak boleh berpindah ke %s", term, to)
			}
		}
	}
	if Unknown.IsTerminal() || Pending.IsTerminal() {
		t.Error("unknown/pending bukan terminal")
	}
}
