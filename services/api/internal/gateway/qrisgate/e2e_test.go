package qrisgate

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/platformsetting"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/qris"
	dom "github.com/erayadigitalsolusindo/erapay/services/api/internal/qrisgate"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

type world struct {
	db       *pgxpool.Pool
	merchant string
	svc      *dom.Service
	a        *Adapter
	o        *payment.Orchestrator
	secret   string
	set      *platformsetting.Service
}

func staticQRIS() string {
	guid, nmid := "ID.CO.EXAMPLE.WWW", "936000000000000001"
	inner := fmt.Sprintf("00%02d%s01%02d%s", len(guid), guid, len(nmid), nmid)
	body := "000201" + "010211" + fmt.Sprintf("26%02d%s", len(inner), inner) + "52045812" + "5303360" + "5802ID" + "5911TOKO CONTOH" + "6007JAKARTA" + "6304"
	return body + qris.CRC16(body)
}

func newWorld(t *testing.T) world {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
	m, _ := testdb.SeedMerchant(t, db, "QG")
	box, err := secretbox.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	set := platformsetting.NewService(db)
	svc := dom.NewService(db, set, nil)
	svc.SetBox(box)
	var user string
	if err := db.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ('t@x.test','x') RETURNING id::text`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveQRIS(ctx, m, user, "", staticQRIS()); err != nil {
		t.Fatal(err)
	}
	if err := svc.SavePackages(ctx, m, user, "", []string{"id.dana"}); err != nil {
		t.Fatal(err)
	}
	secret, err := svc.RotateListenerSecret(ctx, m, user, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE merchant_gateway_accounts SET activation_status = 'active' WHERE merchant_id = $1 AND gateway = 'qrisgate'`, m); err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Port: svc})
	if err != nil {
		t.Fatal(err)
	}
	reg := gateway.NewRegistry()
	reg.Register(a)
	o := payment.NewOrchestrator(payment.NewService(db), db, idempotency.NewStore(db), reg, "production", slog.New(slog.NewTextHandler(io.Discard, nil)))
	return world{db, m, svc, a, o, secret, set}
}

func (w world) create(t *testing.T, ref string, amount int64) payment.Payment {
	t.Helper()
	out, err := w.o.Create(context.Background(), payment.CreateRequest{MerchantID: w.merchant, MerchantReference: ref, Amount: amount, Currency: "IDR",
		PaymentMethod: "QRIS", IdempotencyKey: "idem-" + ref + "-0001", RequestHash: []byte(ref)})
	if err != nil {
		t.Fatal(err)
	}
	return out.Payment
}

// deliver plays the listener: a signed POST to /webhooks/qrisgate/<merchant>, run through the same verify + apply the
// HTTP handler uses.
func (w world) deliver(t *testing.T, delivery, text string, at time.Time) (payment.Result, error) {
	t.Helper()
	body := fmt.Sprintf(`{"event":"notification.posted","delivery_id":%q,"package":"id.dana","title":"Dana masuk","text":%q,"posted_at":%q}`,
		delivery, text, at.UTC().Format(time.RFC3339))
	ts := time.Now().Unix()
	h := hook(w.secret, ts, body, delivery)
	ev, err := w.a.VerifyWebhook(h, "/webhooks/qrisgate/"+w.merchant, []byte(body))
	if err != nil {
		return payment.Result{}, err
	}
	return w.o.ApplyWebhook(context.Background(), Code, ev)
}

