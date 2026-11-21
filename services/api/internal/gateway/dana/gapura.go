package dana

import (
	"context"
	"strings"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// createGapura creates a QRIS order through Gapura Custom Checkout (scenario API: our own UI, no redirect).
// ref is the already validated 25 character partnerReferenceNo. Same ambiguity rules as Generate QRIS.
func (a *Adapter) createGapura(ctx context.Context, r gateway.CreateRequest, ref string) (gateway.CreateResult, error) {
	amt := money{fmtMoney(r.Amount), "IDR"}
	payload := map[string]any{
		"partnerReferenceNo": ref, "merchantId": a.cfg.MerchantID, "externalStoreId": a.cfg.StoreID,
		"amount": amt, "validUpTo": timestamp(r.ExpiresAt),
		"urlParams": []map[string]string{
			{"url": a.cfg.CallbackURL, "type": "NOTIFICATION", "isDeeplink": "N"},
			{"url": a.cfg.ReturnURL, "type": "PAY_RETURN", "isDeeplink": "N"},
		},
		"payOptionDetails": []map[string]any{{"payMethod": "NETWORK_PAY", "payOption": "NETWORK_PAY_PG_QRIS", "transAmount": amt}},
		"additionalInfo": map[string]any{
			"order":   map[string]string{"orderTitle": "EraPay QRIS", "scenario": "API"},
			"mcc":     a.cfg.MCC,
			"envInfo": map[string]string{"sourcePlatform": "IPG", "orderTerminalType": "SYSTEM", "terminalType": "SYSTEM"},
		},
	}
	if r.Hosted {
		// BELUM JELAS: the Hosted Checkout Create Order page is not read yet. Assumed: same endpoint, scenario
		// REDIRECT, and no payOptionDetails so DANA's page lists every method. DANA's answer in sandbox settles it.
		delete(payload, "payOptionDetails")
		payload["additionalInfo"].(map[string]any)["order"] = map[string]string{"orderTitle": "EraPay", "scenario": "REDIRECT"}
	}
	env, status, err := a.do(ctx, pathGapuraCreate, payload, rcGapuraCreateOK)
	if err != nil {
		return gateway.CreateResult{}, err
	}
	if r.Hosted {
		if !strings.HasPrefix(env.WebRedirectURL, "https://") {
			return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "create response without checkout URL"}
		}
		gref := env.ReferenceNo
		if gref == "" {
			gref = r.Reference
		}
		return gateway.CreateResult{GatewayReference: gref, QRPayload: env.WebRedirectURL, RawStatus: "01", ResponseCode: env.ResponseCode}, nil
	}
	// BELUM JELAS (docs/integrations/dana.md): paymentCode is documented as the code for "Virtual Account/QRIS";
	// only an EMVCo string (starts with 000201) may be shown as a QR.
	if !strings.HasPrefix(env.AdditionalInfo.PaymentCode, "000201") {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "create response without QRIS payment code"}
	}
	gref := env.ReferenceNo
	if gref == "" {
		gref = r.Reference
	}
	return gateway.CreateResult{GatewayReference: gref, QRPayload: env.AdditionalInfo.PaymentCode, RawStatus: "01", ResponseCode: env.ResponseCode}, nil
}
