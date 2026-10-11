// Package audit writes to the append-only audit_logs table.
package audit

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is satisfied by *pgxpool.Pool and pgx.Tx.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type Entry struct {
	ActorID    string // user id; empty when unknown
	Action     string
	EntityType string
	EntityID   string
	Metadata   map[string]any // never put secrets, passwords or raw tokens here
	RequestID  string
}

func Record(ctx context.Context, db Execer, e Entry) error {
	md := e.Metadata
	if md == nil {
		md = map[string]any{}
	}
	b, err := json.Marshal(md)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx,
		`INSERT INTO audit_logs (actor_id, action, entity_type, entity_id, metadata, request_id)
		 VALUES (NULLIF($1,'')::uuid, $2, $3, NULLIF($4,''), $5, NULLIF($6,''))`,
		e.ActorID, e.Action, e.EntityType, e.EntityID, b, e.RequestID)
	return err
}