func (w world) status(t *testing.T, id string) string {
	t.Helper()
	var s string
	if err := w.db.QueryRow(context.Background(), `SELECT status FROM payments WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (w world) n(t *testing.T, q string, args ...any) (v int64) {
	t.Helper()
	if err := w.db.QueryRow(context.Background(), q, args...).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEndToEndPaymentIsRecognisedFromAListenerNotificationExactlyOnce(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	if err := w.set.SetFee(ctx, 100, "biaya awal", "", ""); err != nil {
		t.Fatal(err)
	}
	p := w.create(t, "E2E-1", 25_000)
	if p.Status != payment.Pending || p.QRCode == nil || w.n(t, `SELECT count(*) FROM payments WHERE id = $1 AND direct`, p.ID) != 1 {
		t.Fatalf("created: %+v", p)
	}
	pay := qris.Amount(p.QRCode.Value)
	if pay != 25_001 || p.Amount != 25_000 {
		t.Fatalf("QR asks %d for a %d sale; want 25001 / 25000", pay, p.Amount)
	}
	if _, err := qris.ValidateStatic(p.QRCode.Value); err == nil {
		t.Fatal("the issued QR must be dynamic, not the static one")
	}

	// Wrong amount, outgoing and wrong-app notifications do not pay it.
	if res, err := w.deliver(t, "d-wrong", "Anda menerima Rp 25.002 dari BUDI", time.Now()); err == nil && res.Applied {
		t.Fatal("a different amount paid the payment")
	}
	if res, err := w.deliver(t, "d-out", "Anda mengirim Rp 25.001 ke ANI", time.Now()); err == nil && res.Applied {
		t.Fatal("an outgoing transfer paid the payment")
	}
	if w.status(t, p.ID) != "pending" {
		t.Fatalf("status %s; want pending", w.status(t, p.ID))
	}

	// The same notification delivered concurrently (retries) pays once.
	var applied atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := w.deliver(t, "d-ok", "Anda menerima Rp 25.001 dari BUDI", time.Now())
			if err != nil {
				t.Errorf("deliver: %v", err)
			} else if res.Applied {
				applied.Add(1)
			}
		}()
	}
	wg.Wait()
	if applied.Load() != 1 || w.status(t, p.ID) != "succeeded" {
		t.Fatalf("applied %d times, status %s", applied.Load(), w.status(t, p.ID))
	}
	// Direct payment: no sale journal, only the fee from the tenant's credit; the claim is free again.
	if bal, _ := w.svc.Balance(ctx, w.merchant); bal != -100 {
		t.Fatalf("credit balance %d; want -100", bal)
	}
	if n := w.n(t, `SELECT count(*) FROM qrisgate_claims WHERE payment_id = $1 AND released_at IS NOT NULL`, p.ID); n != 1 {
		t.Fatal("claim not released on success")
	}
	if n := w.n(t, `SELECT COALESCE(SUM(debit_minus_credit),0) FROM ledger_account_balances WHERE code = 'merchant_payable'`); n != 0 {
		t.Fatalf("merchant_payable %d", n)
	}
	// A replay of the paid notification, now that the code is free, must not pay the NEXT payment of that amount.
	p2 := w.create(t, "E2E-2", 25_000)
	if qris.Amount(p2.QRCode.Value) != 25_001 {
		t.Fatalf("the freed code should be reused: %d", qris.Amount(p2.QRCode.Value))
	}
	if res, err := w.deliver(t, "d-ok", "Anda menerima Rp 25.001 dari BUDI", time.Now()); err != nil || res.Applied {
		t.Fatalf("replay of a paid notification: %+v %v", res, err)
	}
	if w.status(t, p2.ID) != "pending" {
		t.Fatal("a replayed notification paid a different payment")
	}
	// A fresh notification does.
	if res, err := w.deliver(t, "d-new", "Anda menerima Rp 25.001 dari ANI", time.Now()); err != nil || !res.Applied {
		t.Fatalf("fresh notification: %+v %v", res, err)
	}
	if bal, _ := w.svc.Balance(ctx, w.merchant); bal != -200 {
		t.Fatalf("balance %d; want -200", bal)
	}
}

func TestConcurrentPaymentsOfTheSameAmountGetDistinctCodesAndAreNotMixedUp(t *testing.T) {
	w := newWorld(t)
	const n = 8
	pays := make([]payment.Payment, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); pays[i] = w.create(t, "C"+strconv.Itoa(i), 10_000) }()
	}
	wg.Wait()
	seen := map[int64]bool{}
	for _, p := range pays {
		a := qris.Amount(p.QRCode.Value)
		if a <= 10_000 || a > 10_099 || seen[a] {
			t.Fatalf("pay amounts must be distinct within 10001..10099: %d (seen %v)", a, seen)
		}
		seen[a] = true
	}
	// Paying exactly one of them leaves the others untouched.
	target := pays[3]
	if res, err := w.deliver(t, "d-3", fmt.Sprintf("Masuk Rp %s", dots(qris.Amount(target.QRCode.Value))), time.Now()); err != nil || !res.Applied {
		t.Fatalf("%+v %v", res, err)
	}
	for i, p := range pays {
		want := "pending"
		if i == 3 {
			want = "succeeded"
		}
		if got := w.status(t, p.ID); got != want {
			t.Errorf("payment %d: %s; want %s", i, got, want)
		}
	}
}

func dots(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) > 3 {
		return s[:len(s)-3] + "." + s[len(s)-3:]
	}
	return s
}

func TestLateOrEarlyNotificationsDoNotMatchAndExpiredQRClosesViaInquiry(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	p := w.create(t, "T1", 40_000)
	pay := qris.Amount(p.QRCode.Value)
	text := "Anda menerima Rp " + dots(pay)
	// Far before the QR existed, and long after it expired: no match.
	if res, err := w.deliver(t, "early", text, time.Now().Add(-3*time.Hour)); err == nil && res.Applied {
		t.Fatal("a notification from before the QR existed paid it")
	}
	if res, err := w.deliver(t, "late", text, time.Now().Add(3*time.Hour)); err == nil && res.Applied {
		t.Fatal("a notification long after expiry paid it")
	}
	// The unmatched ones are kept for the tenant to review.
	evs, err := w.svc.Events(ctx, w.merchant, 10)
	if err != nil || len(evs) != 2 || evs[0].Status != "unmatched" {
		t.Fatalf("events: %+v %v", evs, err)
	}
	// Expiry: once the claim's time has passed, inquiry says EXPIRED (the poller then closes the payment).
	if _, err := w.db.Exec(ctx, `UPDATE qrisgate_claims SET expires_at = now() - interval '1 minute' WHERE payment_id = $1`, p.ID); err != nil {
		t.Fatal(err)
	}
	q, err := w.a.GetPayment(ctx, gateway.QueryRequest{Reference: p.ID, AccountRef: w.merchant})
	if err != nil || q.RawStatus != "EXPIRED" {
		t.Fatalf("inquiry: %+v %v", q, err)
	}
	// An expired, unreleased claim frees its code for the next payment of that amount.
	p2 := w.create(t, "T2", 40_000)
	if qris.Amount(p2.QRCode.Value) != pay {
		t.Fatalf("expired claim not reusable: %d vs %d", qris.Amount(p2.QRCode.Value), pay)
	}
}

func TestNotificationOfAnotherMerchantOrWithoutSignatureNeverReachesAPayment(t *testing.T) {
	w := newWorld(t)
	p := w.create(t, "X1", 15_000)
	text := "Anda menerima Rp " + dots(qris.Amount(p.QRCode.Value))
	// Signed with a wrong secret.
	body := fmt.Sprintf(`{"event":"notification.posted","delivery_id":"x","package":"id.dana","title":"t","text":%q}`, text)
	h := hook("qls_not-the-real-secret-xx", time.Now().Unix(), body, "x")
	if _, err := w.a.VerifyWebhook(h, "/webhooks/qrisgate/"+w.merchant, []byte(body)); err == nil {
		t.Fatal("a wrong secret was accepted")
	}
	if w.status(t, p.ID) != "pending" {
		t.Fatal("status changed")
	}
}

// Nominal persis: tanpa kode unik pelanggan membayar tepat harga. Bila nominal yang sama sedang menunggu, transaksi
// berikutnya jatuh ke kode unik, jadi satu notifikasi tetap hanya melunasi satu transaksi.
func TestExactAmountChargesThePlainPriceAndFallsBackToACodeOnlyOnCollision(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	var user string
	if err := w.db.QueryRow(ctx, `SELECT id::text FROM users LIMIT 1`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.SetExactAmount(ctx, w.merchant, user, "", "uji", true); err != nil {
		t.Fatal(err)
	}
	first := w.create(t, "X1", 10_000)
	second := w.create(t, "X2", 10_000) // same price while the first still waits
	if a := qris.Amount(first.QRCode.Value); a != 10_000 {
		t.Fatalf("exact mode must ask for the plain price, got %d", a)
	}
	if a := qris.Amount(second.QRCode.Value); a <= 10_000 || a > 10_099 {
		t.Fatalf("a collision must fall back to a unique code, got %d", a)
	}
	if res, err := w.deliver(t, "d-x1", "Masuk Rp 10.000", time.Now()); err != nil || !res.Applied {
		t.Fatalf("%+v %v", res, err)
	}
	if w.status(t, first.ID) != "succeeded" || w.status(t, second.ID) != "pending" {
		t.Fatalf("only the plain-price payment may settle: %s / %s", w.status(t, first.ID), w.status(t, second.ID))
	}
	// Turned off again: new payments carry a unique code once more.
	if err := w.svc.SetExactAmount(ctx, w.merchant, user, "", "uji", false); err != nil {
		t.Fatal(err)
	}
	if a := qris.Amount(w.create(t, "X3", 20_000).QRCode.Value); a == 20_000 {
		t.Fatalf("default mode must add a unique code, got %d", a)
	}
}
