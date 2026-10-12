package payment

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func (e env) endpoint(t *testing.T, merchant, status string) {
	t.Helper()
	if _, err := e.db.Exec(context.Background(), `
		INSERT INTO webhook_endpoints (merchant_id, url, secret_enc, secret_hint, status)
		VALUES ($1, 'https://merchant.example/hook', '\x00', 'abcd', $2)`, merchant, status); err != nil {
		t.Fatal(err)
	}
}

// Only final statuses notify the merchant, one event per payment status, fanned out to active endpoints only.
func TestFinalStatusQueuesMerchantWebhookOnce(t *testing.T) {
	e := setup(t)
	e.endpoint(t, e.mid, "active")
	e.endpoint(t, e.mid, "active")
	e.endpoint(t, e.mid, "disabled")

	p := e.create(t, "ORD-1", 100000)
	e.move(t, p.ID, Pending, SourceInternal)
	if n := e.count(t, `SELECT count(*) FROM webhook_events`); n != 0 {
		t.Fatalf("non-final transition queued %d events", n)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.Transition(context.Background(), TransitionInput{PaymentID: p.ID, To: Succeeded,
				Source: SourceWebhook, DedupKey: "evt-" + strconv.Itoa(i)}); err != nil {
				t.Errorf("transition: %v", err)
			}
		}()
	}
	wg.Wait()
	// A late conflicting report must not create a second, contradictory notification.
	e.move(t, p.ID, Expired, SourceInquiry)

	if n := e.count(t, `SELECT count(*) FROM webhook_events`); n != 1 {
		t.Fatalf("events = %d, want 1", n)
	}
	if n := e.count(t, `SELECT count(*) FROM webhook_deliveries WHERE status = 'pending'`); n != 2 {
		t.Fatalf("deliveries = %d, want 2 (active endpoints only)", n)
	}

	var typ string
	var raw []byte
	if err := e.db.QueryRow(context.Background(), `SELECT type, data FROM webhook_events`).Scan(&typ, &raw); err != nil {
		t.Fatal(err)
	}
	var d struct {
		Payment struct {
			ID, Status, Currency string
			MerchantReference    string `json:"merchant_reference"`
			Amount               int64
			Fee                  struct {
				MerchantNet int64 `json:"merchant_net"`
			} `json:"fee"`
		}
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	pay := d.Payment
	if typ != "payment.succeeded" || pay.ID != p.ID || pay.MerchantReference != "ORD-1" || pay.Status != "succeeded" ||
		pay.Amount != 100000 || pay.Fee.MerchantNet != 100000-700-1000 {
		t.Fatalf("event %s %s", typ, raw)
	}
	for _, leak := range []string{"qr_code", "qr_payload", "gateway_reference", "merchant_id", "secret"} {
		if strings.Contains(strings.ToLower(string(raw)), leak) {
			t.Errorf("event data leaks %q: %s", leak, raw)
		}
	}
}

// Success racing expiry: exactly one final status wins, and exactly one matching event exists.
func TestRaceWinnerIsTheOnlyNotification(t *testing.T) {
	e := setup(t)
	e.endpoint(t, e.mid, "active")
	p := e.create(t, "ORD-2", 5000)
	e.move(t, p.ID, Pending, SourceInternal)
	var wg sync.WaitGroup
	for _, to := range []Status{Succeeded, Expired, Succeeded, Expired} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.svc.Transition(context.Background(), TransitionInput{PaymentID: p.ID, To: to, Source: SourceInquiry}) //nolint:errcheck
		}()
	}
	wg.Wait()
	got, err := e.svc.Get(context.Background(), e.mid, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM webhook_events WHERE type = $1`, "payment."+string(got.Status)); n != 1 {
		t.Fatalf("events for winner %s = %d", got.Status, n)
	}
	if n := e.count(t, `SELECT count(*) FROM webhook_events`); n != 1 {
		t.Fatalf("total events = %d", n)
	}
}
