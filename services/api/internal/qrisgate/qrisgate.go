// Package qrisgate holds the tenant's prepaid QRISGate credit: manual top-ups confirmed by Finance, and the
// per-transaction fee (platformsetting.QRISGateFee) charged to it. The balance is derived from the ledger
// account qrisgate_credit only (credits - debits) and may be negative: a fee is never refused for lack of credit.
package qrisgate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/ledger"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/platformsetting"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
)

const Currency = "IDR"

// MaxTopup bounds one top-up request (Rp100.000.000). MinQRISTopup keeps the MDR from eating a tiny top-up.
const (
	MaxTopup     = 100_000_000
	MinQRISTopup = 10_000
)

var (
	ErrInvalid      = errors.New("qrisgate: invalid input")
	ErrNotFound     = errors.New("qrisgate: not found")
	ErrInvalidState = errors.New("qrisgate: top-up is not in a state that allows this")
	ErrSelfDecision = errors.New("qrisgate: requester cannot decide their own top-up")
	ErrConflict     = errors.New("qrisgate: Idempotency-Key used with a different amount")
	ErrNoAccount    = errors.New("qrisgate: credentials are not configured")
	ErrUnavailable  = errors.New("qrisgate: saving credentials is unavailable (no encryption key or sandbox merchant)")
)

// Payer creates the QRIS payment behind a top-up (satisfied by *payment.Orchestrator).
type Payer interface {
	Create(ctx context.Context, r payment.CreateRequest) (payment.Outcome, error)
}

type Service struct {
	db       *pgxpool.Pool
	settings *platformsetting.Service
	payer    Payer          // nil disables QRIS top-ups
	box      *secretbox.Box // nil disables saving per-tenant QRISGate credentials
}

// SetPayer and SetBox wire optional parts after construction (the payer needs the gateway registry, which in turn
// needs this service as the adapter's credential source).
func (s *Service) SetPayer(p Payer)        { s.payer = p }
func (s *Service) SetBox(b *secretbox.Box) { s.box = b }

func NewService(db *pgxpool.Pool, settings *platformsetting.Service, payer Payer) *Service {
	return &Service{db: db, settings: settings, payer: payer}
}

