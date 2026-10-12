// Package onboarding handles merchant self-sign-up and the platform review that follows.
//
// A signed-up merchant is 'submitted': payments and payouts already refuse any merchant that is
// not 'approved', so nothing here can move money. Approval is the one step that assigns fees.
package onboarding

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
)

var (
	ErrEmailTaken  = errors.New("email already registered")
	ErrRateLimited = errors.New("too many sign-ups from this address")
	ErrNotFound    = errors.New("merchant not found")
	ErrState       = errors.New("merchant is not awaiting review")
	ErrValidation  = errors.New("validation")
)

// ValidationError carries a message that is safe to show to the caller.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }
func (e *ValidationError) Unwrap() error { return ErrValidation }

func invalid(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

const (
	maxPasswordLen  = 128
	maxSignupsPerIP = 5
	signupWindow    = time.Hour
)

// reviewable are the states a platform admin may still approve or reject from.
var reviewable = []string{"submitted", "under_review", "action_required"}

// Statuses is every value of merchants.onboarding_status (used to validate list filters).
var Statuses = map[string]bool{
	"draft": true, "submitted": true, "under_review": true, "action_required": true,
	"approved": true, "rejected": true, "suspended": true,
}

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

type RegisterInput struct {
	AccountType  string // "individual" or "business"
	OwnerName    string
	BusinessName string // required for business
	Email        string
	Phone        string // national number, with or without +62 / 0
	Password     string
	AcceptTerms  bool
	IP           string
	RequestID    string
}

type Registered struct {
	MerchantID string `json:"merchant_id"`
	Status     string `json:"status"`
}

var phoneRe = regexp.MustCompile(`^8\d{8,11}$`)

func (in *RegisterInput) normalize() error {
	in.OwnerName = strings.TrimSpace(in.OwnerName)
	in.BusinessName = strings.TrimSpace(in.BusinessName)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))

	if in.AccountType != "individual" && in.AccountType != "business" {
		return invalid("account_type must be individual or business")
	}
	if n := utf8.RuneCountInString(in.OwnerName); n < 3 || n > 100 {
		return invalid("name must be 3-100 characters")
	}
	if in.AccountType == "business" {
		if n := utf8.RuneCountInString(in.BusinessName); n < 3 || n > 120 {
			return invalid("business_name must be 3-120 characters")
		}
	} else {
		in.BusinessName = in.OwnerName
	}
	if len(in.Email) > 254 {
		return invalid("email is invalid")
	}
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email || !strings.Contains(in.Email[strings.LastIndex(in.Email, "@"):], ".") {
		return invalid("email is invalid")
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, in.Phone)
	digits = strings.TrimPrefix(strings.TrimPrefix(digits, "62"), "0")
	if !phoneRe.MatchString(digits) {
		return invalid("phone is invalid")
	}
	in.Phone = "+62" + digits
	if err := validatePassword(in.Password); err != nil {
		return err
	}
	if !in.AcceptTerms {
		return invalid("terms must be accepted")
	}
	return nil
}

// validatePassword applies the platform password policy (auth.MinPasswordLen and up, length only).
// The upper bound keeps argon2id from being used as a CPU sink.
func validatePassword(pw string) error {
	if len(pw) < auth.MinPasswordLen || len(pw) > maxPasswordLen {
		return invalid("password must be %d-%d characters", auth.MinPasswordLen, maxPasswordLen)
	}
	return nil
}

