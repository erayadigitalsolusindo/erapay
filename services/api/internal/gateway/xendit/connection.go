package xendit

import (
	"context"
	"errors"
	"net/http"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var _ gateway.ConnectionTester = (*Adapter)(nil)

// TestConnection checks the key without creating a QR: inquiry of an id that cannot exist answers
// 404 DATA_NOT_FOUND when the key is valid and 401 when it is not.
func (a *Adapter) TestConnection(ctx context.Context, _ string, _ bool) []gateway.ConnectionStep {
	_, status, err := a.do(ctx, http.MethodGet, pathRequest+"/pr-00000000-0000-0000-0000-000000000000", nil)
	st := gateway.ConnectionStep{Name: "credentials", OK: status == http.StatusNotFound}
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
