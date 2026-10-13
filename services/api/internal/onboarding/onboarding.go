// Package onboarding handles merchant self-sign-up and the platform review that follows.
//
// A signed-up merchant is 'submitted': payments and payouts already refuse any merchant that is
// not 'approved', so nothing here can move money. Approval is the one step that assigns fees.
package onboarding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/throttle"
)

var (
	ErrEmailTaken  = errors.New("email already registered")
	ErrRateLimited = errors.New("too many sign-ups from this address")
	ErrNotFound    = errors.New("merchant not found")
	ErrState       = errors.New("merchant is not awaiting review")
	ErrValidation  = errors.New("validation")
	// ErrEmailUnverified: approving means trusting the owner's contact details, so the address must be proven first.
	ErrEmailUnverified          = errors.New("owner email is not verified")
	ErrEmailDeliveryUnavailable = errors.New("owner email delivery is not configured")
	ErrARUSMerchantAmbiguous    = errors.New("multiple existing merchants match this ARUS tenant")
)

// ValidationError carries a message that is safe to show to the caller.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }
func (e *ValidationError) Unwrap() error { return ErrValidation }

func invalid(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

const signupWindow = time.Hour

// reviewable are the states a platform admin may still approve or reject from.
var reviewable = []string{"submitted", "under_review", "action_required"}

// Statuses is every value of merchants.onboarding_status (used to validate list filters).
var Statuses = map[string]bool{
	"draft": true, "submitted": true, "under_review": true, "action_required": true,
	"approved": true, "rejected": true, "suspended": true,
}

// Notifier sends the sign-up emails (implemented by account.Service). With no notifier, sign-up is
// disabled: a new merchant could never prove its address, so it would be stuck.
type Notifier interface {
	SendVerification(ctx context.Context, userID, email string) error
	SendAlreadyRegistered(ctx context.Context, email string) error
}

type Config struct {
	RegisterPerIP int // sign-up attempts per IP per hour; default 5
	Log           *slog.Logger
}

type Service struct {
	db     *pgxpool.Pool
	notify Notifier
	cfg    Config
}

func NewService(db *pgxpool.Pool, n Notifier, cfg Config) *Service {
	if cfg.RegisterPerIP <= 0 {
		cfg.RegisterPerIP = 5
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Service{db: db, notify: n, cfg: cfg}
}

// CanRegister reports whether sign-up is available (needs a way to send the verification email).
func (s *Service) CanRegister() bool { return s.notify != nil }

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

// Registered is deliberately identical for a new and an already-registered address, so sign-up cannot
// be used to find out which emails have accounts. MerchantID is for internal use and never serialized.
type Registered struct {
	Status     string `json:"status"` // always "check_email"
	MerchantID string `json:"-"`
}

var phoneRe = regexp.MustCompile(`^8\d{8,11}$`)

// normalizePhone turns "0812 3456 7890", "+62 812-3456-7890" and "81234567890" into "+6281234567890".
func normalizePhone(raw string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	digits = strings.TrimPrefix(strings.TrimPrefix(digits, "62"), "0")
	if !phoneRe.MatchString(digits) {
		return "", invalid("phone is invalid")
	}
	return "+62" + digits, nil
}

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
	phone, err := normalizePhone(in.Phone)
	if err != nil {
		return err
	}
	in.Phone = phone
	if err := validatePassword(in.Password); err != nil {
		return err
	}
	if !in.AcceptTerms {
		return invalid("terms must be accepted")
	}
	return nil
}

func validatePassword(pw string) error {
	if err := auth.ValidateNewPassword(pw); err != nil {
		return invalid("%s", err.Error())
	}
	return nil
}

// Register creates the user, the merchant (status 'submitted'), its first outlet, and the owner
// membership in one transaction, then emails a verification link. It does not log the caller in.
// If the email already has an account nothing is created and that address is told by email instead;
// the caller still gets the normal answer.
func (s *Service) Register(ctx context.Context, in RegisterInput) (Registered, error) {
	if s.notify == nil {
		return Registered{}, errors.New("onboarding: sign-up is not configured")
	}
	if err := in.normalize(); err != nil {
		return Registered{}, err
	}
	// Throttle before the (deliberately expensive) password hash. Failed and duplicate attempts count too.
	if _, err := throttle.Reserve(ctx, s.db, "register", in.IP, "", throttle.Limits{PerIP: s.cfg.RegisterPerIP, Window: signupWindow}); err != nil {
		if errors.Is(err, throttle.ErrIPLimited) {
			return Registered{}, ErrRateLimited
		}
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
			tx.Rollback(ctx) //nolint:errcheck
			if err := s.notify.SendAlreadyRegistered(ctx, in.Email); err != nil {
				s.cfg.Log.Error("already-registered notice not sent", "err", err)
			}
			return Registered{Status: "check_email"}, nil
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
	// The account exists even if the email fails; the owner can ask for another link.
	if err := s.notify.SendVerification(ctx, uid, in.Email); err != nil {
		s.cfg.Log.Error("verification email not sent", "err", err)
	}
	return Registered{Status: "check_email", MerchantID: mid}, nil
}

// Merchant is the review view of a merchant (no secrets; owner email is needed to contact them).
type Merchant struct {
	ID                 string     `json:"id"`
	LegalName          string     `json:"legal_name"`
	DisplayName        string     `json:"display_name"`
	AccountType        *string    `json:"account_type"`
	OwnerName          *string    `json:"owner_name"`
	OwnerEmail         *string    `json:"owner_email"`
	OwnerEmailVerified bool       `json:"owner_email_verified"`
	ContactPhone       *string    `json:"contact_phone"`
	Status             string     `json:"status"`
	ReviewNote         *string    `json:"review_note"`
	ReviewedAt         *time.Time `json:"reviewed_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

const merchantCols = `m.id::text, m.legal_name, m.display_name, m.account_type, m.owner_name,
	(SELECT u.email FROM merchant_users mu JOIN users u ON u.id = mu.user_id
	   JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner'
	  WHERE mu.merchant_id = m.id ORDER BY u.created_at LIMIT 1),
	COALESCE((SELECT bool_or(u.email_verified_at IS NOT NULL) FROM merchant_users mu JOIN users u ON u.id = mu.user_id
	   JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner' WHERE mu.merchant_id = m.id), false),
	m.contact_phone, m.onboarding_status, m.review_note, m.reviewed_at, m.created_at`

func scanMerchant(row pgx.Row) (Merchant, error) {
	var m Merchant
	err := row.Scan(&m.ID, &m.LegalName, &m.DisplayName, &m.AccountType, &m.OwnerName, &m.OwnerEmail, &m.OwnerEmailVerified,
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
	if err := lockIn(ctx, tx, in.MerchantID, reviewable); err != nil {
		return Merchant{}, err
	}
	var verified bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(bool_or(u.email_verified_at IS NOT NULL), false) FROM merchant_users mu
		JOIN users u ON u.id = mu.user_id JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner'
		WHERE mu.merchant_id = $1`, in.MerchantID).Scan(&verified); err != nil {
		return Merchant{}, err
	}
	if !verified {
		return Merchant{}, ErrEmailUnverified
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
	if err := lockIn(ctx, tx, in.MerchantID, reviewable); err != nil {
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

// lockIn row-locks the merchant and returns ErrNotFound, or ErrState unless its status is in allowed.
func lockIn(ctx context.Context, tx pgx.Tx, id string, allowed []string) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT onboarding_status FROM merchants WHERE id::text = $1 FOR NO KEY UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	for _, ok := range allowed {
		if status == ok {
			return nil
		}
	}
	return ErrState
}

type ReviewInput struct {
	MerchantID string
	ActorID    string
	Note       string
	RequestID  string
}

// StartReview marks a submitted application as being looked at (so two reviewers do not duplicate work).
func (s *Service) StartReview(ctx context.Context, in ReviewInput) (Merchant, error) {
	return s.move(ctx, in, []string{"submitted"}, "under_review", "merchant.review_started", false)
}

// RequestChanges sends the application back to the applicant with a required explanation.
func (s *Service) RequestChanges(ctx context.Context, in ReviewInput) (Merchant, error) {
	return s.move(ctx, in, []string{"submitted", "under_review"}, "action_required", "merchant.changes_requested", true)
}

func (s *Service) move(ctx context.Context, in ReviewInput, from []string, to, action string, noteRequired bool) (Merchant, error) {
	in.Note = strings.TrimSpace(in.Note)
	if n := utf8.RuneCountInString(in.Note); noteRequired && (n < 3 || n > 500) {
		return Merchant{}, invalid("note must be 3-500 characters")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := lockIn(ctx, tx, in.MerchantID, from); err != nil {
		return Merchant{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET onboarding_status=$3, review_note=NULLIF($4,''), reviewed_by=$2,
		reviewed_at=now(), updated_at=now(), version=version+1 WHERE id=$1`, in.MerchantID, in.ActorID, to, in.Note); err != nil {
		return Merchant{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: action, EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID}); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, err
	}
	return s.Get(ctx, in.MerchantID)
}

type ResubmitInput struct {
	MerchantID   string // always from the session, never from the request
	ActorID      string
	LegalName    *string // optional corrections
	ContactPhone *string
	RequestID    string
}

// Resubmit lets the applicant answer a change request: optionally correct the legal name and phone,
// and put the application back in the queue.
func (s *Service) Resubmit(ctx context.Context, in ResubmitInput) (Merchant, error) {
	var phone *string
	if in.ContactPhone != nil && strings.TrimSpace(*in.ContactPhone) != "" {
		p, err := normalizePhone(*in.ContactPhone)
		if err != nil {
			return Merchant{}, err
		}
		phone = &p
	}
	var legal *string
	if in.LegalName != nil {
		if l := strings.TrimSpace(*in.LegalName); l != "" {
			if n := utf8.RuneCountInString(l); n < 3 || n > 120 {
				return Merchant{}, invalid("legal_name must be 3-120 characters")
			}
			legal = &l
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := lockIn(ctx, tx, in.MerchantID, []string{"action_required"}); err != nil {
		return Merchant{}, err
	}
	// Right-hand sides see the old row, so display_name follows legal_name only if they were the same.
	if _, err := tx.Exec(ctx, `UPDATE merchants SET onboarding_status='submitted', review_note=NULL,
		display_name = CASE WHEN display_name = legal_name THEN COALESCE($2, display_name) ELSE display_name END,
		legal_name = COALESCE($2, legal_name), contact_phone = COALESCE($3, contact_phone),
		updated_at=now(), version=version+1 WHERE id=$1`, in.MerchantID, legal, phone); err != nil {
		return Merchant{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant.resubmitted", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID}); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, err
	}
	return s.Get(ctx, in.MerchantID)
}
