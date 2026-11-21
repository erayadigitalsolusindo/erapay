package dana

import (
	"context"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// PayOption is one payment method/option DANA offers for an order (Gapura Consult Pay, docs/integrations/dana.md).
type PayOption struct {
	PayMethod string `json:"payMethod"` // e.g. NETWORK_PAY, VIRTUAL_ACCOUNT
	PayOption string `json:"payOption"` // e.g. NETWORK_PAY_PG_OVO
}

// ConsultPay lists the payment options DANA offers for an amount. Read-only: it creates nothing and is not
// part of gateway.Adapter; it is the first step of Gapura Custom Checkout, whose Create Order is not implemented.
// Promo info is ignored until a checkout flow needs it.
func (a *Adapter) ConsultPay(ctx context.Context, amount int64) ([]PayOption, error) {
	if amount <= 0 {
		return nil, &gateway.Error{Kind: gateway.Rejected, Message: "invalid consult request"}
	}
	payload := map[string]any{
		"merchantId": a.cfg.MerchantID, "amount": money{fmtMoney(amount), "IDR"},
		"externalStoreId": a.cfg.StoreID, // mandatory when QRIS/stores are used; store_id is already required
		"additionalInfo":  map[string]any{"envInfo": map[string]string{"sourcePlatform": "IPG", "terminalType": "SYSTEM", "orderTerminalType": "SYSTEM"}},
	}
	env, _, err := a.do(ctx, pathConsult, payload, rcConsultOK)
	return env.PaymentInfos, err
}
