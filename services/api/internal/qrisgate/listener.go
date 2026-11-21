package qrisgate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/qris"
)

// MaxUniqueCode is the widest band of unique codes: a total to pay is base + 1..99 rupiah.
const MaxUniqueCode = 99

func aad(merchantID, field string) []byte { return []byte("qrisgate/" + merchantID + "/" + field) }

var pkgRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

// Account is what the tenant sees; the listener secret is only shown once, when it is created.
type Account struct {
	HasQRIS          bool     `json:"has_qris"`
	QRISName         string   `json:"qris_name,omitempty"`
	QRISPayload      string   `json:"qris_payload,omitempty"` // QRIS statis tersimpan (tercetak publik di toko, bukan rahasia); dasar QRIS dinamis tiap pembayaran
	HasSecret        bool     `json:"has_secret"`
	SecretHint       string   `json:"secret_hint,omitempty"`
	Packages         []string `json:"packages"`
	Ready            bool     `json:"ready"`        // QRIS + secret + at least one package
	ExactAmount      bool     `json:"exact_amount"` // the customer pays exactly the price (no unique code) unless the same amount is already waiting
	ActivationStatus string   `json:"activation_status,omitempty"`
	WebhookPath      string   `json:"webhook_path"`
}

func (s *Service) Account(ctx context.Context, merchantID string) (Account, error) {
	a := Account{WebhookPath: "/webhooks/qrisgate/" + merchantID, Packages: []string{}}
	var qrisName, hint, payload *string
	var sealed []byte
	err := s.db.QueryRow(ctx, `SELECT static_qris, qris_name, listener_secret_enc, secret_hint, packages, exact_amount
		FROM qrisgate_accounts WHERE merchant_id = $1`, merchantID).Scan(&payload, &qrisName, &sealed, &hint, &a.Packages, &a.ExactAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	a.HasQRIS, a.HasSecret = payload != nil, sealed != nil
	if payload != nil {
		a.QRISPayload = *payload
	}
	if qrisName != nil {
		a.QRISName = *qrisName
	}
	if hint != nil {
		a.SecretHint = *hint
	}
	if a.Packages == nil {
		a.Packages = []string{}
	}
	a.Ready = a.HasQRIS && a.HasSecret && len(a.Packages) > 0
	err = s.db.QueryRow(ctx, `SELECT activation_status FROM merchant_gateway_accounts WHERE merchant_id = $1 AND gateway = 'qrisgate' AND environment = 'production'`, merchantID).Scan(&a.ActivationStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return a, err
}

// ensure creates the account row and the gateway account row (pending_confirmation = the per-merchant flag, OFF
// until DEWA activates it) and rejects sandbox merchants. gateway_ref = the merchant id.
func (s *Service) ensure(ctx context.Context, tx pgx.Tx, merchantID string) error {
	var sandbox bool
	if err := tx.QueryRow(ctx, `SELECT is_sandbox FROM merchants WHERE id = $1`, merchantID).Scan(&sandbox); err != nil {
		return err
	}
	if sandbox {
		return ErrUnavailable
	}
	if _, err := tx.Exec(ctx, `INSERT INTO qrisgate_accounts (merchant_id) VALUES ($1) ON CONFLICT DO NOTHING`, merchantID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO merchant_gateway_accounts (merchant_id, gateway, environment, gateway_ref, activation_status)
		VALUES ($1::uuid, 'qrisgate', 'production', $2, 'pending_confirmation') ON CONFLICT (merchant_id, gateway, environment) DO NOTHING`, merchantID, merchantID)
	return err
}

// SaveQRIS validates and stores the tenant's static QRIS (replacing a previous one).
func (s *Service) SaveQRIS(ctx context.Context, merchantID, userID, requestID, payload string) (qris.Info, error) {
	info, err := qris.ValidateStatic(payload)
	if err != nil {
		return qris.Info{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return qris.Info{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := s.ensure(ctx, tx, merchantID); err != nil {
		return qris.Info{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE qrisgate_accounts SET static_qris = $2, qris_name = $3, updated_by = NULLIF($4,'')::uuid, updated_at = now() WHERE merchant_id = $1`,
		merchantID, strings.TrimSpace(payload), info.MerchantName, userID); err != nil {
		return qris.Info{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "qrisgate_account.save_qris", EntityType: "merchant", EntityID: merchantID, RequestID: requestID}); err != nil {
		return qris.Info{}, err
	}
	return info, tx.Commit(ctx)
}

// SetExactAmount turns "customer pays exactly the price" on or off for a merchant. Only the platform (DEWA) does this,
// on the tenant's request; it only affects NEW payments.
// Risk the tenant accepts by turning it on: any incoming transfer of exactly the price while a payment waits can settle it.
func (s *Service) SetExactAmount(ctx context.Context, merchantID, userID, requestID, note string, on bool) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := s.ensure(ctx, tx, merchantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE qrisgate_accounts SET exact_amount = $2, updated_by = NULLIF($3,'')::uuid, updated_at = now() WHERE merchant_id = $1`,
		merchantID, on, userID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "qrisgate_account.exact_amount", EntityType: "merchant", EntityID: merchantID,
		RequestID: requestID, Metadata: map[string]any{"enabled": on, "note": note}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// StaticQRIS returns the tenant's saved static QRIS payload and its name ("" when none). A static QRIS is printed
// publicly at the store, so showing it to the tenant is not a secret leak.
func (s *Service) StaticQRIS(ctx context.Context, merchantID string) (payload, name string, err error) {
	var p, n *string
	err = s.db.QueryRow(ctx, `SELECT static_qris, qris_name FROM qrisgate_accounts WHERE merchant_id = $1`, merchantID).Scan(&p, &n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if p != nil {
		payload = *p
	}
	if n != nil {
		name = *n
	}
	return payload, name, err
}

// SavePackages stores the allow-list of app package names whose notifications are accepted (1-20 names).
func (s *Service) SavePackages(ctx context.Context, merchantID, userID, requestID string, pkgs []string) error {
	seen := map[string]bool{}
	var clean []string
	for _, p := range pkgs {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if utf8.RuneCountInString(p) > 200 || !pkgRe.MatchString(p) {
			return ErrInvalid
		}
		seen[p] = true
		clean = append(clean, p)
	}
	if len(clean) == 0 || len(clean) > 20 {
		return ErrInvalid
	}
	sort.Strings(clean)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := s.ensure(ctx, tx, merchantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE qrisgate_accounts SET packages = $2, updated_by = NULLIF($3,'')::uuid, updated_at = now() WHERE merchant_id = $1`, merchantID, clean, userID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "qrisgate_account.save_packages", EntityType: "merchant", EntityID: merchantID, RequestID: requestID,
		Metadata: map[string]any{"packages": clean}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RotateListenerSecret creates a new HMAC secret for the Android listener and returns it ONCE; the previous one stops
// working at once. Only an encrypted copy and a 4-character hint are kept.
func (s *Service) RotateListenerSecret(ctx context.Context, merchantID, userID, requestID string) (string, error) {
	if s.box == nil {
		return "", ErrUnavailable
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	secret := "qls_" + hex.EncodeToString(raw[:])
	sealed, err := s.box.Seal([]byte(secret), aad(merchantID, "listener_secret"))
	if err != nil {
		return "", err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := s.ensure(ctx, tx, merchantID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE qrisgate_accounts SET listener_secret_enc = $2, secret_hint = $3, updated_by = NULLIF($4,'')::uuid, updated_at = now() WHERE merchant_id = $1`,
		merchantID, sealed, "…"+secret[len(secret)-4:], userID); err != nil {
		return "", err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "qrisgate_account.rotate_listener_secret", EntityType: "merchant", EntityID: merchantID, RequestID: requestID}); err != nil {
		return "", err
	}
	return secret, tx.Commit(ctx)
}

// ListenerSecret returns the merchant's listener secret for signature verification.
func (s *Service) ListenerSecret(ctx context.Context, merchantID string) (string, error) {
	if s.box == nil {
		return "", ErrUnavailable
	}
	var sealed []byte
	err := s.db.QueryRow(ctx, `SELECT listener_secret_enc FROM qrisgate_accounts WHERE merchant_id::text = $1`, merchantID).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && sealed == nil) {
		return "", ErrNoAccount
	}
	if err != nil {
		return "", err
	}
	b, err := s.box.Open(sealed, aad(merchantID, "listener_secret"))
	return string(b), err
}

// Claim reserves a pay amount (base + a free unique code) for a payment and returns it with the dynamic QRIS that
// asks for exactly that amount. Idempotent per payment. Expired claims of the merchant are released first.
func (s *Service) Claim(ctx context.Context, merchantID, paymentID string, base int64, expiresAt time.Time) (int64, string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var static *string
	var exact bool
	err = tx.QueryRow(ctx, `SELECT static_qris, exact_amount FROM qrisgate_accounts WHERE merchant_id = $1`, merchantID).Scan(&static, &exact)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && static == nil) {
		return 0, "", ErrNoQRIS
	}
	if err != nil {
		return 0, "", err
	}
	var pay int64
	err = tx.QueryRow(ctx, `SELECT pay_amount FROM qrisgate_claims WHERE payment_id = $1`, paymentID).Scan(&pay)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `UPDATE qrisgate_claims SET released_at = now() WHERE merchant_id = $1 AND released_at IS NULL AND expires_at < now()`, merchantID); err != nil {
			return 0, "", err
		}
		first := 1
		if exact {
			first = 0 // try the plain price first; the unique index makes a second identical open amount fall through to 1..99
		}
		for code := first; code <= MaxUniqueCode; code++ {
			err = tx.QueryRow(ctx, `INSERT INTO qrisgate_claims (payment_id, merchant_id, pay_amount, unique_code, expires_at)
				VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING RETURNING pay_amount`, paymentID, merchantID, base+int64(code), code, expiresAt).Scan(&pay)
			if err == nil {
				break
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return 0, "", err
			}
		}
		if pay == 0 {
			return 0, "", ErrNoFreeCode
		}
	} else if err != nil {
		return 0, "", err
	}
	payload, err := qris.Dynamic(*static, pay)
	if err != nil {
		return 0, "", ErrNoQRIS // the stored QRIS no longer validates: the tenant must save it again
	}
	return pay, payload, tx.Commit(ctx)
}

// ClaimExpired tells the poller whether a payment's QR has run out (the QR itself has no way to say so).
func (s *Service) ClaimExpired(ctx context.Context, paymentID string) (found, expired bool, err error) {
	var exp time.Time
	err = s.db.QueryRow(ctx, `SELECT expires_at FROM qrisgate_claims WHERE payment_id = $1`, paymentID).Scan(&exp)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	return true, time.Now().After(exp), err
}

// Words that mark an OUTGOING movement. A notification containing one is ignored, never matched to a payment.
var outgoing = []string{"transfer ke ", "mengirim", "dikirim", "pembayaran ke ", "bayar ke ", "tarik tunai", "pembelian", "debit", "top up", "topup", "isi saldo"}

var amountRe = regexp.MustCompile(`(?i)(?:rp\.?|idr)\s*([0-9]{1,3}(?:\.[0-9]{3})+|[0-9]+)(?:,[0-9]{1,2})?`)

// ParseAmount reads the first rupiah amount in text ("Rp 25.007", "Rp25007,00", "IDR 50.000"); 0 when there is none.
func ParseAmount(text string) int64 {
	m := amountRe.FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	var v int64
	for _, c := range strings.ReplaceAll(m[1], ".", "") {
		if v > (1<<62)/10 {
			return 0
		}
		v = v*10 + int64(c-'0')
	}
	return v
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// MatchNotification records one notification from the listener and, when it is an incoming payment whose amount
// equals the pay amount of exactly one active claim (within the claim's time window), returns that payment.
// Idempotent per (merchant, delivery id): a replay returns the earlier outcome and never matches again.
func (s *Service) MatchNotification(ctx context.Context, merchantID, deliveryID, pkg, title, text string, amount int64, postedAt time.Time) (string, int64, bool, error) {
	if serverAmount := ParseAmount(title + " " + text); serverAmount > 0 {
		amount = serverAmount // the server's own reading wins over the device's
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", 0, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var allowed []string
	if err := tx.QueryRow(ctx, `SELECT packages FROM qrisgate_accounts WHERE merchant_id = $1`, merchantID).Scan(&allowed); err != nil {
		return "", 0, false, err
	}
	status, reason := "unmatched", ""
	lower := strings.ToLower(title + " " + text)
	switch {
	case !contains(allowed, pkg):
		status, reason = "ignored", "package_not_allowed"
	case amount <= 0:
		status, reason = "ignored", "no_amount"
	default:
		for _, w := range outgoing {
			if strings.Contains(lower, w) {
				status, reason = "ignored", "looks_outgoing"
				break
			}
		}
	}
	var evID string
	var amt *int64
	if amount > 0 {
		amt = &amount
	}
	err = tx.QueryRow(ctx, `INSERT INTO qrisgate_events (merchant_id, delivery_id, package, title, body, amount, posted_at, status, reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')) ON CONFLICT (merchant_id, delivery_id) DO NOTHING RETURNING id::text`,
		merchantID, deliveryID, clip(pkg, 200), clip(title, 300), clip(text, 1000), amt, postedAt, status, reason).Scan(&evID)
	if errors.Is(err, pgx.ErrNoRows) { // replay: report the earlier outcome
		var pid *string
		var st string
		if err := tx.QueryRow(ctx, `SELECT status, payment_id::text FROM qrisgate_events WHERE merchant_id = $1 AND delivery_id = $2`, merchantID, deliveryID).Scan(&st, &pid); err != nil {
			return "", 0, false, err
		}
		if st != "matched" || pid == nil {
			return "", 0, false, nil
		}
		base, err := s.baseOf(ctx, tx, *pid)
		return *pid, base, err == nil, err
	}
	if err != nil {
		return "", 0, false, err
	}
	if status != "unmatched" {
		return "", 0, false, tx.Commit(ctx)
	}
	var pid string
	var base int64
	err = tx.QueryRow(ctx, `
		SELECT c.payment_id::text, c.pay_amount - c.unique_code
		FROM qrisgate_claims c JOIN payments p ON p.id = c.payment_id
		WHERE c.merchant_id = $1 AND c.pay_amount = $2 AND c.released_at IS NULL AND p.status IN ('created','pending','unknown')
		  AND $3 >= c.created_at - interval '2 minutes' AND $3 <= c.expires_at + interval '5 minutes'`, merchantID, amount, postedAt).Scan(&pid, &base)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, tx.Commit(ctx)
	}
	if err != nil {
		return "", 0, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE qrisgate_events SET status = 'matched', payment_id = $2 WHERE id = $1`, evID, pid); err != nil {
		return "", 0, false, err
	}
	return pid, base, true, tx.Commit(ctx)
}

func (s *Service) baseOf(ctx context.Context, tx pgx.Tx, paymentID string) (int64, error) {
	var base int64
	err := tx.QueryRow(ctx, `SELECT amount FROM payments WHERE id = $1`, paymentID).Scan(&base)
	return base, err
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// Event is one received notification, for the tenant's review list.
type Event struct {
	ID         string    `json:"id"`
	ReceivedAt time.Time `json:"received_at"`
	Package    string    `json:"package"`
	Title      string    `json:"title"`
	Text       string    `json:"text"`
	Amount     *int64    `json:"amount,omitempty"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	PaymentID  string    `json:"payment_id,omitempty"`
}

func (s *Service) Events(ctx context.Context, merchantID string, limit int) ([]Event, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.Query(ctx, `SELECT id::text, received_at, package, title, body, amount, status, COALESCE(reason,''), COALESCE(payment_id::text,'')
		FROM qrisgate_events WHERE merchant_id = $1 ORDER BY received_at DESC, id LIMIT $2`, merchantID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Event, error) {
		var e Event
		return e, r.Scan(&e.ID, &e.ReceivedAt, &e.Package, &e.Title, &e.Text, &e.Amount, &e.Status, &e.Reason, &e.PaymentID)
	})
}