// Register creates the user, the merchant (status 'submitted'), its first outlet, and the
// owner membership in one transaction. It does not log the caller in.
func (s *Service) Register(ctx context.Context, in RegisterInput) (Registered, error) {
	if err := in.normalize(); err != nil {
		return Registered{}, err
	}
	// Throttle before the (deliberately expensive) password hash. Failed and duplicate attempts count too.
	if err := s.reserveAttempt(ctx, in.IP); err != nil {
		return Registered{}, err
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return Registered{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Registered{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var uid, mid string
	if err := tx.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1,$2) RETURNING id::text`,
		in.Email, hash).Scan(&uid); err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return Registered{}, ErrEmailTaken
		}
		return Registered{}, err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO merchants (legal_name, display_name, onboarding_status, account_type, owner_name, contact_phone)
		VALUES ($1,$1,'submitted',$2,$3,$4) RETURNING id::text`,
		in.BusinessName, in.AccountType, in.OwnerName, in.Phone).Scan(&mid); err != nil {
		return Registered{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_outlets (merchant_id, name) VALUES ($1,'Outlet Utama')`, mid); err != nil {
		return Registered{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_users (merchant_id, user_id, role_id)
		SELECT $1, $2, id FROM roles WHERE code = 'merchant_owner'`, mid, uid); err != nil {
		return Registered{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: uid, Action: "merchant.registered", EntityType: "merchant",
		EntityID: mid, RequestID: in.RequestID, Metadata: map[string]any{"ip": in.IP, "account_type": in.AccountType}}); err != nil {
		return Registered{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Registered{}, err
	}
	return Registered{MerchantID: mid, Status: "submitted"}, nil
}

// reserveAttempt records the attempt in its own committed transaction (so rejected attempts still
// count) under a per-IP advisory lock, so parallel requests cannot slip past the limit.
func (s *Service) reserveAttempt(ctx context.Context, ip string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('register:i:'||$1, 0))`, ip); err != nil {
		return err
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM register_attempts WHERE ip = $1 AND created_at > now() - $2::interval`,
		ip, fmt.Sprintf("%d seconds", int(signupWindow.Seconds()))).Scan(&n); err != nil {
		return err
	}
	if n >= maxSignupsPerIP {
		return ErrRateLimited
	}
	if _, err := tx.Exec(ctx, `INSERT INTO register_attempts (ip) VALUES ($1)`, ip); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Merchant is the review view of a merchant (no secrets; owner email is needed to contact them).
type Merchant struct {
	ID           string     `json:"id"`
	LegalName    string     `json:"legal_name"`
	DisplayName  string     `json:"display_name"`
	AccountType  *string    `json:"account_type"`
	OwnerName    *string    `json:"owner_name"`
	OwnerEmail   *string    `json:"owner_email"`
	ContactPhone *string    `json:"contact_phone"`
	Status       string     `json:"status"`
	ReviewNote   *string    `json:"review_note"`
	ReviewedAt   *time.Time `json:"reviewed_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

const merchantCols = `m.id::text, m.legal_name, m.display_name, m.account_type, m.owner_name,
	(SELECT u.email FROM merchant_users mu JOIN users u ON u.id = mu.user_id
	   JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner'
	  WHERE mu.merchant_id = m.id ORDER BY u.created_at LIMIT 1),
	m.contact_phone, m.onboarding_status, m.review_note, m.reviewed_at, m.created_at`

func scanMerchant(row pgx.Row) (Merchant, error) {
	var m Merchant
	err := row.Scan(&m.ID, &m.LegalName, &m.DisplayName, &m.AccountType, &m.OwnerName, &m.OwnerEmail,
		&m.ContactPhone, &m.Status, &m.ReviewNote, &m.ReviewedAt, &m.CreatedAt)
	return m, err
}

// List returns merchants in one status. The pending queue is oldest-first; other lists newest-first.
func (s *Service) List(ctx context.Context, status string, limit int) ([]Merchant, error) {
	if !Statuses[status] {
		return nil, invalid("unknown status")
	}
	order := "m.created_at DESC"
	if status == "submitted" {
		order = "m.created_at ASC"
	}
	rows, err := s.db.Query(ctx, `SELECT `+merchantCols+` FROM merchants m WHERE m.onboarding_status = $1
		ORDER BY `+order+` LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Merchant{}
	for rows.Next() {
		m, err := scanMerchant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, id string) (Merchant, error) {
	m, err := scanMerchant(s.db.QueryRow(ctx, `SELECT `+merchantCols+` FROM merchants m WHERE m.id::text = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Merchant{}, ErrNotFound
	}
	return m, err
}

type ApproveInput struct {
	MerchantID  string
	ActorID     string
	MDRBPS      int // basis points, 100 = 1%
	PlatformBPS int
	RequestID   string
}

// Approve activates a merchant and assigns its QRIS pricing in one transaction. The merchant row
// is locked first, so concurrent or repeated approvals produce exactly one plan: the loser sees a
// non-reviewable status and gets ErrState.
func (s *Service) Approve(ctx context.Context, in ApproveInput) (Merchant, error) {
	if in.MDRBPS < 0 || in.MDRBPS > 10000 || in.PlatformBPS < 0 || in.PlatformBPS > 10000 {
		return Merchant{}, invalid("mdr_bps and platform_bps must be integers 0..10000")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := lockReviewable(ctx, tx, in.MerchantID); err != nil {
		return Merchant{}, err
	}
	var planID string
	if err := tx.QueryRow(ctx, `INSERT INTO pricing_plans
		(code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding, created_by, approved_by)
		VALUES ('qris-'||$1::text, 1, 'doku', 'QRIS', $2, 'half_up', $3, 'half_up', $4, $4) RETURNING id::text`,
		in.MerchantID, in.MDRBPS, in.PlatformBPS, in.ActorID).Scan(&planID); err != nil {
		return Merchant{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from, created_by)
		VALUES ($1,$2,now(),$3)`, in.MerchantID, planID, in.ActorID); err != nil {
		return Merchant{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET onboarding_status='approved', review_note=NULL, reviewed_by=$2,
		reviewed_at=now(), updated_at=now(), version=version+1 WHERE id=$1`, in.MerchantID, in.ActorID); err != nil {
		return Merchant{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant.approved", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID,
		Metadata: map[string]any{"plan_id": planID, "mdr_bps": in.MDRBPS, "platform_bps": in.PlatformBPS}}); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, err
	}
	return s.Get(ctx, in.MerchantID)
}

type RejectInput struct {
	MerchantID string
	ActorID    string
	Note       string
	RequestID  string
}

// Reject closes a pending application. The note is required so the applicant can be told why.
func (s *Service) Reject(ctx context.Context, in RejectInput) (Merchant, error) {
	in.Note = strings.TrimSpace(in.Note)
	if n := utf8.RuneCountInString(in.Note); n < 3 || n > 500 {
		return Merchant{}, invalid("note must be 3-500 characters")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := lockReviewable(ctx, tx, in.MerchantID); err != nil {
		return Merchant{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET onboarding_status='rejected', review_note=$3, reviewed_by=$2,
		reviewed_at=now(), updated_at=now(), version=version+1 WHERE id=$1`, in.MerchantID, in.ActorID, in.Note); err != nil {
		return Merchant{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant.rejected", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID}); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, err
	}
	return s.Get(ctx, in.MerchantID)
}

// lockReviewable row-locks the merchant and returns ErrNotFound or ErrState.
func lockReviewable(ctx context.Context, tx pgx.Tx, id string) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT onboarding_status FROM merchants WHERE id::text = $1 FOR NO KEY UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	for _, ok := range reviewable {
		if status == ok {
			return nil
		}
	}
	return ErrState
}
