package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrRateLimited        = errors.New("too many login attempts")
	ErrMerchantRequired   = errors.New("merchant_id required: user belongs to several merchants")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrForbidden          = errors.New("forbidden")
	ErrInvalidInput       = errors.New("invalid input")
)

// ErrPlatformSetupDone is returned when a DEWA already exists and the bootstrap
// endpoint is called again. Bootstrap is a one-time, irreversible step.
var ErrPlatformSetupDone = errors.New("platform setup already done")

// RateLimitedError carries how long the caller must wait. errors.Is(err, ErrRateLimited) holds.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string   { return ErrRateLimited.Error() }
func (e *RateLimitedError) Is(t error) bool { return t == ErrRateLimited }

// BadCredentialsError carries the failed attempts left before lockout. errors.Is(err, ErrInvalidCredentials) holds.
type BadCredentialsError struct{ AttemptsLeft int }

func (e *BadCredentialsError) Error() string   { return ErrInvalidCredentials.Error() }
func (e *BadCredentialsError) Is(t error) bool { return t == ErrInvalidCredentials }

// MFARequiredError indicates that MFA verification is needed before login can complete.
type MFARequiredError struct {
	UserID      string
	ChallengeID string
	ExpiresAt   time.Time
}

func (e *MFARequiredError) Error() string   { return ErrMFARequired.Error() }
func (e *MFARequiredError) Is(t error) bool { return t == ErrMFARequired }

type Config struct {
	SessionTTL   time.Duration // absolute lifetime
	IdleTTL      time.Duration // max gap between requests
	AttemptWin   time.Duration // rate-limit window
	MaxFailEmail int           // failed attempts per email within window
	MaxFailIP    int           // failed attempts per IP within window
}

func DefaultConfig() Config {
	return Config{SessionTTL: 12 * time.Hour, IdleTTL: 30 * time.Minute,
		AttemptWin: 15 * time.Minute, MaxFailEmail: 5, MaxFailIP: 20}
}

// Principal is the authenticated identity. MerchantID is the ONLY source of
// tenant identity for requests (PRD §0 #7); it is empty for platform sessions.
type Principal struct {
	UserID        string
	Email         string
	SessionID     string
	KeyID         string // set when authenticated by API key (UserID/SessionID empty)
	MerchantID    string
	EmailVerified bool
	Permissions   map[string]struct{}
}

func (p *Principal) Has(perm string) bool { _, ok := p.Permissions[perm]; return ok }

type Service struct {
	db  *pgxpool.Pool
	cfg Config
	now func() time.Time

	dummyOnce sync.Once
	dummyHash string
}

func NewService(db *pgxpool.Pool, cfg Config) *Service {
	return &Service{db: db, cfg: cfg, now: time.Now}
}

type LoginInput struct {
	Email      string
	Password   string
	MerchantID string // required only when the user belongs to several merchants
	IP         string
	UserAgent  string
	RequestID  string
}

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func emailKey(e string) string {
	h := sha256.Sum256([]byte(normEmail(e)))
	return hex.EncodeToString(h[:])
}

// ClearFailedAttempts removes recorded failed logins for an email within a
// transaction. A successful password reset proves mailbox ownership, so stale
// failures must not keep locking the user out of their own account.
func ClearFailedAttempts(ctx context.Context, tx pgx.Tx, email string) error {
	_, err := tx.Exec(ctx, `DELETE FROM auth_attempts WHERE NOT succeeded AND email_key = $1`, emailKey(email))
	return err
}

func hashToken(tok string) []byte { h := sha256.Sum256([]byte(tok)); return h[:] }

// CreateUser inserts a user with an argon2id hash (bootstrap/tests; no public signup).
func (s *Service) CreateUser(ctx context.Context, email, password string) (string, error) {
	h, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	var id string
	err = s.db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1,$2) RETURNING id::text`,
		normEmail(email), h).Scan(&id)
	return id, err
}

// PlatformAdminExists reports whether a DEWA (platform RBAC manager) already
// exists. The panel exposes /platform/setup until this becomes true.
func (s *Service) PlatformAdminExists(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE r.code = 'dewa')`,
	).Scan(&exists)
	return exists, err
}

