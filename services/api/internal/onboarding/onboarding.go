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
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
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
	// ErrIdentityIncomplete: approving means the reviewers have verified the merchant's "data diri"
	// against what was submitted, so a complete identity is a precondition (DOKU-style, 14 Okt 2026).
	ErrIdentityIncomplete = errors.New("merchant identity data is not complete")
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
	// MDRCap / MDRCapUpTo: optional MDR cap for plans assigned at approval, e.g. the BI QRIS
	// rule (Rp500 for Rp0-Rp100.000, effective 1 Nov 2026). 0 = uncapped, so nothing changes
	// until the operator opts in (docs/integrations/doku.md §6).
	MDRCap     int64
	MDRCapUpTo int64
	// Box seals the owner's NIK / NPWP at rest (PRD §0 #12, data.md: PII dienkripsi). Without it,
	// sign-up stays off rather than storing strong identifiers in cleartext.
	Box *secretbox.Box
	Log *slog.Logger
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

// CanRegister reports whether sign-up is available: it needs a way to send the verification email
// and a key to seal the identity data at rest.
func (s *Service) CanRegister() bool { return s.notify != nil && s.cfg.Box != nil }

type RegisterInput struct {
	AccountType  string // "individual" or "business"
	OwnerName    string
	BusinessName string // required for business
	Email        string
	Phone        string // national number, with or without +62 / 0
	Password     string
	AcceptTerms  bool
	Identity     *IdentityInput // data diri (DOKU-style); optional. Absent → merchant starts as 'draft'.
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
var (
	// NIK is 16 digits (KTP); NPWP 15 (personal) or 16 (badan usaha) digits; NIB is 13 digits.
	nikRe  = regexp.MustCompile(`^\d{16}$`)
	npwpRe = regexp.MustCompile(`^\d{15,16}$`)
	nibRe  = regexp.MustCompile(`^\d{13}$`)
)

// IdentityInput is the "data diri" a merchant submits for verification, DOKU-style (decision
// 14 Okt 2026). It is OPTIONAL at sign-up (decision 15 Okt 2026): a merchant that registers with
// only basic account fields is created as 'draft' and completes the whole data diri afterwards
// via the KYC wizard. When provided, NIK is required for every account; business accounts
// additionally require the company NPWP and NIB. The owner's own NPWP is optional.
type IdentityInput struct {
	IDNumber     string `json:"id_number"` // NIK, 16 digits
	PlaceOfBirth string `json:"place_of_birth"`
	DateOfBirth  string `json:"date_of_birth"` // "2006-01-02"
	Gender       string `json:"gender"`        // "male" or "female"
	Address      string `json:"address"`
	NPWP         string `json:"npwp"`          // optional
	BusinessNPWP string `json:"business_npwp"` // required for business
	BusinessNIB  string `json:"business_nib"`  // required for business
}

// identity is the normalized form: digits-only numbers, trimmed text, parsed date.
type identity struct {
	idNumber     string
	placeOfBirth string
	dob          time.Time
	gender       string
	address      string
	npwp         string
	businessNPWP string
	businessNIB  string
}

func digitsOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// ageYears is the age at `now`; dates only, so it needs no time zone.
func ageYears(dob, now time.Time) int {
	age := now.Year() - dob.Year()
	if now.Month() < dob.Month() || (now.Month() == dob.Month() && now.Day() < dob.Day()) {
		age--
	}
	return age
}

// normalizeIdentity validates the submitted data diri. accountType decides which business fields
// are required; it comes from the merchant row (register input or the DB), never from the caller.
func normalizeIdentity(accountType *string, raw *IdentityInput) (identity, error) {
	var out identity
	if raw == nil {
		// Identity is optional at sign-up: an account may register with only the basic fields and
		// complete the data diri afterwards (decision 15 Okt 2026).
		return out, nil
	}
	out.idNumber = digitsOnly(raw.IDNumber)
	if !nikRe.MatchString(out.idNumber) {
		return out, invalid("id_number must be 16 digits")
	}
	out.placeOfBirth = strings.TrimSpace(raw.PlaceOfBirth)
	if n := utf8.RuneCountInString(out.placeOfBirth); n < 2 || n > 100 {
		return out, invalid("place_of_birth must be 2-100 characters")
	}
	dob, err := time.Parse("2006-01-02", strings.TrimSpace(raw.DateOfBirth))
	if err != nil {
		return out, invalid("date_of_birth must be a valid date in YYYY-MM-DD form")
	}
	// A KTP is only issued from age 17; 120 is the plausibility ceiling.
	if age := ageYears(dob, time.Now()); age < 17 || age > 120 {
		return out, invalid("date_of_birth must be between 17 and 120 years of age")
	}
	out.dob = dob
	out.gender = strings.ToLower(strings.TrimSpace(raw.Gender))
	if out.gender != "male" && out.gender != "female" {
		return out, invalid("gender must be male or female")
	}
	out.address = strings.TrimSpace(raw.Address)
	if n := utf8.RuneCountInString(out.address); n < 5 || n > 255 {
		return out, invalid("address must be 5-255 characters")
	}
	out.npwp = digitsOnly(raw.NPWP)
	if out.npwp != "" && !npwpRe.MatchString(out.npwp) {
		return out, invalid("npwp must be 15 or 16 digits")
	}
	business := accountType != nil && *accountType == "business"
	if business {
		out.businessNPWP = digitsOnly(raw.BusinessNPWP)
		if !npwpRe.MatchString(out.businessNPWP) {
			return out, invalid("business_npwp is required for a business account and must be 15 or 16 digits")
		}
		out.businessNIB = digitsOnly(raw.BusinessNIB)
		if !nibRe.MatchString(out.businessNIB) {
			return out, invalid("business_nib is required for a business account and must be 13 digits")
		}
	} else if strings.TrimSpace(raw.BusinessNPWP) != "" || strings.TrimSpace(raw.BusinessNIB) != "" {
		return out, invalid("business_npwp and business_nib are only for business accounts")
	}
	return out, nil
}

// sealNumber seals a strong identifier (NIK, NPWP) for storage and returns the last4 for the masked
// view. An empty value seals nothing; the column stays NULL.
func (s *Service) sealNumber(value, aad string) (sealed []byte, last4 string, err error) {
	if value == "" {
		return nil, "", nil
	}
	sealed, err = s.cfg.Box.Seal([]byte(value), []byte(aad))
	if err != nil {
		return nil, "", err
	}
	return sealed, value[len(value)-4:], nil
}

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

	// Daftar minimal (15 Okt 2026): hanya Nama, Email, Telpon, Password yang wajib dari pengguna.
	// Account type default "individual" bila tidak dikirim; nama usaha default nama pemilik.
	if in.AccountType == "" {
		in.AccountType = "individual"
	}
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

// Register creates the user, the merchant, its first outlet, and the owner membership in one
// transaction, then emails a verification link. It does not log the caller in. A merchant that
// signs up WITHOUT identity (daftar minimal, decision 15 Okt 2026) starts as 'draft' and completes
// the data diri afterwards via the KYC wizard; one that supplies identity starts as 'submitted'.
// If the email already has an account nothing is created and that address is told by email instead;
// the caller still gets the normal answer.
func (s *Service) Register(ctx context.Context, in RegisterInput) (Registered, error) {
	if s.notify == nil {
		return Registered{}, errors.New("onboarding: sign-up is not configured")
	}
	if s.cfg.Box == nil {
		return Registered{}, errors.New("onboarding: encryption key is not configured")
	}
	if err := in.normalize(); err != nil {
		return Registered{}, err
	}
	at := in.AccountType
	ident, err := normalizeIdentity(&at, in.Identity)
	if err != nil {
		return Registered{}, err
	}
	// Status awal: daftar minimal tanpa identity -> merchant draft (lengkapi data diri via KYC
	// wizard setelah login). Identity yang diisi penuh -> submitted (jalur lama & tes lama).
	initialStatus := "submitted"
	if in.Identity == nil {
		initialStatus = "draft"
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
	// The merchant id is generated first so the sealed NIK/NPWP can be bound to its row
	// (the secretbox AAD), exactly like a bank account number.
	if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&mid); err != nil {
		return Registered{}, err
	}
	idEnc, idLast4, err := s.sealNumber(ident.idNumber, mid)
	if err != nil {
		return Registered{}, err
	}
	npwpEnc, npwpLast4, err := s.sealNumber(ident.npwp, mid)
	if err != nil {
		return Registered{}, err
	}
	bizEnc, bizLast4, err := s.sealNumber(ident.businessNPWP, mid)
	if err != nil {
		return Registered{}, err
	}
	// Identity cells are NULL when the merchant signs up without data diri (daftar minimal):
	// the DOKU-style columns only exist once the KYC wizard is completed after login. Passing
	// empty strings here would violate the owner_gender CHECK and write a bogus date of birth.
	var placeArg, dobArg, genderArg, addrArg any = nil, nil, nil, nil
	if in.Identity != nil {
		placeArg, dobArg, genderArg, addrArg = ident.placeOfBirth, ident.dob, ident.gender, ident.address
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO merchants (id, legal_name, display_name, onboarding_status, account_type, owner_name, contact_phone,
			owner_id_enc, owner_id_last4, owner_place_of_birth, owner_date_of_birth, owner_gender, owner_address,
			owner_npwp_enc, owner_npwp_last4, business_npwp_enc, business_npwp_last4, business_nib)
		VALUES ($1,$2,$2,$17,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		mid, in.BusinessName, in.AccountType, in.OwnerName, in.Phone,
		idEnc, idLast4, placeArg, dobArg, genderArg, addrArg,
		npwpEnc, npwpLast4, bizEnc, bizLast4, ident.businessNIB, initialStatus); err != nil {
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
	IdentityComplete   bool       `json:"identity_complete"`
	Identity           *Identity  `json:"identity"`
	// Sealed NIK/NPWP, opened only by GetForReview for the reviewer detail view. Never serialized.
	idEnc   []byte
	npwpEnc []byte
	bizEnc  []byte
}

// Identity is the view of the merchant's "data diri". The masked fields (id_masked, npwp_masked,
// business_npwp_masked) are what every audience sees; id_number / npwp / business_npwp are filled
// only by GetForReview, the reviewer detail view (merchant:review), so reviewers can check what was
// submitted. Place/date of birth, gender, address and NIB are cleartext columns: manual review has
// to read them (PRD data.md: review masih manual dari data isian).
type Identity struct {
	IDMasked           string `json:"id_masked,omitempty"`
	IDNumber           string `json:"id_number,omitempty"`
	PlaceOfBirth       string `json:"place_of_birth,omitempty"`
	DateOfBirth        string `json:"date_of_birth,omitempty"`
	Gender             string `json:"gender,omitempty"`
	Address            string `json:"address,omitempty"`
	NPWPMasked         string `json:"npwp_masked,omitempty"`
	NPWP               string `json:"npwp,omitempty"`
	BusinessNPWPMasked string `json:"business_npwp_masked,omitempty"`
	BusinessNPWP       string `json:"business_npwp,omitempty"`
	BusinessNIB        string `json:"business_nib,omitempty"`
	Complete           bool   `json:"complete"`
}

const merchantCols = `m.id::text, m.legal_name, m.display_name, m.account_type, m.owner_name,
	(SELECT u.email FROM merchant_users mu JOIN users u ON u.id = mu.user_id
	   JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner'
	  WHERE mu.merchant_id = m.id ORDER BY u.created_at LIMIT 1),
	COALESCE((SELECT bool_or(u.email_verified_at IS NOT NULL) FROM merchant_users mu JOIN users u ON u.id = mu.user_id
	   JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner' WHERE mu.merchant_id = m.id), false),
	m.contact_phone, m.onboarding_status, m.review_note, m.reviewed_at, m.created_at,
	m.owner_place_of_birth, m.owner_date_of_birth, m.owner_gender, m.owner_address,
	m.owner_id_enc, m.owner_id_last4, m.owner_npwp_enc, m.owner_npwp_last4,
	m.business_npwp_enc, m.business_npwp_last4, m.business_nib`

func scanMerchant(row pgx.Row) (Merchant, error) {
	var m Merchant
	var place, gender, address, nib *string
	var dob *time.Time
	var idLast4, npwpLast4, bizLast4 *string
	err := row.Scan(&m.ID, &m.LegalName, &m.DisplayName, &m.AccountType, &m.OwnerName, &m.OwnerEmail, &m.OwnerEmailVerified,
		&m.ContactPhone, &m.Status, &m.ReviewNote, &m.ReviewedAt, &m.CreatedAt,
		&place, &dob, &gender, &address, &m.idEnc, &idLast4, &m.npwpEnc, &npwpLast4, &m.bizEnc, &bizLast4, &nib)
	m.Identity = m.identityView(place, dob, gender, address, idLast4, npwpLast4, bizLast4, nib)
	m.Identity.Complete = m.complete()
	m.IdentityComplete = m.Identity.Complete
	return m, err
}

// complete reports whether the data diri is sufficient for approval (business accounts also need
// the company NPWP and NIB). Approve re-checks the same rule from the database itself.
func (m Merchant) complete() bool {
	if m.Identity == nil || m.idEnc == nil {
		return false
	}
	if m.AccountType != nil && *m.AccountType == "business" {
		return m.bizEnc != nil && m.Identity.BusinessNIB != ""
	}
	return true
}

// identityView builds the masked view from the stored columns. Plaintext fields stay empty here;
// GetForReview opens the sealed values into them.
func (m Merchant) identityView(place *string, dob *time.Time, gender, address, idLast4, npwpLast4, bizLast4, nib *string) *Identity {
	v := &Identity{}
	if place != nil {
		v.PlaceOfBirth = *place
	}
	if dob != nil {
		v.DateOfBirth = dob.Format("2006-01-02")
	}
	if gender != nil {
		v.Gender = *gender
	}
	if address != nil {
		v.Address = *address
	}
	if nib != nil {
		v.BusinessNIB = *nib
	}
	// An empty last4 means nothing was stored for that identifier ("" is written when the
	// value is absent), so no mask must be shown for it: "••••" with no digits would read
	// as an identifier that exists.
	if idLast4 != nil && *idLast4 != "" {
		v.IDMasked = "••••" + *idLast4
	}
	if npwpLast4 != nil && *npwpLast4 != "" {
		v.NPWPMasked = "••••" + *npwpLast4
	}
	if bizLast4 != nil && *bizLast4 != "" {
		v.BusinessNPWPMasked = "••••" + *bizLast4
	}
	return v
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

// GetForReview is the reviewer detail view: it additionally opens the sealed NIK/NPWP so Finance/Ops
// can check the submitted data diri. Everything else (list, the merchant's own view) sees masks only.
func (s *Service) GetForReview(ctx context.Context, id string) (Merchant, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return m, err
	}
	if s.cfg.Box == nil {
		return m, invalid("identity data cannot be read: encryption key is not configured")
	}
	if v, err := s.cfg.Box.Open(m.idEnc, []byte(m.ID)); err == nil {
		m.Identity.IDNumber = string(v)
	}
	if m.npwpEnc != nil {
		if v, err := s.cfg.Box.Open(m.npwpEnc, []byte(m.ID)); err == nil {
			m.Identity.NPWP = string(v)
		}
	}
	if m.bizEnc != nil {
		if v, err := s.cfg.Box.Open(m.bizEnc, []byte(m.ID)); err == nil {
			m.Identity.BusinessNPWP = string(v)
		}
	}
	return m, nil
}

type ApproveInput struct {
	MerchantID  string
	ActorID     string
	MDRBPS      int // basis points, 100 = 1%
	PlatformBPS int
	// Biaya tetap per transaksi (rupiah, integer). Tiap komponen (MDR, platform) boleh persen, nominal, atau keduanya.
	MDRFixed      int64
	PlatformFixed int64
	RequestID     string
}

// MaxFixedFee membatasi biaya tetap agar salah ketik (mis. tambahan nol) tidak lolos: Rp 1.000.000.
const MaxFixedFee int64 = 1_000_000

// Approve activates a merchant and assigns its QRIS pricing in one transaction. The merchant row
// is locked first, so concurrent or repeated approvals produce exactly one plan: the loser sees a
// non-reviewable status and gets ErrState.
func (s *Service) Approve(ctx context.Context, in ApproveInput) (Merchant, error) {
	if in.MDRBPS < 0 || in.MDRBPS > 10000 || in.PlatformBPS < 0 || in.PlatformBPS > 10000 {
		return Merchant{}, invalid("mdr_bps and platform_bps must be integers 0..10000")
	}
	if in.MDRFixed < 0 || in.MDRFixed > MaxFixedFee || in.PlatformFixed < 0 || in.PlatformFixed > MaxFixedFee {
		return Merchant{}, invalid("mdr_fixed and platform_fixed must be integer rupiah 0..%d", MaxFixedFee)
	}
	if s.cfg.MDRCap < 0 || s.cfg.MDRCapUpTo < 0 {
		return Merchant{}, invalid("mdr_cap and mdr_cap_up_to must be non-negative")
	}
	if s.cfg.MDRCapUpTo > 0 && s.cfg.MDRCap == 0 {
		return Merchant{}, invalid("mdr_cap_up_to requires mdr_cap > 0")
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
	// The data diri is verified against what was submitted, so it must be complete first. The rule
	// is read from the database, not the request: business accounts additionally need NPWP + NIB.
	// Only the fields both onboarding paths collect are required: the KYC wizard (internal/kyc)
	// stores NIK, NPWP and NIB but not birth data, gender or a personal address.
	var complete bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(
		owner_id_enc IS NOT NULL
		AND (account_type IS DISTINCT FROM 'business' OR (business_npwp_enc IS NOT NULL AND business_nib IS NOT NULL))
	, false) FROM merchants WHERE id = $1`, in.MerchantID).Scan(&complete); err != nil {
		return Merchant{}, err
	}
	if !complete {
		return Merchant{}, ErrIdentityIncomplete
	}
	var planID string
	if err := tx.QueryRow(ctx, `INSERT INTO pricing_plans
		(code, version, gateway, payment_method, mdr_bps, mdr_fixed, mdr_rounding, platform_bps, platform_fixed, platform_rounding, mdr_cap, mdr_cap_up_to, created_by, approved_by)
		SELECT 'qris-'||$1::text, COALESCE(max(version),0)+1, 'doku', 'QRIS', $2, $7, 'half_up', $3, $8, 'half_up', $5, $6, $4, $4
		FROM pricing_plans WHERE code = 'qris-'||$1::text RETURNING id::text`,
		in.MerchantID, in.MDRBPS, in.PlatformBPS, in.ActorID, s.cfg.MDRCap, s.cfg.MDRCapUpTo, in.MDRFixed, in.PlatformFixed).Scan(&planID); err != nil {
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
		Metadata: map[string]any{"plan_id": planID, "mdr_bps": in.MDRBPS, "platform_bps": in.PlatformBPS,
			"mdr_fixed": in.MDRFixed, "platform_fixed": in.PlatformFixed}}); err != nil {
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

type RevokeInput struct {
	MerchantID string
	ActorID    string
	To         string // "under_review" | "rejected"
	Note       string // alasan, wajib (masuk audit dan dibaca merchant)
	RequestID  string
}

// Revoke mencabut persetujuan: approved -> under_review | rejected. Gerbang pembayaran membaca
// onboarding_status, jadi transaksi baru berhenti seketika; dana/ledger yang sudah ada tidak disentuh.
// Plan harga lama tetap tersimpan; persetujuan ulang membuat plan versi berikutnya.
func (s *Service) Revoke(ctx context.Context, in RevokeInput) (Merchant, error) {
	if in.To != "under_review" && in.To != "rejected" {
		return Merchant{}, invalid("to must be under_review or rejected")
	}
	in.Note = strings.TrimSpace(in.Note)
	if n := utf8.RuneCountInString(in.Note); n < 3 || n > 500 {
		return Merchant{}, invalid("note must be 3-500 characters")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := lockIn(ctx, tx, in.MerchantID, []string{"approved"}); err != nil {
		return Merchant{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET onboarding_status=$3, review_note=$4, reviewed_by=$2,
		reviewed_at=now(), updated_at=now(), version=version+1 WHERE id=$1`,
		in.MerchantID, in.ActorID, in.To, in.Note); err != nil {
		return Merchant{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant.approval_revoked", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID, Metadata: map[string]any{"to": in.To, "note": in.Note}}); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, err
	}
	return s.Get(ctx, in.MerchantID)
}

type SuspendInput struct {
	MerchantID string
	ActorID    string
	Note       string // alasan (wajib; dibaca merchant dan masuk audit)
	RequestID  string
}

// Suspend menangguhkan merchant: submitted/under_review/action_required/approved -> suspended.
// Berbeda dari Reject (penolakan yang bisa diajukan ulang), penangguhan menghentikan transaksi
// baru dan mengunci data sampai admin mengaktifkannya kembali (Reactivate). Dana/ledger tidak disentuh.
func (s *Service) Suspend(ctx context.Context, in SuspendInput) (Merchant, error) {
	in.Note = strings.TrimSpace(in.Note)
	if n := utf8.RuneCountInString(in.Note); n < 3 || n > 500 {
		return Merchant{}, invalid("note must be 3-500 characters")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	from := append(append([]string{}, reviewable...), "approved")
	if err := lockIn(ctx, tx, in.MerchantID, from); err != nil {
		return Merchant{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET onboarding_status='suspended', review_note=$3, reviewed_by=$2,
		reviewed_at=now(), updated_at=now(), version=version+1 WHERE id=$1`, in.MerchantID, in.ActorID, in.Note); err != nil {
		return Merchant{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant.suspended", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID, Metadata: map[string]any{"note": in.Note}}); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, err
	}
	return s.Get(ctx, in.MerchantID)
}

// Reactivate mengakhiri penangguhan: kembali approved bila merchant pernah disetujui (punya plan harga),
// selain itu kembali under_review agar diputuskan lewat alur biasa. Alasan wajib.
func (s *Service) Reactivate(ctx context.Context, in SuspendInput) (Merchant, error) {
	in.Note = strings.TrimSpace(in.Note)
	if n := utf8.RuneCountInString(in.Note); n < 3 || n > 500 {
		return Merchant{}, invalid("note must be 3-500 characters")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := lockIn(ctx, tx, in.MerchantID, []string{"suspended"}); err != nil {
		return Merchant{}, err
	}
	var to string
	if err := tx.QueryRow(ctx, `SELECT CASE WHEN EXISTS (SELECT 1 FROM merchant_pricing_assignments WHERE merchant_id = $1)
		THEN 'approved' ELSE 'under_review' END`, in.MerchantID).Scan(&to); err != nil {
		return Merchant{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET onboarding_status=$3, review_note=NULL, reviewed_by=$2,
		reviewed_at=now(), updated_at=now(), version=version+1 WHERE id=$1`, in.MerchantID, in.ActorID, to); err != nil {
		return Merchant{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant.reactivated", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID, Metadata: map[string]any{"note": in.Note, "to": to}}); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, err
	}
	return s.Get(ctx, in.MerchantID)
}

type VerifyEmailInput struct {
	MerchantID string
	ActorID    string
	Note       string // alasan/bukti verifikasi manual (wajib, masuk audit)
	RequestID  string
}

// VerifyOwnerEmail menandai email pemilik merchant sebagai terverifikasi oleh admin (tanpa tautan email),
// untuk kasus email tak sampai. Idempoten: yang sudah terverifikasi tidak diubah. Sesi yang berjalan
// membaca status ini dari DB, jadi berlaku seketika.
func (s *Service) VerifyOwnerEmail(ctx context.Context, in VerifyEmailInput) (Merchant, error) {
	in.Note = strings.TrimSpace(in.Note)
	if n := utf8.RuneCountInString(in.Note); n < 3 || n > 500 {
		return Merchant{}, invalid("note must be 3-500 characters")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var status string
	if err := tx.QueryRow(ctx, `SELECT onboarding_status FROM merchants WHERE id::text = $1 FOR NO KEY UPDATE`, in.MerchantID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Merchant{}, ErrNotFound
		}
		return Merchant{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET email_verified_at = now(), updated_at = now()
		WHERE email_verified_at IS NULL AND id IN (
			SELECT mu.user_id FROM merchant_users mu JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner'
			WHERE mu.merchant_id::text = $1)`, in.MerchantID)
	if err != nil {
		return Merchant{}, err
	}
	if tag.RowsAffected() > 0 {
		if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant.email_verified_manually", EntityType: "merchant",
			EntityID: in.MerchantID, RequestID: in.RequestID, Metadata: map[string]any{"note": in.Note}}); err != nil {
			return Merchant{}, err
		}
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

type SetIdentityInput struct {
	MerchantID string // always from the session, never from the request
	ActorID    string
	Identity   *IdentityInput
	RequestID  string
}

// SetIdentity lets the merchant complete or correct the data diri used for verification: at
// sign-up (already filled), later through the Status akun menu, or after a change request. Once a
// merchant is decided (approved / rejected / suspended) the data is locked, exactly like resubmit.
func (s *Service) SetIdentity(ctx context.Context, in SetIdentityInput) (Merchant, error) {
	if s.cfg.Box == nil {
		return Merchant{}, errors.New("onboarding: encryption key is not configured")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := lockIn(ctx, tx, in.MerchantID, reviewable); err != nil {
		return Merchant{}, err
	}
	// The rules for required fields follow the merchant's own account type, never the request.
	var accountType *string
	if err := tx.QueryRow(ctx, `SELECT account_type FROM merchants WHERE id = $1`, in.MerchantID).Scan(&accountType); err != nil {
		return Merchant{}, err
	}
	ident, err := normalizeIdentity(accountType, in.Identity)
	if err != nil {
		return Merchant{}, err
	}
	idEnc, idLast4, err := s.sealNumber(ident.idNumber, in.MerchantID)
	if err != nil {
		return Merchant{}, err
	}
	npwpEnc, npwpLast4, err := s.sealNumber(ident.npwp, in.MerchantID)
	if err != nil {
		return Merchant{}, err
	}
	bizEnc, bizLast4, err := s.sealNumber(ident.businessNPWP, in.MerchantID)
	if err != nil {
		return Merchant{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE merchants SET
		owner_id_enc=$2, owner_id_last4=$3, owner_place_of_birth=$4, owner_date_of_birth=$5,
		owner_gender=$6, owner_address=$7, owner_npwp_enc=$8, owner_npwp_last4=$9,
		business_npwp_enc=$10, business_npwp_last4=$11, business_nib=$12,
		updated_at=now(), version=version+1 WHERE id=$1`,
		in.MerchantID, idEnc, idLast4, ident.placeOfBirth, ident.dob, ident.gender, ident.address,
		npwpEnc, npwpLast4, bizEnc, bizLast4, ident.businessNIB); err != nil {
		return Merchant{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant.identity_updated", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID,
		// Never the plaintext: only the completeness and the masked tail, like bank account events.
		Metadata: map[string]any{"id_last4": idLast4, "complete": true}}); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Merchant{}, err
	}
	return s.Get(ctx, in.MerchantID)
}
