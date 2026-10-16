package auth

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// Profile adalah data akun yang boleh dilihat/diubah pemiliknya sendiri.
type Profile struct {
	UserID        string     `json:"user_id"`
	Email         string     `json:"email"`
	FullName      string     `json:"full_name"`
	Phone         string     `json:"phone"`
	EmailVerified bool       `json:"email_verified"`
	MFAEnabled    bool       `json:"mfa_enabled"`
	MFAEnrolledAt *time.Time `json:"mfa_enrolled_at"`
	RecoveryLeft  int        `json:"recovery_codes_left"`
	CreatedAt     time.Time  `json:"created_at"`
}

var (
	ErrInvalidProfile  = errors.New("invalid profile")
	ErrWrongPassword   = errors.New("wrong password")
	ErrMFAStepUpFailed = errors.New("password or code invalid")
	phoneRe            = regexp.MustCompile(`^\+?[0-9][0-9 ()-]{3,30}[0-9]$`)
)

func (s *Service) GetProfile(ctx context.Context, userID string) (Profile, error) {
	var p Profile
	err := s.db.QueryRow(ctx, `
		SELECT id::text, email, COALESCE(full_name,''), COALESCE(phone,''), email_verified_at IS NOT NULL,
		       mfa_secret IS NOT NULL, mfa_enrolled_at, COALESCE(cardinality(mfa_recovery_codes),0), created_at
		FROM users WHERE id = $1`, userID).
		Scan(&p.UserID, &p.Email, &p.FullName, &p.Phone, &p.EmailVerified, &p.MFAEnabled, &p.MFAEnrolledAt, &p.RecoveryLeft, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrUserNotFound
	}
	return p, err
}

// UpdateProfile menyimpan nama tampilan dan telepon (kosong = hapus). Email tidak bisa diubah di sini.
func (s *Service) UpdateProfile(ctx context.Context, userID, fullName, phone, requestID string) error {
	fullName, phone = strings.TrimSpace(fullName), strings.TrimSpace(phone)
	if n := len([]rune(fullName)); n > 100 {
		return ErrInvalidProfile
	}
	if phone != "" && !phoneRe.MatchString(phone) {
		return ErrInvalidProfile
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE users SET full_name = NULLIF($2,''), phone = NULLIF($3,''), updated_at = now(), version = version + 1
		WHERE id = $1`, userID, fullName, phone); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "user.profile.updated", EntityType: "user",
		EntityID: userID, RequestID: requestID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ChangePassword mengganti kata sandi setelah memverifikasi yang lama, lalu mencabut semua sesi lain
// (sesi saat ini tetap hidup). Dibatasi laju lewat throttle percobaan login yang sama.
func (s *Service) ChangePassword(ctx context.Context, userID, currentSessionID, current, next, ip, requestID string) error {
	if err := ValidateNewPassword(next); err != nil {
		return ErrWeakPassword
	}
	var email, hash string
	if err := s.db.QueryRow(ctx, `SELECT email, password_hash FROM users WHERE id = $1`, userID).Scan(&email, &hash); err != nil {
		return err
	}
	ek := emailKey(email)
	attemptID, _, err := s.reserveAttempt(ctx, ek, ip)
	if err != nil {
		return err
	}
	if !VerifyPassword(current, hash) {
		return ErrWrongPassword
	}
	newHash, err := HashPassword(next)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE auth_attempts SET succeeded = true WHERE id = $1`, attemptID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = now(), version = version + 1 WHERE id = $1`, userID, newHash); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now(), revoked_reason = 'password_changed'
		WHERE user_id = $1 AND revoked_at IS NULL AND id::text <> $2`, userID, currentSessionID)
	if err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "user.password.changed", EntityType: "user",
		EntityID: userID, RequestID: requestID, Metadata: map[string]any{"sessions_revoked": tag.RowsAffected()}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// StepUpMFA memastikan pemilik akun: kata sandi benar DAN kode TOTP/pemulihan valid. Dipakai sebelum
// menonaktifkan MFA atau membuat ulang kode pemulihan, supaya sesi yang dicuri tidak bisa mencopot MFA.
// Percobaan gagal dihitung pada batas laju MFA yang sama dengan login.
func (s *Service) StepUpMFA(ctx context.Context, userID, password, code string) error {
	const bucket = "self-service"
	if err := s.checkMFARateLimit(ctx, userID, bucket); err != nil {
		return err
	}
	var hash string
	var secret *string
	if err := s.db.QueryRow(ctx, `SELECT password_hash, mfa_secret FROM users WHERE id = $1`, userID).Scan(&hash, &secret); err != nil {
		return err
	}
	if secret == nil {
		return ErrMFANotEnrolled
	}
	code = strings.TrimSpace(code)
	ok := VerifyPassword(password, hash)
	if ok {
		if len(code) == 16 && isHex(code) {
			var err error
			ok, err = s.verifyRecoveryCode(ctx, userID, code)
			if err != nil && !errors.Is(err, ErrRecoveryCodeUsed) && !errors.Is(err, ErrNoRecoveryCodesLeft) {
				return err
			}
		} else {
			ok = totp.Validate(code, *secret)
		}
	}
	s.recordMFAAttempt(ctx, userID, bucket, ok)
	if !ok {
		return ErrMFAStepUpFailed
	}
	return nil
}
