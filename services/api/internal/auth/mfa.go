// Package auth implements password hashing, sessions, login rate limiting, RBAC, and MFA (TOTP).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// MFA errors
var (
	ErrMFARequired          = errors.New("MFA required")
	ErrMFANotEnrolled       = errors.New("MFA not enrolled")
	ErrMFAAlreadyEnrolled   = errors.New("MFA already enrolled")
	ErrMFAInvalidCode       = errors.New("invalid MFA code")
	ErrMFARateLimited       = errors.New("too many MFA attempts")
	ErrRecoveryCodeUsed     = errors.New("recovery code already used")
	ErrNoRecoveryCodesLeft  = errors.New("no recovery codes left")
	ErrMFAChallengeExpired  = errors.New("MFA challenge expired")
	ErrMFAEnrollmentExpired = errors.New("MFA enrollment expired")
	ErrMFAEnrollmentPending = errors.New("MFA enrollment already pending")
)

// MFARateLimitedError carries how long the caller must wait.
type MFARateLimitedError struct{ RetryAfter time.Duration }

func (e *MFARateLimitedError) Error() string   { return ErrMFARateLimited.Error() }
func (e *MFARateLimitedError) Is(t error) bool { return t == ErrMFARateLimited }

// MFAEnrollment holds the data needed for TOTP enrollment.
type MFAEnrollment struct {
	Secret        string   // base32 encoded secret (for QR code)
	QRCodeURL     string   // otpauth:// URL for QR code
	RecoveryCodes []string // plaintext recovery codes (shown once)
	ExpiresAt     time.Time
}

// MFAVerifyInput is the input for MFA verification.
type MFAVerifyInput struct {
	UserID      string
	Code        string // TOTP code or recovery code
	ChallengeID string // sha256 hex of challenge nonce (for rate limiting)
	IP          string
	RequestID   string
}

// MFASessionInfo holds MFA-related session info.
type MFASessionInfo struct {
	UserID          string
	MFAEnrolled     bool
	MFAVerified     bool
	MFARegisteredAt *time.Time
}

// AdminMFARequired returns true if the principal has a platform admin role that requires MFA.
func (p *Principal) AdminMFARequired() bool {
	// Platform roles that require MFA per PRD auth.md
	return p.Has("gateway:manage") || // super_admin
		p.Has("user:manage") || // super_admin
		p.Has("audit:read") || // super_admin, ops_support
		p.Has("merchant:write") || // super_admin
		p.Has("merchant:review") || // ops_support, super_admin
		p.Has("merchant:approve") || // super_admin
		p.Has("report:read") || // finance, super_admin
		p.Has("rbac:manage") || // dewa
		p.Has("menu:manage") // dewa
}

const (
	mfaChallengeTTL     = 5 * time.Minute  // challenge valid for 5 min
	mfaEnrollmentTTL    = 10 * time.Minute // enrollment valid for 10 min
	mfaRateLimitWindow  = 15 * time.Minute
	mfaMaxFailPerWindow = 5
)

// generateTOTPSecret generates a new base32-encoded TOTP secret.
func generateTOTPSecret(accountName string) (string, []byte, error) {
	if accountName == "" {
		accountName = "erapay-mfa-setup"
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "EraPay",
		AccountName: accountName,
		SecretSize:  20, // 160 bits
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
	})
	if err != nil {
		return "", nil, err
	}
	// Decode the base32 secret back to raw bytes so the QR URL below
	// encodes the exact same secret (Generate expects raw bytes).
	raw, err := decodeTOTPSecret(key.Secret())
	if err != nil {
		return "", nil, err
	}
	return key.Secret(), raw, nil
}

// generateRecoveryCodes generates one-time recovery codes.
func generateRecoveryCodes(count int) ([]string, error) {
	if count <= 0 {
		count = 10
	}
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		b := make([]byte, 8) // 8 bytes = 16 hex chars
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		codes[i] = hex.EncodeToString(b)
	}
	return codes, nil
}

// hashRecoveryCodes argon2id-hashes recovery codes for storage.
func hashRecoveryCodes(codes []string) ([]string, error) {
	hashed := make([]string, len(codes))
	for i, code := range codes {
		h, err := HashPassword(code)
		if err != nil {
			return nil, err
		}
		hashed[i] = h
	}
	return hashed, nil
}

