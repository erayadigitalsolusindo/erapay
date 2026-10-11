// Package idempotency makes retried operations safe using the PostgreSQL
// UNIQUE (merchant_id, operation, key) constraint (PRD §0 #4, api.md).
//
// Same key + same request  -> the first result is replayed.
// Same key + different request -> ErrConflict (IDEMPOTENCY_CONFLICT).
// Same key while the first is still running -> ErrInProgress.
package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConflict   = errors.New("idempotency key reused with a different request")
	ErrInProgress = errors.New("a request with this idempotency key is still in progress")
	ErrBadKey     = errors.New("invalid idempotency key")
)

const DefaultTTL = 24 * time.Hour

var keyRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

// ValidKey reports whether k is an acceptable Idempotency-Key value.
func ValidKey(k string) bool { return keyRe.MatchString(k) }

// Scope identifies one idempotent operation. MerchantID must come from the Principal.
type Scope struct {
	MerchantID string
	Operation  string
	Key        string
}

// Response is the stored outcome of the first execution.
type Response struct {
	Status       int
	Body         json.RawMessage
	ResourceType string
	ResourceID   string
}

// HashJSON returns a canonical hash of a JSON request body (key order and
// whitespace do not matter; numbers are kept exact). Include method/path in
// extra when the same key could be used on different endpoints.
func HashJSON(body []byte, extra ...string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	canon, err := json.Marshal(v) // map keys are emitted sorted
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	for _, e := range extra {
		h.Write([]byte(e))
		h.Write([]byte{0})
	}
	h.Write(canon)
	return h.Sum(nil), nil
}

type Store struct {
	db  *pgxpool.Pool
	ttl time.Duration
	now func() time.Time
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db, ttl: DefaultTTL, now: time.Now} }

type claim int

const (
	claimed claim = iota
	replay
)

// claimTx tries to insert the record. Under concurrency the losing inserter
// blocks on the unique index until the winner commits/rolls back.
func (s *Store) claimTx(ctx context.Context, tx pgx.Tx, sc Scope, hash []byte) (claim, *Response, error) {
	if !ValidKey(sc.Key) {
		return 0, nil, ErrBadKey
	}
	now := s.now()
	// Expired COMPLETED records may be reused; in-progress ones never are (recovery decides).
	if _, err := tx.Exec(ctx,
		`DELETE FROM idempotency_records
		 WHERE merchant_id=$1 AND operation=$2 AND key=$3 AND status='completed' AND expires_at < $4`,
		sc.MerchantID, sc.Operation, sc.Key, now); err != nil {
		return 0, nil, err
	}
	tag, err := tx.Exec(ctx,
		`INSERT INTO idempotency_records (merchant_id, operation, key, request_hash, expires_at)
		 VALUES ($1,$2,$3,$4,$5) ON CONFLICT (merchant_id, operation, key) DO NOTHING`,
		sc.MerchantID, sc.Operation, sc.Key, hash, now.Add(s.ttl))
	if err != nil {
		return 0, nil, err
	}
	if tag.RowsAffected() == 1 {
		return claimed, nil, nil
	}
	var (
		prev   []byte
		status string
		r      Response
		code   *int
		rt, id *string
		raw    *string
	)
	err = tx.QueryRow(ctx,
		`SELECT request_hash, status, response_status, response_body, resource_type, resource_id
		 FROM idempotency_records WHERE merchant_id=$1 AND operation=$2 AND key=$3`,
		sc.MerchantID, sc.Operation, sc.Key).Scan(&prev, &status, &code, &raw, &rt, &id)
	if err != nil {
		return 0, nil, err
	}
	if !bytes.Equal(prev, hash) {
		return 0, nil, ErrConflict
	}
	if status != "completed" {
		return 0, nil, ErrInProgress
	}
	if raw != nil {
		r.Body = json.RawMessage(*raw)
	}
	if code != nil {
		r.Status = *code
	}
	if rt != nil {
		r.ResourceType = *rt
	}
	if id != nil {
		r.ResourceID = *id
	}
	return replay, &r, nil
}

func complete(ctx context.Context, tx pgx.Tx, sc Scope, r Response, now time.Time) error {
	body := r.Body
	if body == nil {
		body = json.RawMessage("null")
	}
	_, err := tx.Exec(ctx,
		`UPDATE idempotency_records
		 SET status='completed', response_status=$4, response_body=$5, resource_type=NULLIF($6,''),
		     resource_id=NULLIF($7,''), completed_at=$8
		 WHERE merchant_id=$1 AND operation=$2 AND key=$3`,
		sc.MerchantID, sc.Operation, sc.Key, r.Status, string(body), r.ResourceType, r.ResourceID, now)
	return err
}

// Do runs fn at most once per (scope, key) in ONE transaction: claim, domain
// writes (via tx), and the stored response commit atomically. Concurrent
// duplicates wait on the unique index and then get the stored response
// (replayed=true). If fn fails, everything rolls back and the key stays free.
func (s *Store) Do(ctx context.Context, sc Scope, hash []byte,
	fn func(ctx context.Context, tx pgx.Tx) (Response, error)) (resp Response, replayed bool, err error) {

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Response{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	c, prev, err := s.claimTx(ctx, tx, sc, hash)
	if err != nil {
		return Response{}, false, err
	}
	if c == replay {
		return *prev, true, nil
	}
	resp, err = fn(ctx, tx)
	if err != nil {
		return Response{}, false, err
	}
	if err := complete(ctx, tx, sc, resp, s.now()); err != nil {
		return Response{}, false, err
	}
	return resp, false, tx.Commit(ctx)
}

// Reserve claims the key in its own committed transaction, for operations that
// call an external system (e.g. a gateway) and cannot run inside one DB
// transaction. Exactly one caller gets (nil, nil) and must later call Complete;
// others get the stored response, ErrInProgress or ErrConflict. A crashed
// holder leaves the record in_progress on purpose: recovery/inquiry must
// resolve it before any retry (PRD §0 #3).
func (s *Store) Reserve(ctx context.Context, sc Scope, hash []byte) (*Response, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	c, prev, err := s.claimTx(ctx, tx, sc, hash)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if c == replay {
		return prev, nil
	}
	return nil, nil
}

// Complete stores the outcome of a Reserve'd operation.
func (s *Store) Complete(ctx context.Context, sc Scope, r Response) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := complete(ctx, tx, sc, r, s.now()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