// CreatePlatformAdmin is the one-time bootstrap that creates the first DEWA.
// The operator vouches for the address by presenting ERAPAY_PLATFORM_SETUP_TOKEN,
// so the email is considered verified. It is atomic and guarded by an advisory
// lock, so two parallel requests cannot both succeed.
func (s *Service) CreatePlatformAdmin(ctx context.Context, email, password, requestID string) (string, error) {
	if err := ValidateNewPassword(password); err != nil {
		return "", err
	}
	email = normEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return "", ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('platform:setup', 0))`); err != nil {
		return "", err
	}
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE r.code = 'dewa')`,
	).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return "", ErrPlatformSetupDone
	}
	h, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	var uid string
	if err := tx.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, email_verified_at) VALUES ($1,$2,now()) RETURNING id::text`,
		email, h).Scan(&uid); err != nil {
		return "", err
	}
	// Role dewa comes from migration 0016; fail loudly if it is ever missing.
	var rid string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM roles WHERE code = 'dewa' AND scope = 'platform'`).Scan(&rid); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2)`, uid, rid); err != nil {
		return "", err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: uid, Action: "bootstrap.platform_dewa",
		EntityType: "user", EntityID: uid, RequestID: requestID}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return uid, nil
}

// Login verifies credentials and opens a session. It returns the opaque session
// token (shown to the caller once; only its sha256 is stored).
// If the user has a platform admin role requiring MFA and MFA is enrolled,
// it returns an MFARequiredError with a challenge instead of a session.
func (s *Service) Login(ctx context.Context, in LoginInput) (string, *Principal, error) {
	ek := emailKey(in.Email)
	attemptID, left, err := s.reserveAttempt(ctx, ek, in.IP)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			s.audit(ctx, "", "auth.login.rate_limited", in, ek)
		}
		return "", nil, err
	}

	uid, hash, status, err := s.lookupUser(ctx, in.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		VerifyPassword(in.Password, s.dummy()) // equalize timing; result ignored
		s.audit(ctx, "", "auth.login.failed", in, ek)
		return "", nil, &BadCredentialsError{left}
	}
	if err != nil {
		return "", nil, err
	}
	if !VerifyPassword(in.Password, hash) || status != "active" {
		s.audit(ctx, uid, "auth.login.failed", in, ek)
		return "", nil, &BadCredentialsError{left}
	}

	merchantID, err := s.pickMerchant(ctx, uid, in.MerchantID)
	if err != nil {
		s.audit(ctx, uid, "auth.login.failed", in, ek)
		return "", nil, err
	}

	// Check if this is a platform admin session that requires MFA
	isPlatformAdmin := merchantID == ""
	var mfaEnrolled bool
	if isPlatformAdmin {
		err = s.db.QueryRow(ctx,
			`SELECT mfa_secret IS NOT NULL FROM users WHERE id = $1`, uid).Scan(&mfaEnrolled)
		if err != nil {
			return "", nil, err
		}
	}

	// If platform admin with MFA enrolled, require MFA challenge
	if isPlatformAdmin && mfaEnrolled {
		// Generate MFA challenge
		challengeID, _, err := s.GenerateMFAChallenge(ctx, uid)
		if err != nil {
			return "", nil, err
		}

		// Record that password was verified but MFA pending
		if _, err := s.db.Exec(ctx, `UPDATE auth_attempts SET succeeded = true WHERE id = $1`, attemptID); err != nil {
			return "", nil, err
		}
		if err := audit.Record(ctx, s.db, audit.Entry{ActorID: uid, Action: "auth.login.password_verified_mfa_pending",
			EntityType: "user", EntityID: uid, RequestID: in.RequestID,
			Metadata: map[string]any{"ip": in.IP, "merchant_id": merchantID}}); err != nil {
			return "", nil, err
		}

		return "", nil, &MFARequiredError{
			UserID:      uid,
			ChallengeID: challengeID,
			ExpiresAt:   s.now().Add(mfaChallengeTTL),
		}
	}

	// No MFA required (merchant user or platform admin without MFA) - create session
	return s.createSession(ctx, uid, merchantID, attemptID, in)
}

// createSession creates a session after successful authentication (password + optional MFA).
func (s *Service) createSession(ctx context.Context, uid, merchantID string, attemptID int64, in LoginInput) (string, *Principal, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var sid string
	if err := tx.QueryRow(ctx,
		`INSERT INTO sessions (user_id, merchant_id, token_hash, expires_at, ip, user_agent)
		 VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5, $6) RETURNING id::text`,
		uid, merchantID, hashToken(token), s.now().Add(s.cfg.SessionTTL), in.IP, in.UserAgent).Scan(&sid); err != nil {
		return "", nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_attempts SET succeeded = true WHERE id = $1`, attemptID); err != nil {
		return "", nil, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: uid, Action: "auth.login.success",
		EntityType: "session", EntityID: sid, RequestID: in.RequestID,
		Metadata: map[string]any{"ip": in.IP, "merchant_id": merchantID}}); err != nil {
		return "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	p, err := s.Authenticate(ctx, token)
	return token, p, err
}

