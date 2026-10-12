// Package account proves email ownership and handles password reset with one-time email tokens.
//
// Tokens are 256-bit random values; only their sha256 is stored. They are single use (consumed by an
// atomic UPDATE, so parallel requests cannot both succeed) and expire. Endpoints that take an email
// answer identically whether or not the address exists, and throttle per IP and per address.
package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/mailer"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/throttle"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
	ErrRateLimited  = errors.New("too many requests")
	ErrValidation   = errors.New("validation")
)

type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }
func (e *ValidationError) Unwrap() error { return ErrValidation }

const (
	verifyTTL = 48 * time.Hour
	resetTTL  = time.Hour

	purposeVerify = "verify_email"
	purposeReset  = "reset_password"
)

// Limits for the email-sending endpoints. The per-email limit exists so nobody can mail-bomb a victim.
var emailLimits = throttle.Limits{PerIP: 10, PerEmail: 3, Window: time.Hour}

type Service struct {
	db     *pgxpool.Pool
	mail   mailer.Mailer
	webURL string
	log    *slog.Logger
	wg     sync.WaitGroup
}

// send delivers in the background. Doing it inline would make "this address has an account" visible as a
// slower response (an SMTP round trip), defeating the identical answers the endpoints give.
// Failures are logged, never reported to the caller.
func (s *Service) send(m mailer.Message) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := s.mail.Send(ctx, m); err != nil {
			s.log.Error("email not sent", "subject", m.Subject, "recipient", throttle.EmailKey(m.To)[:12], "err", err)
		}
	}()
}

// Wait blocks until queued emails are delivered or have failed (graceful shutdown, tests).
func (s *Service) Wait() { s.wg.Wait() }

func NewService(db *pgxpool.Pool, ml mailer.Mailer, webURL string, log *slog.Logger) *Service {
	return &Service{db: db, mail: ml, webURL: strings.TrimRight(webURL, "/"), log: log}
}

