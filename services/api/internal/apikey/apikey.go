// Package apikey issues and verifies merchant API keys for POS/integrators.
//
// Key format: epk_<sbx|prd>_<prefix:8 hex>_<secret:64 hex>. The prefix is public
// and used for lookup; the secret (256-bit random) is shown once and only its
// SHA-256 is stored.
package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

var (
	ErrInvalidKey = errors.New("invalid api key")
	ErrBadScope   = errors.New("invalid scope")
	ErrNotFound   = errors.New("api key not found")
)

// AllowedScopes are the only permissions a key may carry (never admin/user/key management).
var AllowedScopes = []string{"payment:read", "payment:create", "report:read"}

const (
	MaxRotateGrace = 24 * time.Hour
	touchEvery     = time.Minute
)

// Identity is what a verified key proves. MerchantID is the tenant (PRD §0 #7).
type Identity struct {
	KeyID       string
	MerchantID  string
	Environment string
	Prefix      string
	Scopes      []string
}

type Key struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Environment string     `json:"environment"`
	Prefix      string     `json:"prefix"`
	Scopes      []string   `json:"scopes"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

// Issued is returned once; Secret (the full key) cannot be recovered later.
type Issued struct {
	Key
	Secret string `json:"secret"`
}

type Service struct {
	db  *pgxpool.Pool
	env string // "sandbox" | "production": the only environment this deployment issues/accepts
	now func() time.Time
}

func NewService(db *pgxpool.Pool, environment string) *Service {
	return &Service{db: db, env: environment, now: time.Now}
}

// EnvFromApp maps ERAPAY_ENV to a key environment; production is explicit, all else sandbox.
func EnvFromApp(appEnv string) string {
	if appEnv == "production" {
		return "production"
	}
	return "sandbox"
}

func envTag(env string) string {
	if env == "production" {
		return "prd"
	}
	return "sbx"
}

func hashSecret(secret string) []byte { h := sha256.Sum256([]byte(secret)); return h[:] }

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ValidateScopes requires scopes to be non-empty, allowed, and held by the creator.
func ValidateScopes(scopes []string, creatorHas func(string) bool) error {
	if len(scopes) == 0 {
		return ErrBadScope
	}
	for _, s := range scopes {
		if !slices.Contains(AllowedScopes, s) || !creatorHas(s) {
			return ErrBadScope
		}
	}
	return nil
}

type CreateInput struct {
	MerchantID string
	CreatedBy  string
	Name       string
	Scopes     []string
	ExpiresAt  *time.Time
	RequestID  string
}

// Create issues a new key. Scope policy must be checked by the caller via ValidateScopes.
func (s *Service) Create(ctx context.Context, in CreateInput) (Issued, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Issued{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	out, err := s.insert(ctx, tx, in, "")
	if err != nil {
		return Issued{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.CreatedBy, Action: "api_key.created",
		EntityType: "api_key", EntityID: out.ID, RequestID: in.RequestID,
		Metadata: map[string]any{"merchant_id": in.MerchantID, "prefix": out.Prefix, "scopes": out.Scopes}}); err != nil {
		return Issued{}, err
	}
	return out, tx.Commit(ctx)
}

func (s *Service) insert(ctx context.Context, tx pgx.Tx, in CreateInput, rotatedFrom string) (Issued, error) {
	for attempt := 0; attempt < 5; attempt++ {
		prefix, err := randHex(4)
		if err != nil {
			return Issued{}, err
		}
		secret, err := randHex(32)
		if err != nil {
			return Issued{}, err
		}
		full := "epk_" + envTag(s.env) + "_" + prefix + "_" + secret
		// Savepoint so a prefix collision doesn't abort the surrounding transaction.
		sp, err := tx.Begin(ctx)
		if err != nil {
			return Issued{}, err
		}
		var k Issued
		err = sp.QueryRow(ctx, `
			INSERT INTO api_keys (merchant_id, name, environment, prefix, secret_hash, scopes, created_by, expires_at, rotated_from)
			VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8,NULLIF($9,'')::uuid)
			RETURNING id::text, name, environment, prefix, scopes, created_at, expires_at`,
			in.MerchantID, in.Name, s.env, prefix, hashSecret(secret), in.Scopes, in.CreatedBy, in.ExpiresAt, rotatedFrom,
		).Scan(&k.ID, &k.Name, &k.Environment, &k.Prefix, &k.Scopes, &k.CreatedAt, &k.ExpiresAt)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "prefix") {
			_ = sp.Rollback(ctx)
			continue
		}
		if err != nil {
			_ = sp.Rollback(ctx)
			return Issued{}, err
		}
		if err := sp.Commit(ctx); err != nil {
			return Issued{}, err
		}
		k.Secret = full
		return k, nil
	}
	return Issued{}, errors.New("could not allocate api key prefix")
}

// Verify authenticates a presented key. All failure modes return ErrInvalidKey.
func (s *Service) Verify(ctx context.Context, presented string) (*Identity, error) {
	parts := strings.Split(presented, "_")
	if len(parts) != 4 || parts[0] != "epk" || len(parts[2]) != 8 || len(parts[3]) != 64 {
		return nil, ErrInvalidKey
	}
	if parts[1] != envTag(s.env) {
		return nil, ErrInvalidKey // sandbox and production keys never cross environments
	}
	var (
		id, mid, env string
		hash         []byte
		scopes       []string
		last         *time.Time
	)
	err := s.db.QueryRow(ctx, `
		SELECT id::text, merchant_id::text, environment, secret_hash, scopes, last_used_at
		FROM api_keys
		WHERE prefix = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > $2)`,
		parts[2], s.now()).Scan(&id, &mid, &env, &hash, &scopes, &last)
	if errors.Is(err, pgx.ErrNoRows) {
		subtle.ConstantTimeCompare(hashSecret(parts[3]), hashSecret("x")) // keep timing similar
		return nil, ErrInvalidKey
	}
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare(hashSecret(parts[3]), hash) != 1 || env != s.env {
		return nil, ErrInvalidKey
	}
	if last == nil || s.now().Sub(*last) > touchEvery {
		_, _ = s.db.Exec(ctx, `UPDATE api_keys SET last_used_at = $2 WHERE id = $1`, id, s.now())
	}
	return &Identity{KeyID: id, MerchantID: mid, Environment: env, Prefix: parts[2], Scopes: scopes}, nil
}

// List returns the merchant's keys (never secrets), newest first.
func (s *Service) List(ctx context.Context, merchantID string) ([]Key, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, environment, prefix, scopes, created_at, expires_at, revoked_at, last_used_at
		FROM api_keys WHERE merchant_id = $1 ORDER BY created_at DESC, id`, merchantID)
	if err != nil {
		return nil, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Key])
	if keys == nil {
		keys = []Key{}
	}
	return keys, err
}

