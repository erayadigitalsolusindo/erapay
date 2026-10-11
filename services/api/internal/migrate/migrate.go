// Package migrate runs the embedded goose migrations against PostgreSQL.
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/erayadigitalsolusindo/erapay/services/api/migrations"
)

// Runner applies migrations through a single database/sql handle.
type Runner struct {
	db *sql.DB
	p  *goose.Provider
}

// New opens databaseURL and prepares the migration provider. Call Close when done.
func New(databaseURL string) (*Runner, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Runner{db: db, p: p}, nil
}

func (r *Runner) Close() error { return r.db.Close() }

// Up applies all pending migrations.
func (r *Runner) Up(ctx context.Context) error {
	_, err := r.p.Up(ctx)
	return err
}

// Down rolls back exactly one migration.
func (r *Runner) Down(ctx context.Context) error {
	_, err := r.p.Down(ctx)
	return err
}

// Reset rolls back every migration (test/dev only; destructive).
func (r *Runner) Reset(ctx context.Context) error {
	_, err := r.p.DownTo(ctx, 0)
	return err
}

// Status returns one line per known migration.
func (r *Runner) Status(ctx context.Context) ([]string, error) {
	st, err := r.p.Status(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(st))
	for _, s := range st {
		out = append(out, fmt.Sprintf("%04d %-8s %s", s.Source.Version, s.State, s.Source.Path))
	}
	return out, nil
}

var _ = stdlib.GetDefaultDriver // registers the "pgx" driver
