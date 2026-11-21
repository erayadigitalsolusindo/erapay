package onboarding

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
)

type stubNotifier struct{}

func (stubNotifier) SendVerification(_ context.Context, _, _ string) error   { return nil }
func (stubNotifier) SendAlreadyRegistered(_ context.Context, _ string) error { return nil }

// Sign-up needs a way to deliver the verification email and a key to seal the NIK/NPWP at rest:
// without either, /v1/auth/register stays off rather than storing strong identifiers in cleartext.
func TestCanRegisterRequiresMailerAndBox(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if NewService(nil, stubNotifier{}, Config{Log: log}).CanRegister() {
		t.Fatal("register must be off without an encryption key")
	}
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	if !NewService(nil, stubNotifier{}, Config{Box: box, Log: log}).CanRegister() {
		t.Fatal("register must be on with a mailer and a key")
	}
	if NewService(nil, nil, Config{Box: box, Log: log}).CanRegister() {
		t.Fatal("register must be off without a mailer")
	}
}

// Approve and ChangePricing share one validation: a typo must not become a tariff.
func TestValidateFees(t *testing.T) {
	s := NewService(nil, nil, Config{})
	ok := ApproveInput{MDRBPS: 70, PlatformBPS: 100, MDRFlatUpTo: 100000, MDRFlatFee: 500}
	if err := s.validateFees(ok); err != nil {
		t.Fatalf("valid fees refused: %v", err)
	}
	for name, in := range map[string]ApproveInput{
		"bps too high":      {MDRBPS: 10001},
		"fixed too high":    {PlatformFixed: MaxFixedFee + 1},
		"flat fee, no cap":  {MDRFlatFee: 500},
		"flat fee > cutoff": {MDRFlatUpTo: 100, MDRFlatFee: 500},
	} {
		if err := s.validateFees(in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