// verifyRecoveryCode checks if the provided code matches any unused recovery code.
// If matched, it removes that code from the stored list.
func (s *Service) verifyRecoveryCode(ctx context.Context, userID, code string) (bool, error) {
	var storedCodes []string
	err := s.db.QueryRow(ctx,
		`SELECT mfa_recovery_codes FROM users WHERE id = $1`,
		userID).Scan(&storedCodes)
	if err != nil {
		return false, err
	}
	if len(storedCodes) == 0 {
		return false, ErrNoRecoveryCodesLeft
	}

	// Check each hashed code
	for i, hashed := range storedCodes {
		if VerifyPassword(code, hashed) {
			// Remove this used code
			newCodes := append(storedCodes[:i], storedCodes[i+1:]...)
			_, err := s.db.Exec(ctx,
				`UPDATE users SET mfa_recovery_codes = $1, version = version + 1 WHERE id = $2`,
				newCodes, userID)
			if err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, ErrRecoveryCodeUsed
}

// EnrollMFA starts MFA enrollment for a user. Returns secret, QR code URL, and recovery codes.
// The secret is stored in a pending table; it's only moved to users table after successful verification.
func (s *Service) EnrollMFA(ctx context.Context, userID, email string) (*MFAEnrollment, error) {
	// Check if already enrolled
	var enrolled bool
	err := s.db.QueryRow(ctx,
		`SELECT mfa_secret IS NOT NULL FROM users WHERE id = $1`,
		userID).Scan(&enrolled)
	if err != nil {
		return nil, err
	}
	if enrolled {
		return nil, ErrMFAAlreadyEnrolled
	}

	// Pendaftaran yang tertunda ditimpa (ON CONFLICT di bawah): memuat ulang halaman tidak boleh membuat pengguna buntu.

	secret, secretRaw, err := generateTOTPSecret(email)
	if err != nil {
		return nil, err
	}

	// Build otpauth:// URL for QR code (use the same raw secret)
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "EraPay",
		AccountName: email,
		Secret:      secretRaw,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
	})
	if err != nil {
		return nil, err
	}
	qrURL := key.URL()

	recoveryCodes, err := generateRecoveryCodes(10)
	if err != nil {
		return nil, err
	}

	expiresAt := s.now().Add(mfaEnrollmentTTL)

	// Store pending enrollment
	_, err = s.db.Exec(ctx, `
		INSERT INTO mfa_pending_enrollment (user_id, secret, recovery_codes, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET secret = $2, recovery_codes = $3, expires_at = $4`,
		userID, secret, recoveryCodes, expiresAt)
	if err != nil {
		return nil, err
	}

	return &MFAEnrollment{
		Secret:        secret,
		QRCodeURL:     qrURL,
		RecoveryCodes: recoveryCodes,
		ExpiresAt:     expiresAt,
	}, nil
}

// ConfirmMFAEnrollment verifies the first TOTP code and stores the secret + recovery codes.
func (s *Service) ConfirmMFAEnrollment(ctx context.Context, userID, code, requestID string) error {
	// Get pending enrollment
	var secret string
	var recoveryCodes []string
	err := s.db.QueryRow(ctx,
		`SELECT secret, recovery_codes FROM mfa_pending_enrollment WHERE user_id = $1 AND expires_at > now()`,
		userID).Scan(&secret, &recoveryCodes)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMFAEnrollmentExpired
	}
	if err != nil {
		return err
	}

	// Verify TOTP code
	valid := totp.Validate(code, secret)
	if !valid {
		// Record failed attempt
		s.recordMFAAttempt(ctx, userID, "", false)
		return ErrMFAInvalidCode
	}

	// Hash recovery codes
	hashedCodes, err := hashRecoveryCodes(recoveryCodes)
	if err != nil {
		return err
	}

	// Move to users table
	now := s.now()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, `
		UPDATE users SET mfa_secret = $1, mfa_recovery_codes = $2, mfa_enrolled_at = $3, version = version + 1
		WHERE id = $4`,
		secret, hashedCodes, now, userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM mfa_pending_enrollment WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}

	// Record success
	s.recordMFAAttempt(ctx, userID, "", true)

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: userID, Action: "mfa.enroll.confirmed", EntityType: "user", EntityID: userID,
		RequestID: requestID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// CancelMFAEnrollment cancels a pending enrollment.
