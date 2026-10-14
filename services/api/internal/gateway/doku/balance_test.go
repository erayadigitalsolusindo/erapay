package doku

import (
	"context"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

func TestBalanceInquiryParsesDocumentedResponse(t *testing.T) {
	f, a := newFake(t)
	f.respond = func(string, map[string]any) (int, any) {
		return 200, map[string]any{
			"responseCode": "2005300", "responseMessage": "Successful",
			"accountNo": "6281234567890", "name": "PT ERA DIGITAL",
			"accountInfos": map[string]any{
				"holdAmount":       map[string]any{"value": "10000.00", "currency": "IDR"},
				"avaliableBalance": map[string]any{"value": "950000.00", "currency": "IDR"}, // typo DOKU dipertahankan
			},
			"additionalInfo": map[string]any{"creditAlertLimit": map[string]any{"value": "100000.00", "currency": "IDR"}},
		}
	}
	res, err := a.BalanceInquiry(context.Background(), "6281234567890")
	if err != nil {
		t.Fatal(err)
	}
	if res.AvailableBalance != 950000 || res.HoldAmount != 10000 ||
		res.AccountNo != "6281234567890" || res.Name != "PT ERA DIGITAL" || res.ResponseCode != "2005300" {
		t.Fatalf("balance: %+v", res)
	}
	if m := f.last; m["accountNo"] != "6281234567890" || m["partnerReferenceNo"] == "" {
		t.Fatalf("balance body: %v", m)
	}
}

func TestBalanceInquiryWithoutBalanceIsAmbiguous(t *testing.T) {
	f, a := newFake(t)
	f.respond = func(string, map[string]any) (int, any) {
		return 200, map[string]any{"responseCode": "2005300", "accountNo": "6281234567890"}
	}
	if _, err := a.BalanceInquiry(context.Background(), "6281234567890"); !gateway.IsAmbiguous(err) {
		t.Fatalf("got %v", err)
	}
}

func TestBalanceInquiryValidatesAccountNo(t *testing.T) {
	f, a := newFake(t)
	for _, bad := range []string{"", "01234567890123456"} { // >16
		if _, err := a.BalanceInquiry(context.Background(), bad); !gateway.IsRejected(err) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if f.calls.Load() != 0 {
		t.Fatal("invalid balance request reached the gateway")
	}
}
