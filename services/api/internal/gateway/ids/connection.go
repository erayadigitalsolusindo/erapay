package ids

import (
	"context"
	"errors"
	"net/http"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var _ gateway.ConnectionTester = (*Adapter)(nil)

// TestConnection implements gateway.ConnectionTester. IDS has no per-merchant account, so deep adds
// nothing: the check is a read-only GET /v1/api/payment-methods with the API key, which proves the
// base URL, the key and the channel are valid without creating a charge.
func (a *Adapter) TestConnection(ctx context.Context, _ string, _ bool) []gateway.ConnectionStep {
	_, _, err := a.do(ctx, http.MethodGet, pathMethods, nil, nil)
	st := gateway.ConnectionStep{Name: "credentials", OK: err == nil}
	if err != nil {
		var ge *gateway.Error
		if errors.As(err, &ge) {
			st.Code, st.Message = ge.Code, ge.Error()
		} else {
			st.Message = "unexpected error"
		}
	}
	return []gateway.ConnectionStep{st}
}