func (s *Service) CancelMFAEnrollment(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM mfa_pending_enrollment WHERE user_id = $1`, userID)
	return err
}

// GetMFAEnrollmentStatus returns the current MFA status for a user.
func (s *Service) GetMFAEnrollmentStatus(ctx context.Context, userID string) (*MFASessionInfo, error) {
	var secret *string // NULL until the user enrolls
	var enrolledAt *time.Time
	err := s.db.QueryRow(ctx,
		`SELECT mfa_secret, mfa_enrolled_at FROM users WHERE id = $1`,
		userID).Scan(&secret, &enrolledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMFANotEnrolled
	}
	if err != nil {
		return nil, err
	}

	enrolled := secret != nil && *secret != ""
	return &MFASessionInfo{
		UserID:          userID,
		MFAEnrolled:     enrolled,
		MFARegisteredAt: enrolledAt,
	}, nil
}

// DisableMFA disables MFA for a user (admin or self-service with password confirmation).
func (s *Service) DisableMFA(ctx context.Context, actorID, userID, requestID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, `
		UPDATE users SET mfa_secret = NULL, mfa_recovery_codes = NULL, mfa_enrolled_at = NULL, version = version + 1
		WHERE id = $1`,
		userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `DELETE FROM mfa_pending_enrollment WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		ActorID: actorID, Action: "mfa.disable", EntityType: "user", EntityID: userID,
		RequestID: requestID,
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GenerateMFAChallenge generates a new challenge nonce for MFA verification.
func (s *Service) GenerateMFAChallenge(ctx context.Context, userID string) (string, string, error) {
	// Check if user has MFA enrolled
	var secret string
	err := s.db.QueryRow(ctx,
		`SELECT mfa_secret FROM users WHERE id = $1 AND mfa_secret IS NOT NULL`,
		userID).Scan(&secret)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrMFANotEnrolled
	}
	if err != nil {
		return "", "", err
	}

	// Generate challenge nonce
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}
	challengeID := hex.EncodeToString(nonce)

	// Store challenge (for rate limiting). Hash the hex-encoded challengeID
	// itself so VerifyMFA / checkMFARateLimit / recordMFAAttempt (which all
	// hash []byte(challengeID)) compute the same value.
	challengeHash := sha256.Sum256([]byte(challengeID))
	challengeHashHex := hex.EncodeToString(challengeHash[:])

	_, err = s.db.Exec(ctx, `
		INSERT INTO mfa_challenges (user_id, challenge_hash, expires_at)
		VALUES ($1, $2, now() + $3)
		ON CONFLICT (user_id) DO UPDATE SET challenge_hash = $2, expires_at = now() + $3`,
		userID, challengeHashHex, mfaChallengeTTL)
	if err != nil {
		return "", "", err
	}

	return challengeID, secret, nil
}

// VerifyMFA verifies a TOTP code or recovery code for a challenge.
func (s *Service) VerifyMFA(ctx context.Context, in MFAVerifyInput) error {
	// Rate limit check
	if err := s.checkMFARateLimit(ctx, in.UserID, in.ChallengeID); err != nil {
		return err
	}

	// Get challenge
	var challengeHash string
	err := s.db.QueryRow(ctx,
		`SELECT challenge_hash FROM mfa_challenges WHERE user_id = $1 AND expires_at > now()`,
		in.UserID).Scan(&challengeHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMFAChallengeExpired
	}
	if err != nil {
		return err
	}

	// Verify challenge ID matches
	expectedHash := sha256.Sum256([]byte(in.ChallengeID))
	expectedHashHex := hex.EncodeToString(expectedHash[:])
	if subtle.ConstantTimeCompare([]byte(challengeHash), []byte(expectedHashHex)) != 1 {
		return ErrMFAInvalidCode
	}

	// Check if it's a recovery code (16 hex chars) vs TOTP (6 digits)
	isRecoveryCode := len(in.Code) == 16 && isHex(in.Code)

	var success bool
	if isRecoveryCode {
		success, err = s.verifyRecoveryCode(ctx, in.UserID, in.Code)
		if err != nil {
			if errors.Is(err, ErrRecoveryCodeUsed) || errors.Is(err, ErrNoRecoveryCodesLeft) {
				success = false
			} else {
				return err
			}
		}
	} else {
		// Verify TOTP
		var secret string
		err := s.db.QueryRow(ctx,
			`SELECT mfa_secret FROM users WHERE id = $1 AND mfa_secret IS NOT NULL`,
			in.UserID).Scan(&secret)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMFANotEnrolled
		}
		if err != nil {
			return err
		}
		success = totp.Validate(in.Code, secret)
	}

	// Record attempt
	s.recordMFAAttempt(ctx, in.UserID, in.ChallengeID, success)

	if !success {
		return ErrMFAInvalidCode
	}

	// Mark challenge as used
	_, err = s.db.Exec(ctx, `DELETE FROM mfa_challenges WHERE user_id = $1`, in.UserID)
	if err != nil {
		return err
	}

	// Audit
	_ = audit.Record(ctx, s.db, audit.Entry{
		ActorID: in.UserID, Action: "mfa.verify.success", EntityType: "user", EntityID: in.UserID,
		RequestID: in.RequestID,
		Metadata:  map[string]any{"recovery": isRecoveryCode},
	})

	return nil
}