// CompleteMFALogin completes the login after successful MFA verification.
// It re-verifies the password, validates the MFA code against the challenge,
// and only then opens a session.
func (s *Service) CompleteMFALogin(ctx context.Context, in LoginInput, code, challengeID string) (string, *Principal, error) {
	ek := emailKey(in.Email)

	// Reserve an attempt first so rate limiting covers the whole (password+MFA)
	// verification path, mirroring Login.
	attemptID, left, err := s.reserveAttempt(ctx, ek, in.IP)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			s.audit(ctx, "", "auth.login.rate_limited", in, ek)
		}
		return "", nil, err
	}

	// Verify password (timing-equalized like Login).
	uid, hash, status, err := s.lookupUser(ctx, in.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		VerifyPassword(in.Password, s.dummy())
		s.audit(ctx, "", "auth.login.failed", in, ek)
		return "", nil, &BadCredentialsError{AttemptsLeft: left}
	}
	if err != nil {
		return "", nil, err
	}
	if !VerifyPassword(in.Password, hash) || status != "active" {
		s.audit(ctx, uid, "auth.login.failed", in, ek)
		return "", nil, &BadCredentialsError{AttemptsLeft: left}
	}

	merchantID, err := s.pickMerchant(ctx, uid, in.MerchantID)
	if err != nil {
		s.audit(ctx, uid, "auth.login.failed", in, ek)
		return "", nil, err
	}

	// Validate the MFA code against the pending challenge. This also enforces
	// the MFA-specific rate limit and consumes the challenge on success.
	if err := s.VerifyMFA(ctx, MFAVerifyInput{
		UserID:      uid,
		Code:        code,
		ChallengeID: challengeID,
		IP:          in.IP,
		RequestID:   in.RequestID,
	}); err != nil {
		s.audit(ctx, uid, "auth.login.failed", in, ek)
		return "", nil, err
	}

	// All checks passed: mark successful and create session.
	return s.createSession(ctx, uid, merchantID, attemptID, in)
}

// reserveAttempt enforces the rate limit and records the attempt as a failure
// BEFORE the password is checked, under advisory locks, so parallel guesses
// cannot exceed the limit. A successful login later flips succeeded=true.
func (s *Service) reserveAttempt(ctx context.Context, ek, ip string) (int64, int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Fixed lock order (email, then ip) avoids deadlocks.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('login:e:'||$1, 0))`, ek); err != nil {
		return 0, 0, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('login:i:'||$1, 0))`, ip); err != nil {
		return 0, 0, err
	}
	now := s.now()
	since := now.Add(-s.cfg.AttemptWin)
	var byEmail, byIP int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE email_key = $1), count(*) FILTER (WHERE ip = $2)
		FROM auth_attempts WHERE NOT succeeded AND created_at > $3 AND (email_key = $1 OR ip = $2)`,
		ek, ip, since).Scan(&byEmail, &byIP); err != nil {
		return 0, 0, err
	}
	if byEmail >= s.cfg.MaxFailEmail || byIP >= s.cfg.MaxFailIP {
		// Lockout lifts when enough old failures leave the window: the (count-max+1)-th oldest per dimension.
		var wait time.Duration
		for _, d := range []struct {
			col      string
			val      string
			n, limit int
		}{{"email_key", ek, byEmail, s.cfg.MaxFailEmail}, {"ip", ip, byIP, s.cfg.MaxFailIP}} {
			if d.n < d.limit {
				continue
			}
			var oldest time.Time
			if err := tx.QueryRow(ctx, `SELECT created_at FROM auth_attempts WHERE NOT succeeded AND created_at > $2 AND `+d.col+` = $1
				ORDER BY created_at OFFSET $3 LIMIT 1`, d.val, since, d.n-d.limit).Scan(&oldest); err != nil {
				return 0, 0, err
			}
			if w := oldest.Add(s.cfg.AttemptWin).Sub(now); w > wait {
				wait = w
			}
		}
		return 0, 0, &RateLimitedError{RetryAfter: wait}
	}
	var id int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO auth_attempts (email_key, ip, created_at) VALUES ($1,$2,$3) RETURNING id`,
		ek, ip, now).Scan(&id); err != nil {
		return 0, 0, err
	}
	left := min(s.cfg.MaxFailEmail-byEmail-1, s.cfg.MaxFailIP-byIP-1)
	return id, left, tx.Commit(ctx)
}

func (s *Service) lookupUser(ctx context.Context, email string) (id, hash, status string, err error) {
	err = s.db.QueryRow(ctx,
		`SELECT id::text, password_hash, status FROM users WHERE lower(email) = $1`,
		normEmail(email)).Scan(&id, &hash, &status)
	return
}