type Topup struct {
	ID            string     `json:"id"`
	MerchantID    string     `json:"merchant_id"`
	Amount        int64      `json:"amount"`
	Currency      string     `json:"currency"`
	Status        string     `json:"status"`
	Method        string     `json:"method"`
	PaymentID     string     `json:"payment_id,omitempty"`
	QRPayload     string     `json:"qr_payload,omitempty"` // only while the QRIS payment can still be paid
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	BankReference string     `json:"bank_reference,omitempty"`
	Note          string     `json:"note,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
	Replayed      bool       `json:"replayed,omitempty"`
}

// topupFrom reads top-ups together with the QR of a still-payable QRIS payment.
const topupFrom = `SELECT t.id::text, t.merchant_id::text, t.amount, t.currency, t.status, t.method, COALESCE(t.payment_id::text,''),
	CASE WHEN p.status = 'pending' THEN COALESCE(p.qr_payload,'') ELSE '' END, CASE WHEN p.status = 'pending' THEN p.expires_at END,
	COALESCE(t.bank_reference,''), COALESCE(t.note,''), t.created_at, t.decided_at
	FROM qrisgate_topups t LEFT JOIN payments p ON p.id = t.payment_id `

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func scanTopup(r pgx.Row) (Topup, error) {
	var t Topup
	return t, r.Scan(&t.ID, &t.MerchantID, &t.Amount, &t.Currency, &t.Status, &t.Method, &t.PaymentID, &t.QRPayload, &t.ExpiresAt,
		&t.BankReference, &t.Note, &t.CreatedAt, &t.DecidedAt)
}

func getTopup(ctx context.Context, q queryRower, id string) (Topup, error) {
	return scanTopup(q.QueryRow(ctx, topupFrom+`WHERE t.id = $1`, id))
}

// Balance = credits - debits of the tenant's qrisgate_credit account (negative = the tenant owes EraPay).
func (s *Service) Balance(ctx context.Context, merchantID string) (int64, error) {
	var dmc int64
	err := s.db.QueryRow(ctx, `SELECT COALESCE(SUM(debit_minus_credit),0) FROM ledger_account_balances
		WHERE merchant_id = $1 AND currency = $2 AND code = $3`, merchantID, Currency, ledger.QRISGateCredit).Scan(&dmc)
	return -dmc, err
}

// RequestTopup is idempotent per (merchant, key): a replay returns the first request; the same key with
// another amount is ErrConflict.
func (s *Service) RequestTopup(ctx context.Context, merchantID, userID, key string, amount int64) (Topup, error) {
	if amount <= 0 || amount > MaxTopup || utf8.RuneCountInString(key) < 8 || utf8.RuneCountInString(key) > 128 {
		return Topup{}, ErrInvalid
	}
	t, _, err := s.insertTopup(ctx, merchantID, userID, key, amount, "manual")
	return t, err
}

// insertTopup is idempotent per (merchant, key): replay = the existing row (replayed=true); the same key with a
// different amount or method is ErrConflict.
func (s *Service) insertTopup(ctx context.Context, merchantID, userID, key string, amount int64, method string) (Topup, bool, error) {
	var id string
	err := s.db.QueryRow(ctx, `
		INSERT INTO qrisgate_topups (merchant_id, amount, currency, method, idempotency_key, requested_by)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (merchant_id, idempotency_key) DO NOTHING RETURNING id::text`,
		merchantID, amount, Currency, method, key, userID).Scan(&id)
	replayed := errors.Is(err, pgx.ErrNoRows)
	if replayed {
		err = s.db.QueryRow(ctx, `SELECT id::text FROM qrisgate_topups WHERE merchant_id = $1 AND idempotency_key = $2`, merchantID, key).Scan(&id)
	}
	if err != nil {
		return Topup{}, false, err
	}
	t, err := getTopup(ctx, s.db, id)
	if err != nil {
		return Topup{}, false, err
	}
	if replayed && (t.Amount != amount || t.Method != method) {
		return Topup{}, false, ErrConflict
	}
	t.Replayed = replayed
	return t, replayed, nil
}

// RequestQRISTopup starts a top-up paid by QRIS through EraPay. The credit arrives by itself when the payment
// succeeds (payment.Transition posts the journal); this only creates the request and the QRIS payment. Retrying
// with the same key returns the same payment, also after a crash between the two steps.
func (s *Service) RequestQRISTopup(ctx context.Context, merchantID, userID, key string, amount int64) (Topup, error) {
	if s.payer == nil || amount < MinQRISTopup || amount > MaxTopup || utf8.RuneCountInString(key) < 8 || utf8.RuneCountInString(key) > 128 {
		return Topup{}, ErrInvalid
	}
	t, _, err := s.insertTopup(ctx, merchantID, userID, key, amount, "qris")
	if err != nil || t.PaymentID != "" || t.Status != "requested" {
		return t, err
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("qgtopup|%s|%d", t.ID, amount)))
	out, err := s.payer.Create(ctx, payment.CreateRequest{MerchantID: merchantID, MerchantReference: "TOPUP-" + t.ID[:8],
		Description: "Top-up saldo QRISGate", Amount: amount, Currency: Currency, PaymentMethod: "QRIS",
		IdempotencyKey: "qgtopup-" + t.ID, RequestHash: h[:], Purpose: payment.PurposeQRISGateTopup})
	if errors.Is(err, payment.ErrMethodUnavailable) || errors.Is(err, payment.ErrNoPricing) || errors.Is(err, payment.ErrMerchantNotActive) || errors.Is(err, payment.ErrValidation) {
		_, _ = s.db.Exec(ctx, `UPDATE qrisgate_topups SET status = 'cancelled', decided_at = now() WHERE id = $1 AND status = 'requested' AND payment_id IS NULL`, t.ID)
	}
	if err != nil {
		return Topup{}, err
	}
	if _, err := s.db.Exec(ctx, `UPDATE qrisgate_topups SET payment_id = $2 WHERE id = $1 AND payment_id IS NULL`, t.ID, out.Payment.ID); err != nil {
		return Topup{}, err
	}
	return getTopup(ctx, s.db, t.ID)
}

// CancelTopup: the tenant withdraws a request that is still open.
func (s *Service) CancelTopup(ctx context.Context, merchantID, id string) error {
	tag, err := s.db.Exec(ctx, `UPDATE qrisgate_topups SET status = 'cancelled', decided_at = now()
		WHERE id = $1 AND merchant_id = $2 AND status = 'requested' AND method = 'manual'`, id, merchantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidState
	}
	return nil
}

func (s *Service) ListTopups(ctx context.Context, merchantID, status string, limit int) ([]Topup, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, topupFrom+`
		WHERE ($1 = '' OR t.merchant_id::text = $1) AND ($2 = '' OR t.status = $2) ORDER BY t.created_at DESC, t.id LIMIT $3`, merchantID, status, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Topup, error) { return scanTopup(r) })
}

// Decide confirms (bankRef = the transfer reference, required) or rejects (reason = note) an open request.
// Confirm and the ledger journal (Dr cash_bank_pt / Cr qrisgate_credit) commit together; the row lock makes
// concurrent decisions serial, and the ledger dedup key makes the credit post at most once.
func (s *Service) Decide(ctx context.Context, id, actorID, requestID string, confirm bool, bankRef, note string) (Topup, error) {
	br, nn := utf8.RuneCountInString(bankRef), utf8.RuneCountInString(note)
	if (confirm && (br < 3 || br > 128)) || (!confirm && nn < 3) || nn > 500 {
		return Topup{}, ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Topup{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var merchantID, status, requester, method string
	var amount int64
	err = tx.QueryRow(ctx, `SELECT merchant_id::text, amount, status, requested_by::text, method FROM qrisgate_topups WHERE id = $1 FOR UPDATE`, id).Scan(&merchantID, &amount, &status, &requester, &method)
	if errors.Is(err, pgx.ErrNoRows) || isBadUUID(err) {
		return Topup{}, ErrNotFound
	}
	if err != nil {
		return Topup{}, err
	}
	if status != "requested" || method != "manual" { // QRIS top-ups are settled by the payment itself, never by hand
		return Topup{}, ErrInvalidState
	}
	if requester == actorID {
		return Topup{}, ErrSelfDecision
	}
	to := "rejected"
	if confirm {
		to = "confirmed"
		if _, err := ledger.PostTx(ctx, tx, ledger.Txn{DedupKey: "qrisgate_topup:" + id, SourceType: "qrisgate_topup", SourceID: id, Currency: Currency,
			Entries: []ledger.Entry{
				{Account: ledger.CashBankPT, Direction: ledger.Debit, Amount: amount},
				{Account: ledger.QRISGateCredit, MerchantID: merchantID, Direction: ledger.Credit, Amount: amount}}}); err != nil {
			return Topup{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE qrisgate_topups SET status = $2, decided_by = $3::uuid, decided_at = now(),
		bank_reference = NULLIF($4,''), note = NULLIF($5,'') WHERE id = $1`, id, to, actorID, bankRef, note); err != nil {
		return Topup{}, err
	}
	t, err := getTopup(ctx, tx, id)
	if err != nil {
		return Topup{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "qrisgate_topup." + to, EntityType: "qrisgate_topup", EntityID: id,
		RequestID: requestID, Metadata: map[string]any{"merchant_id": merchantID, "amount": amount}}); err != nil {
		return Topup{}, err
	}
	return t, tx.Commit(ctx)
}

