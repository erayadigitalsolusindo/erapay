// Package webhook stores raw inbound gateway notifications. It is capture-only: it
// never interprets a payload or touches payment state (PRD §0 #2).
package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MaxBody = 64 << 10 // bytes kept per capture
	MaxRows = 5000     // hard cap so an unauthenticated endpoint cannot fill the disk
)

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// Capture records one request. body is already limited by the caller to MaxBody+1 bytes;
// the extra byte only signals truncation. It returns false when the row cap was hit.
func (s *Store) Capture(ctx context.Context, gateway string, r *http.Request, body []byte) (bool, error) {
	truncated := len(body) > MaxBody
	if truncated {
		body = body[:MaxBody]
	}
	hdr := map[string][]string{}
	for k, v := range r.Header {
		if !strings.EqualFold(k, "Authorization") {
			hdr[k] = v
		}
	}
	hj, err := json.Marshal(hdr)
	if err != nil {
		return false, err
	}
	var authLen *int
	var authSum *string
	auth := r.Header.Get("Authorization")
	if auth != "" {
		n := len(auth)
		h := sha256.Sum256([]byte(auth))
		x := hex.EncodeToString(h[:])
		authLen, authSum = &n, &x
	}
	bs := sha256.Sum256(body)
	tag, err := s.db.Exec(ctx, `
		INSERT INTO webhook_captures (gateway, method, path, query, headers, auth_present, auth_len, auth_sha256,
		                              body, body_sha256, body_truncated, remote_addr)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12
		WHERE (SELECT count(*) FROM webhook_captures) < $13`,
		gateway, r.Method, r.URL.Path, r.URL.RawQuery, hj, auth != "", authLen, authSum,
		body, hex.EncodeToString(bs[:]), truncated, r.RemoteAddr, MaxRows)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
