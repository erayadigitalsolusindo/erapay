package kyc

import "testing"

func TestIntlPhone(t *testing.T) {
	for in, want := range map[string]string{
		"81234567890":    "+6281234567890",
		"+6281234567890": "+6281234567890", // stored by sign-up
		"6281234567890":  "+6281234567890",
	} {
		if got := intlPhone(in); got != want {
			t.Errorf("intlPhone(%q) = %q, want %q", in, got, want)
		}
	}
}