// ChargeFee debits the tenant's credit by the platform fee for one succeeded QRISGate payment, inside the
// caller's transaction (so it commits with the payment's own state change). Fee 0 (the default) does nothing.
// Idempotent per payment; the balance may go negative.
func (s *Service) ChargeFee(ctx context.Context, tx pgx.Tx, merchantID, paymentID string) (int64, error) {
	fee, err := s.settings.Int(ctx, platformsetting.QRISGateFee)
	if err != nil || fee == 0 {
		return 0, err
	}
	_, err = ledger.PostTx(ctx, tx, ledger.Txn{DedupKey: "qrisgate_fee:" + paymentID, SourceType: "qrisgate_fee", SourceID: paymentID, Currency: Currency,
		Entries: []ledger.Entry{
			{Account: ledger.QRISGateCredit, MerchantID: merchantID, Direction: ledger.Debit, Amount: fee},
			{Account: ledger.PlatformFeeRevenue, Direction: ledger.Credit, Amount: fee}}})
	return fee, err
}

// Movement is one line of the tenant's credit history: a top-up (credit) or a fee (debit).
type Movement struct {
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"` // qrisgate_topup | qrisgate_fee
	SourceID  string    `json:"source_id"`
	Direction string    `json:"direction"`
	Amount    int64     `json:"amount"`
}

