package midtrans

import (
	"context"
	"errors"
	"net/http"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var _ gateway.ConnectionTester = (*Adapter)(nil)

// TestConnection checks the server key without creating a QR: inquiry of an order that cannot exist
// answers body 404 "Transaction doesn't exist." when the key is valid and 401 when it is not.
// The signature secret cannot be checked without a real notification; the step says so.
func (a *Adapter) TestConnection(ctx context.Context, _ string, _ bool) []gateway.ConnectionStep {
	res, err := a.do(ctx, http.MethodGet, "/v2/erapay-connection-check-0/status", "", nil)
	st := gateway.ConnectionStep{Name: "credentials", OK: res.StatusCode == "404"}
	if !st.OK {
		var ge *gateway.Error
		if errors.As(err, &ge) {
			st.Code, st.Message = ge.Code, ge.Error()
		} else {
			st.Message = "unexpected response"
		}
	}
	return []gateway.ConnectionStep{st}
}