// checkMFARateLimit checks if the user has exceeded MFA attempt limits.
func (s *Service) checkMFARateLimit(ctx context.Context, userID, challengeID string) error {
	challengeHash := sha256.Sum256([]byte(challengeID))
	challengeHashHex := hex.EncodeToString(challengeHash[:])

	since := s.now().Add(-mfaRateLimitWindow)
	var failCount int
	err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM mfa_attempts
		WHERE user_id = $1 AND challenge_id = $2 AND NOT succeeded AND created_at > $3`,
		userID, challengeHashHex, since).Scan(&failCount)
	if err != nil {
		return err
	}
	if failCount >= mfaMaxFailPerWindow {
		// Find oldest failure to calculate retry-after
		var oldest time.Time
		err := s.db.QueryRow(ctx, `
			SELECT created_at FROM mfa_attempts
			WHERE user_id = $1 AND challenge_id = $2 AND NOT succeeded AND created_at > $3
			ORDER BY created_at LIMIT 1`,
			userID, challengeHashHex, since).Scan(&oldest)
		if err != nil {
			return err
		}
		wait := oldest.Add(mfaRateLimitWindow).Sub(s.now())
		if wait < 0 {
			wait = 0
		}
		return &MFARateLimitedError{RetryAfter: wait}
	}
	return nil
}

// recordMFAAttempt records an MFA attempt for rate limiting.
func (s *Service) recordMFAAttempt(ctx context.Context, userID, challengeID string, success bool) {
	challengeHash := sha256.Sum256([]byte(challengeID))
	challengeHashHex := hex.EncodeToString(challengeHash[:])

	_, _ = s.db.Exec(ctx, `
		INSERT INTO mfa_attempts (user_id, challenge_id, succeeded, created_at)
		VALUES ($1, $2, $3, now())`,
		userID, challengeHashHex, success)
}

// isHex checks if a string is valid hex.
func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}

// RegenerateRecoveryCodes generates new recovery codes for a user (requires valid TOTP or recovery code).
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID, requestID string) ([]string, error) {
	// Verify user has MFA enrolled
	var secret string
	err := s.db.QueryRow(ctx,
		`SELECT mfa_secret FROM users WHERE id = $1 AND mfa_secret IS NOT NULL`,
		userID).Scan(&secret)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMFANotEnrolled
	}
	if err != nil {
		return nil, err
	}

	newCodes, err := generateRecoveryCodes(10)
	if err != nil {
		return nil, err
	}
	hashedCodes, err := hashRecoveryCodes(newCodes)
	if err != nil {
		return nil, err
	}

	_, err = s.db.Exec(ctx, `
		UPDATE users SET mfa_recovery_codes = $1, version = version + 1 WHERE id = $2`,
		hashedCodes, userID)
	if err != nil {
		return nil, err
	}

	_ = audit.Record(ctx, s.db, audit.Entry{
		ActorID: userID, Action: "mfa.recovery_codes.regenerated", EntityType: "user", EntityID: userID,
		RequestID: requestID,
	})

	return newCodes, nil
}

// decodeTOTPSecret decodes a base32 (no padding) TOTP secret into raw bytes.
func decodeTOTPSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	// pquerna/otp uses base32 StdEncoding without padding for TOTP secrets.
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	if pad := len(s) % 8; pad != 0 {
		s += strings.Repeat("=", 8-pad)
		enc = base32.StdEncoding
	}
	return enc.DecodeString(s)
}

// MFA migration requires a new table for challenges
// Add this to the migration file:
/*
CREATE TABLE IF NOT EXISTS mfa_challenges (
    user_id      uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    challenge_hash text NOT NULL,
    expires_at   timestamptz NOT NULL
);
*/
