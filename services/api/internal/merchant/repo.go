// Package merchant holds tenant-scoped merchant queries. Every method takes the
// merchant ID from the authenticated Principal and filters on it (PRD §0 #7).
package merchant

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repo struct{ db *pgxpool.Pool }

func NewRepo(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

type Profile struct {
	ID               string `json:"id"`
	LegalName        string `json:"legal_name"`
	DisplayName      string `json:"display_name"`
	OnboardingStatus string `json:"onboarding_status"`
}

type Outlet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (r *Repo) Profile(ctx context.Context, merchantID string) (Profile, error) {
	var p Profile
	err := r.db.QueryRow(ctx,
		`SELECT id::text, legal_name, display_name, onboarding_status FROM merchants WHERE id = $1`,
		merchantID).Scan(&p.ID, &p.LegalName, &p.DisplayName, &p.OnboardingStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (r *Repo) Outlets(ctx context.Context, merchantID string) ([]Outlet, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id::text, name FROM merchant_outlets WHERE merchant_id = $1 ORDER BY created_at, id`, merchantID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Outlet])
	if out == nil {
		out = []Outlet{}
	}
	return out, err
}