// Revoke disables a key of the given merchant. Idempotent; unknown/foreign id -> ErrNotFound.
func (s *Service) Revoke(ctx context.Context, merchantID, keyID, actor, reason, requestID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := s.revokeTx(ctx, tx, merchantID, keyID, reason); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actor, Action: "api_key.revoked", EntityType: "api_key",
		EntityID: keyID, RequestID: requestID, Metadata: map[string]any{"merchant_id": merchantID, "reason": reason}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) revokeTx(ctx context.Context, tx pgx.Tx, merchantID, keyID, reason string) error {
	var one int
	err := tx.QueryRow(ctx, `
		UPDATE api_keys SET revoked_at = COALESCE(revoked_at, $3), revoked_reason = COALESCE(revoked_reason, $4)
		WHERE id = $1 AND merchant_id = $2 RETURNING 1`,
		keyID, merchantID, s.now(), reason).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Rotate atomically issues a replacement (same name/scopes/environment) and retires
// the old key: immediately when grace == 0, otherwise it expires after grace (<= 24h)
// so deployments can switch over.
func (s *Service) Rotate(ctx context.Context, merchantID, keyID, actor string, grace time.Duration, requestID string) (Issued, error) {
	if grace < 0 || grace > MaxRotateGrace {
		return Issued{}, errors.New("grace out of range")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Issued{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var name string
	var scopes []string
	var oldExp *time.Time
	err = tx.QueryRow(ctx, `
		SELECT name, scopes, expires_at FROM api_keys
		WHERE id = $1 AND merchant_id = $2 AND revoked_at IS NULL FOR UPDATE`,
		keyID, merchantID).Scan(&name, &scopes, &oldExp)
	if errors.Is(err, pgx.ErrNoRows) {
		return Issued{}, ErrNotFound
	}
	if err != nil {
		return Issued{}, err
	}
	out, err := s.insert(ctx, tx, CreateInput{MerchantID: merchantID, CreatedBy: actor, Name: name, Scopes: scopes}, keyID)
	if err != nil {
		return Issued{}, err
	}
	if grace == 0 {
		err = s.revokeTx(ctx, tx, merchantID, keyID, "rotated")
	} else {
		exp := s.now().Add(grace)
		if oldExp != nil && oldExp.Before(exp) {
			exp = *oldExp
		}
		_, err = tx.Exec(ctx, `UPDATE api_keys SET expires_at = $2 WHERE id = $1`, keyID, exp)
	}
	if err != nil {
		return Issued{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actor, Action: "api_key.rotated", EntityType: "api_key",
		EntityID: keyID, RequestID: requestID,
		Metadata: map[string]any{"merchant_id": merchantID, "new_key_id": out.ID, "grace_seconds": int(grace.Seconds())}}); err != nil {
		return Issued{}, err
	}
	return out, tx.Commit(ctx)
}