// pickMerchant decides the tenant bound to the new session from DB membership.
// The client-supplied merchant ID is only a selector among the user's own memberships.
func (s *Service) pickMerchant(ctx context.Context, uid, requested string) (string, error) {
	rows, err := s.db.Query(ctx, `SELECT merchant_id::text FROM merchant_users WHERE user_id = $1`, uid)
	if err != nil {
		return "", err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	if requested != "" {
		for _, id := range ids {
			if id == requested {
				return id, nil
			}
		}
		return "", ErrInvalidCredentials // not a member: same answer as bad credentials
	}
	var platform bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1)`, uid).Scan(&platform); err != nil {
		return "", err
	}
	switch {
	case platform:
		return "", nil
	case len(ids) == 1:
		return ids[0], nil
	case len(ids) > 1:
		return "", ErrMerchantRequired
	}
	return "", ErrInvalidCredentials // no role anywhere
}

// Authenticate resolves a session token to a Principal, or ErrUnauthenticated.
// Permissions are read fresh on every call, so role changes and revocations
// take effect immediately.
func (s *Service) Authenticate(ctx context.Context, token string) (*Principal, error) {
	if token == "" {
		return nil, ErrUnauthenticated
	}
	now := s.now()
	var sid, uid, mid, email string
	var verified bool
	var lastSeen time.Time
	var perms []string
	// Satu round-trip untuk sesi + izin (sebelumnya dua query per request terautentikasi).
	// Izin tetap dibaca segar tiap panggilan, jadi pencabutan peran langsung berlaku.
	err := s.db.QueryRow(ctx, `
		SELECT s.id::text, s.user_id::text, COALESCE(s.merchant_id::text,''), s.last_seen_at, u.email, u.email_verified_at IS NOT NULL,
		       CASE WHEN s.merchant_id IS NULL THEN
		         ARRAY(SELECT rp.permission_code FROM user_roles ur JOIN role_permissions rp ON rp.role_id = ur.role_id
		               WHERE ur.user_id = s.user_id)
		       ELSE
		         ARRAY(SELECT rp.permission_code FROM merchant_users mu JOIN role_permissions rp ON rp.role_id = mu.role_id
		               WHERE mu.user_id = s.user_id AND mu.merchant_id = s.merchant_id)
		       END
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > $2
		  AND s.last_seen_at > $3 AND u.status = 'active'`,
		hashToken(token), now, now.Add(-s.cfg.IdleTTL)).Scan(&sid, &uid, &mid, &lastSeen, &email, &verified, &perms)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	if len(perms) == 0 {
		return nil, ErrUnauthenticated // role/membership removed since login
	}
	p := &Principal{UserID: uid, Email: email, SessionID: sid, MerchantID: mid, EmailVerified: verified, Permissions: map[string]struct{}{}}
	for _, c := range perms {
		p.Permissions[c] = struct{}{}
	}
	if now.Sub(lastSeen) > time.Minute { // throttle writes
		_, _ = s.db.Exec(ctx, `UPDATE sessions SET last_seen_at = $2 WHERE id = $1`, sid, now)
	}
	return p, nil
}

// Logout revokes the caller's own session.
func (s *Service) Logout(ctx context.Context, p *Principal, requestID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx,
		`UPDATE sessions SET revoked_at = now(), revoked_reason = 'logout' WHERE id = $1 AND revoked_at IS NULL`,
		p.SessionID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "auth.logout",
		EntityType: "session", EntityID: p.SessionID, RequestID: requestID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RevokeUserSessions revokes every active session of a user (e.g. password reset,
// disable). actor is the user performing the action.
func (s *Service) RevokeUserSessions(ctx context.Context, actor, userID, reason, requestID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx,
		`UPDATE sessions SET revoked_at = now(), revoked_reason = $2 WHERE user_id = $1 AND revoked_at IS NULL`,
		userID, reason)
	if err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actor, Action: "auth.sessions.revoked",
		EntityType: "user", EntityID: userID, RequestID: requestID,
		Metadata: map[string]any{"reason": reason, "count": tag.RowsAffected()}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) dummy() string {
	s.dummyOnce.Do(func() { s.dummyHash, _ = HashPassword("dummy-password-for-timing") })
	return s.dummyHash
}

// audit records a login outcome; failures to audit must not change the login result.
func (s *Service) audit(ctx context.Context, actor, action string, in LoginInput, ek string) {
	_ = audit.Record(ctx, s.db, audit.Entry{ActorID: actor, Action: action, EntityType: "user",
		EntityID: actor, RequestID: in.RequestID,
		Metadata: map[string]any{"ip": in.IP, "email_key": ek[:16]}})
}

// SessionTTL is the absolute session lifetime (used for the cookie Max-Age).
func (s *Service) SessionTTL() time.Duration { return s.cfg.SessionTTL }
