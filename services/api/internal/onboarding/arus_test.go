package onboarding

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestProvisionARUSTenantRequiresTerms(t *testing.T) {
	service := &Service{}
	_, err := service.ProvisionARUSTenant(t.Context(), ARUSTenantInput{
		TenantID: uuid.New(), BusinessName: "Toko Contoh", OwnerName: "Pemilik Toko",
		OwnerEmail: "owner@example.test",
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want terms validation", err)
	}
}