func hashToken(t string) []byte { h := sha256.Sum256([]byte(t)); return h[:] }

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// newToken invalidates the user's earlier unused tokens of this purpose and stores a fresh one.
func newToken(ctx context.Context, tx pgx.Tx, uid, purpose string, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := tx.Exec(ctx, `UPDATE email_tokens SET used_at = now() WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL`, uid, purpose); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO email_tokens (user_id, purpose, token_hash, expires_at) VALUES ($1,$2,$3, now() + make_interval(secs => $4))`,
		uid, purpose, hashToken(tok), ttl.Seconds()); err != nil {
		return "", err
	}
	return tok, nil
}

// consume marks a live token used and returns its user. One atomic statement: of two parallel
// requests with the same token exactly one gets a row.
func consume(ctx context.Context, tx pgx.Tx, tok, purpose string) (string, error) {
	var uid string
	err := tx.QueryRow(ctx, `UPDATE email_tokens SET used_at = now()
		WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now() RETURNING user_id::text`,
		hashToken(tok), purpose).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidToken
	}
	return uid, err
}

func (s *Service) issue(ctx context.Context, uid, purpose string, ttl time.Duration) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tok, err := newToken(ctx, tx, uid, purpose, ttl)
	if err != nil {
		return "", err
	}
	return tok, tx.Commit(ctx)
}

// SendVerification emails a fresh verification link (used right after sign-up and by Resend).
func (s *Service) SendVerification(ctx context.Context, uid, email string) error {
	tok, err := s.issue(ctx, uid, purposeVerify, verifyTTL)
	if err != nil {
		return err
	}
	s.send(mailer.Message{To: email, Subject: "Konfirmasi email Anda - EraPay", Body: fmt.Sprintf(
		"Halo,\n\nTerima kasih telah mendaftar di EraPay. Konfirmasi email Anda dengan membuka tautan berikut (berlaku 48 jam):\n\n%s/verify-email?token=%s\n\nJika Anda tidak mendaftar, abaikan email ini.\n", s.webURL, tok)})
	return nil
}

// SendAlreadyRegistered is what an existing address receives when someone tries to sign up with it.
// Sign-up answers the same either way, so this email is the only place the owner learns about it.
func (s *Service) SendAlreadyRegistered(ctx context.Context, email string) error {
	limited, err := throttle.Reserve(ctx, s.db, "dup_notice", "-", throttle.EmailKey(email), throttle.Limits{PerEmail: 3, Window: time.Hour})
	if err != nil || limited {
		return err
	}
	s.send(mailer.Message{To: email, Subject: "Seseorang mencoba mendaftar dengan email Anda - EraPay", Body: fmt.Sprintf(
		"Halo,\n\nAda yang mencoba mendaftar di EraPay dengan alamat email ini, padahal akunnya sudah ada. Tidak ada akun baru yang dibuat.\n\nJika itu Anda, silakan masuk di %s/login. Lupa kata sandi? Atur ulang di %s/forgot-password.\n\nJika bukan Anda, abaikan email ini.\n", s.webURL, s.webURL)})
	return nil
}

// Verify consumes a verification token and marks the address verified.
func (s *Service) Verify(ctx context.Context, tok, requestID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	uid, err := consume(ctx, tx, tok, purposeVerify)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()), updated_at = now() WHERE id = $1`, uid); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: uid, Action: "auth.email_verified", EntityType: "user", EntityID: uid, RequestID: requestID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// request is the shared shape of Resend and Forgot: throttle, look the user up, send, and never reveal
// whether the address exists. Only an IP over its limit is reported (that says nothing about the address).
func (s *Service) request(ctx context.Context, kind, ip, email, lookup string, send func(uid, email string) error) error {
	limited, err := throttle.Reserve(ctx, s.db, kind, ip, throttle.EmailKey(email), emailLimits)
	if errors.Is(err, throttle.ErrIPLimited) {
		return ErrRateLimited
	}
	if err != nil {
		return err
	}
	if limited {
		return nil
	}
	var uid string
	if err := s.db.QueryRow(ctx, lookup, normEmail(email)).Scan(&uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := send(uid, normEmail(email)); err != nil {
		s.log.Error("account email not sent", "kind", kind, "err", err) // never answer differently: that would reveal the account
	}
	return nil
}

// ResendVerification re-sends the link to an existing, unverified address.
func (s *Service) ResendVerification(ctx context.Context, ip, email string) error {
	return s.request(ctx, "resend", ip, email,
		`SELECT id::text FROM users WHERE lower(email) = $1 AND status = 'active' AND email_verified_at IS NULL`,
		func(uid, em string) error { return s.SendVerification(ctx, uid, em) })
}

// ForgotPassword emails a one-hour reset link to an existing active account.
func (s *Service) ForgotPassword(ctx context.Context, ip, email string) error {
	return s.request(ctx, "forgot", ip, email,
		`SELECT id::text FROM users WHERE lower(email) = $1 AND status = 'active'`,
		func(uid, em string) error {
			tok, err := s.issue(ctx, uid, purposeReset, resetTTL)
			if err != nil {
				return err
			}
			s.send(mailer.Message{To: em, Subject: "Atur ulang kata sandi - EraPay", Body: fmt.Sprintf(
				"Halo,\n\nKami menerima permintaan mengatur ulang kata sandi akun EraPay Anda. Buka tautan berikut (berlaku 1 jam, sekali pakai):\n\n%s/reset-password?token=%s\n\nJika bukan Anda yang meminta, abaikan email ini; kata sandi Anda tidak berubah.\n", s.webURL, tok)})
			return nil
		})
}

// ResetPassword sets a new password with a reset token. It ends every session of the user and also
// counts as proof of email ownership. The token is consumed only if everything else succeeds, so a
// rejected (e.g. too weak) password does not burn it.
func (s *Service) ResetPassword(ctx context.Context, tok, password, requestID string) error {
	if err := auth.ValidateNewPassword(password); err != nil {
		return &ValidationError{Msg: err.Error()}
	}
	// Cheap check before the expensive hash, so junk tokens cannot be used to burn CPU.
	var live bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM email_tokens WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now())`,
		hashToken(tok), purposeReset).Scan(&live); err != nil {
		return err
	}
	if !live {
		return ErrInvalidToken
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	uid, err := consume(ctx, tx, tok, purposeReset)
	if err != nil {
		return err
	}
	var email string
	if err := tx.QueryRow(ctx, `UPDATE users SET password_hash = $2, email_verified_at = COALESCE(email_verified_at, now()),
		updated_at = now(), version = version + 1 WHERE id = $1 RETURNING email`, uid, hash).Scan(&email); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now(), revoked_reason = 'password_reset' WHERE user_id = $1 AND revoked_at IS NULL`, uid)
	if err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: uid, Action: "auth.password_reset", EntityType: "user", EntityID: uid, RequestID: requestID,
		Metadata: map[string]any{"sessions_revoked": tag.RowsAffected()}}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.send(mailer.Message{To: email, Subject: "Kata sandi EraPay Anda telah diubah", Body: fmt.Sprintf(
		"Halo,\n\nKata sandi akun EraPay Anda baru saja diubah dan semua sesi login ditutup. Jika bukan Anda, segera atur ulang di %s/forgot-password dan hubungi tim EraPay.\n", s.webURL)})
	return nil
}
