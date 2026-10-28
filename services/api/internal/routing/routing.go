// Package routing holds the platform's choice of which gateway takes NEW payments (DEWA sets it in
// the panel). It only stores and validates the choice; payment.Orchestrator applies it. Without a
// stored choice the old behaviour stays: a merchant's oldest active gateway account.
package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var (
	// ErrConflict: someone changed the choice since the form was loaded.
	ErrConflict = errors.New("routing: changed since you opened the form")
	// ErrWrongEnv: the server routes one environment; changing the other from here would be a trap.
	ErrWrongEnv = errors.New("routing: environment differs from the server's")
)

// ValidationError carries a message that is safe to return to the admin.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

// Gateways is satisfied by *gateway.Registry.
type Gateways interface {
	Get(code string) (gateway.Adapter, bool)
	Codes() []string
}

// Method is the payment method a routing choice must be able to carry.
const Method = "QRIS"

type Service struct {
	db  *pgxpool.Pool
	gws Gateways
	env string // the environment this server routes
}

func NewService(db *pgxpool.Pool, gws Gateways, env string) *Service {
	return &Service{db: db, gws: gws, env: env}
}

// Preferred returns the gateway chosen for this server's environment, or "" when none was chosen.
// It is read on every payment create: one primary-key lookup.
func (s *Service) Preferred(ctx context.Context) (string, error) {
	var gw string
	err := s.db.QueryRow(ctx, `SELECT gateway FROM payment_routing WHERE environment = $1`, s.env).Scan(&gw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return gw, err
}

// Option is a gateway the panel may offer.
type Option struct {
	Code string `json:"code"`
	// Available: registered on this server (credentials applied) and able to carry QRIS.
	Available bool `json:"available"`
	// PlatformAccount: the gateway needs no per-merchant account, so every approved merchant can use it.
	PlatformAccount bool `json:"platform_account"`
}

type Overview struct {
	ServerEnvironment string     `json:"server_environment"`
	Gateway           string     `json:"gateway"` // "" = not chosen (legacy behaviour)
	Version           int        `json:"version"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
	Options           []Option   `json:"options"`
}

// Candidates are the gateways that can ever be chosen. Listed explicitly so a registered adapter
// that is not a payment gateway never shows up as a choice.
var Candidates = []string{"doku", "ids"}

func (s *Service) options() []Option {
	out := make([]Option, 0, len(Candidates))
	for _, c := range Candidates {
		o := Option{Code: c}
		if ad, ok := s.gws.Get(c); ok {
			caps := ad.Capabilities()
			o.Available, o.PlatformAccount = caps.SupportsMethod(Method), caps.PlatformAccount
		}
		out = append(out, o)
	}
	return out
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	ov := Overview{ServerEnvironment: s.env, Options: s.options()}
	var at time.Time
	err := s.db.QueryRow(ctx, `SELECT gateway, version, updated_at FROM payment_routing WHERE environment = $1`, s.env).
		Scan(&ov.Gateway, &ov.Version, &at)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ov, nil
	case err != nil:
		return Overview{}, err
	}
	ov.UpdatedAt = &at
	return ov, nil
}

type SetInput struct {
	Environment   string
	Gateway       string
	ExpectVersion int // the version the form was loaded with; 0 when nothing was chosen yet
	Note          string
	ActorID       string
	RequestID     string
}

// Set chooses the gateway for new payments. The gateway must be registered on this server and carry
// QRIS, otherwise every payment would fall back and the choice would silently do nothing. A row lock
// plus ExpectVersion keep two admins from overwriting each other.
func (s *Service) Set(ctx context.Context, in SetInput) (Overview, error) {
	in.Note = strings.TrimSpace(in.Note)
	if n := utf8.RuneCountInString(in.Note); n < 3 || n > 500 {
		return Overview{}, invalid("alasan wajib diisi (3-500 karakter)")
	}
	if in.Environment != s.env {
		return Overview{}, ErrWrongEnv
	}
	var opt *Option
	for _, o := range s.options() {
		if o.Code == in.Gateway {
			o := o
			opt = &o
		}
	}
	switch {
	case opt == nil:
		return Overview{}, invalid("gateway tidak dikenal")
	case !opt.Available:
		return Overview{}, invalid("gateway %s belum aktif di server ini: simpan dan terapkan kredensialnya dulu", in.Gateway)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Overview{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `INSERT INTO payment_routing (environment, gateway, version) VALUES ($1, $2, 0)
		ON CONFLICT DO NOTHING`, s.env, in.Gateway); err != nil {
		return Overview{}, err
	}
	var prev string
	var version int
	if err := tx.QueryRow(ctx, `SELECT gateway, version FROM payment_routing WHERE environment = $1 FOR UPDATE`, s.env).Scan(&prev, &version); err != nil {
		return Overview{}, err
	}
	if version != in.ExpectVersion {
		return Overview{}, ErrConflict
	}
	if version > 0 && prev == in.Gateway {
		return Overview{}, invalid("jalur sudah %s", in.Gateway)
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "payment_routing.set", EntityType: "payment_routing",
		EntityID: s.env, RequestID: in.RequestID, Metadata: map[string]any{
			"environment": s.env, "from": prevOrNone(version, prev), "to": in.Gateway, "version": version + 1, "note": in.Note}}); err != nil {
		return Overview{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE payment_routing SET gateway = $2, version = version + 1, updated_by = NULLIF($3,'')::uuid, updated_at = now()
		WHERE environment = $1`, s.env, in.Gateway, in.ActorID); err != nil {
		return Overview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Overview{}, err
	}
	return s.Overview(ctx)
}

func prevOrNone(version int, gw string) string {
	if version == 0 {
		return ""
	}
	return gw
}