func (s *Service) History(ctx context.Context, merchantID string, limit int) ([]Movement, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT t.created_at, t.source_type, t.source_id, e.direction, e.amount
		FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id JOIN ledger_transactions t ON t.id = e.txn_id
		WHERE a.merchant_id = $1 AND a.code = $2 AND a.currency = $3 ORDER BY t.created_at DESC, e.id LIMIT $4`,
		merchantID, ledger.QRISGateCredit, Currency, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Movement, error) {
		var m Movement
		return m, r.Scan(&m.At, &m.Kind, &m.SourceID, &m.Direction, &m.Amount)
	})
}

func isBadUUID(err error) bool {
	var pe interface{ SQLState() string }
	return errors.As(err, &pe) && pe.SQLState() == "22P02"
}

// ---- per-tenant QRISGate account (API key + webhook secret) ----

func aad(merchantID, field string) []byte { return []byte("qrisgate/" + merchantID + "/" + field) }

// Account is what the tenant may see: never the key or the secret, only a hint and the activation state.
type Account struct {
	Configured       bool   `json:"configured"`
	KeyHint          string `json:"key_hint,omitempty"`
	ActivationStatus string `json:"activation_status,omitempty"` // pending_confirmation | active | suspended; "" = no gateway account yet
	WebhookPath      string `json:"webhook_path"`
}

func (s *Service) Account(ctx context.Context, merchantID string) (Account, error) {
	a := Account{WebhookPath: "/webhooks/qrisgate/" + merchantID}
	err := s.db.QueryRow(ctx, `SELECT key_hint FROM qrisgate_accounts WHERE merchant_id = $1`, merchantID).Scan(&a.KeyHint)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	a.Configured = true
	err = s.db.QueryRow(ctx, `SELECT activation_status FROM merchant_gateway_accounts WHERE merchant_id = $1 AND gateway = 'qrisgate' AND environment = 'production'`, merchantID).Scan(&a.ActivationStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	return a, err
}

// SaveAccount stores the tenant's own key and webhook secret (encrypted) and makes sure a gateway account row
// exists in pending_confirmation: that row is the per-merchant flag, OFF until DEWA activates it.
func (s *Service) SaveAccount(ctx context.Context, merchantID, userID, requestID, apiKey, webhookSecret string) error {
	if s.box == nil {
		return ErrUnavailable
	}
	apiKey, webhookSecret = strings.TrimSpace(apiKey), strings.TrimSpace(webhookSecret)
	if !strings.HasPrefix(apiKey, "qg_live_") || len(apiKey) < 16 || len(apiKey) > 200 || len(webhookSecret) < 16 || len(webhookSecret) > 200 ||
		strings.ContainsAny(apiKey+webhookSecret, " \t\r\n") {
		return ErrInvalid
	}
	keyEnc, err := s.box.Seal([]byte(apiKey), aad(merchantID, "api_key"))
	if err != nil {
		return err
	}
	secEnc, err := s.box.Seal([]byte(webhookSecret), aad(merchantID, "webhook_secret"))
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var sandbox bool
	if err := tx.QueryRow(ctx, `SELECT is_sandbox FROM merchants WHERE id = $1`, merchantID).Scan(&sandbox); err != nil {
		return err
	}
	if sandbox { // QRISGate documents no sandbox: it only exists for real merchants
		return ErrUnavailable
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO qrisgate_accounts (merchant_id, api_key_enc, webhook_secret_enc, key_hint, updated_by)
		VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid)
		ON CONFLICT (merchant_id) DO UPDATE SET api_key_enc = EXCLUDED.api_key_enc, webhook_secret_enc = EXCLUDED.webhook_secret_enc,
		    key_hint = EXCLUDED.key_hint, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		merchantID, keyEnc, secEnc, "…"+apiKey[len(apiKey)-4:], userID); err != nil {
		return err
	}
	// gateway_ref = the merchant id: the adapter resolves this merchant's credentials from it.
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_gateway_accounts (merchant_id, gateway, environment, gateway_ref, activation_status)
		VALUES ($1::uuid, 'qrisgate', 'production', $2, 'pending_confirmation') ON CONFLICT (merchant_id, gateway, environment) DO NOTHING`, merchantID, merchantID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "qrisgate_account.save", EntityType: "merchant", EntityID: merchantID, RequestID: requestID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Credentials implements the adapter's CredentialSource.
func (s *Service) Credentials(ctx context.Context, merchantID string) (string, string, error) {
	if s.box == nil {
		return "", "", ErrNoAccount
	}
	var keyEnc, secEnc []byte
	err := s.db.QueryRow(ctx, `SELECT api_key_enc, webhook_secret_enc FROM qrisgate_accounts WHERE merchant_id::text = $1`, merchantID).Scan(&keyEnc, &secEnc)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNoAccount
	}
	if err != nil {
		return "", "", err
	}
	key, err := s.box.Open(keyEnc, aad(merchantID, "api_key"))
	if err != nil {
		return "", "", err
	}
	sec, err := s.box.Open(secEnc, aad(merchantID, "webhook_secret"))
	if err != nil {
		return "", "", err
	}
	return string(key), string(sec), nil
}
