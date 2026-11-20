package dana

import (
	"context"
	"errors"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var _ gateway.ConnectionTester = (*Adapter)(nil)

// TestConnection checks credentials and signing without creating a QR: an inquiry of a reference that cannot
// exist answers 4045501 (Transaction Not Found) when DANA accepted our signature, and 401xxxx when it did not.
func (a *Adapter) TestConnection(ctx context.Context, _ string, _ bool) []gateway.ConnectionStep {
	env, _, err := a.do(ctx, pathQuery, map[string]string{
		"originalPartnerReferenceNo": "erapayconnectiontest", "serviceCode": serviceQRISMPM, "merchantId": a.cfg.MerchantID}, rcQueryOK)
	st := gateway.ConnectionStep{Name: "credentials", OK: env.ResponseCode == rcQueryNotFound}
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
