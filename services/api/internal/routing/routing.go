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
var Candidates = []string{"doku", "ids", "finpay", "dana", "midtrans", "qrisgate"}

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

// ---- Override per merchant ----

// MerchantPreferred returns the gateway chosen for one merchant ("" = follow the platform) and whether a payment may
// fall back to the normal route when that gateway cannot take the merchant. One primary-key lookup per payment create.
func (s *Service) MerchantPreferred(ctx context.Context, merchantID string) (gw string, fallback bool, err error) {
	err = s.db.QueryRow(ctx, `SELECT gateway, fallback FROM merchant_payment_routing WHERE merchant_id::text = $1 AND environment = $2`,
		merchantID, s.env).Scan(&gw, &fallback)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return gw, fallback, err
}

// ErrMerchantNotFound: no such merchant.
var ErrMerchantNotFound = errors.New("routing: merchant not found")

type MerchantOverview struct {
	ServerEnvironment string   `json:"server_environment"`
	Gateway           string   `json:"gateway"` // "" = follows the platform choice
	Fallback          bool     `json:"fallback"`
	Version           int      `json:"version"`
	PlatformGateway   string   `json:"platform_gateway"` // what "follow the platform" means right now ("" = none chosen)
	Options           []Option `json:"options"`          // gateways this merchant can actually use right now
}

// merchantOptions: the gateways the merchant can be sent to now = registered, carry QRIS, and (unless the gateway
// is contracted once for the platform) the merchant has an ACTIVE account for it on this server's environment.
func (s *Service) merchantOptions(ctx context.Context, merchantID string) ([]Option, error) {
	rows, err := s.db.Query(ctx, `SELECT gateway FROM merchant_gateway_accounts
		WHERE merchant_id::text = $1 AND environment = $2 AND activation_status = 'active' AND gateway_ref IS NOT NULL`, merchantID, s.env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	active := map[string]bool{}
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		active[g] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Option
	for _, code := range Candidates {
		ad, ok := s.gws.Get(code)
		if !ok || !ad.Capabilities().SupportsMethod(Method) {
			continue
		}
		pa := ad.Capabilities().PlatformAccount
		if pa || active[code] {
			out = append(out, Option{Code: code, Available: true, PlatformAccount: pa})
		}
	}
	return out, nil
}

func (s *Service) MerchantOverview(ctx context.Context, merchantID string) (MerchantOverview, error) {
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM merchants WHERE id::text = $1)`, merchantID).Scan(&exists); err != nil {
		return MerchantOverview{}, err
	}
	if !exists {
		return MerchantOverview{}, ErrMerchantNotFound
	}
	ov := MerchantOverview{ServerEnvironment: s.env}
	err := s.db.QueryRow(ctx, `SELECT gateway, fallback, version FROM merchant_payment_routing WHERE merchant_id::text = $1 AND environment = $2`,
		merchantID, s.env).Scan(&ov.Gateway, &ov.Fallback, &ov.Version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return MerchantOverview{}, err
	}
	if ov.PlatformGateway, err = s.Preferred(ctx); err != nil {
		return MerchantOverview{}, err
	}
	if ov.Options, err = s.merchantOptions(ctx, merchantID); err != nil {
		return MerchantOverview{}, err
	}
	return ov, nil
}

type SetMerchantInput struct {
	MerchantID    string
	Environment   string
	Gateway       string // "" = remove the override (follow the platform)
	Fallback      bool
	ExpectVersion int // version the form was loaded with; 0 when there was no override
	Note          string
	ActorID       string
	RequestID     string
}

// SetMerchant sets or removes one merchant's override. A gateway the merchant cannot use right now is refused, so the
// choice can never be a silent no-op. A row lock plus ExpectVersion keep two admins from overwriting each other.
func (s *Service) SetMerchant(ctx context.Context, in SetMerchantInput) (MerchantOverview, error) {
	in.Note = strings.TrimSpace(in.Note)
	if n := utf8.RuneCountInString(in.Note); n < 3 || n > 500 {
		return MerchantOverview{}, invalid("alasan wajib diisi (3-500 karakter)")
	}
	if in.Environment != s.env {
		return MerchantOverview{}, ErrWrongEnv
	}
	ov, err := s.MerchantOverview(ctx, in.MerchantID) // also proves the merchant exists
	if err != nil {
		return MerchantOverview{}, err
	}
	if in.Gateway != "" {
		ok := false
		for _, o := range ov.Options {
			ok = ok || o.Code == in.Gateway
		}
		if !ok {
			return MerchantOverview{}, invalid("gateway %s belum bisa dipakai tenant ini: aktifkan akun gateway-nya dulu", in.Gateway)
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MerchantOverview{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var prev string
	var version int
	err = tx.QueryRow(ctx, `SELECT gateway, version FROM merchant_payment_routing WHERE merchant_id::text = $1 AND environment = $2 FOR UPDATE`,
		in.MerchantID, s.env).Scan(&prev, &version)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return MerchantOverview{}, err
	}
	if version != in.ExpectVersion {
		return MerchantOverview{}, ErrConflict
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "merchant_payment_routing.set", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID, Metadata: map[string]any{
			"environment": s.env, "from": prev, "to": in.Gateway, "fallback": in.Fallback, "version": version + 1, "note": in.Note}}); err != nil {
		return MerchantOverview{}, err
	}
	if in.Gateway == "" {
		_, err = tx.Exec(ctx, `DELETE FROM merchant_payment_routing WHERE merchant_id::text = $1 AND environment = $2`, in.MerchantID, s.env)
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO merchant_payment_routing (merchant_id, environment, gateway, fallback, version, updated_by)
			VALUES ($1::uuid, $2, $3, $4, 1, NULLIF($5,'')::uuid)
			ON CONFLICT (merchant_id, environment) DO UPDATE SET gateway = EXCLUDED.gateway, fallback = EXCLUDED.fallback,
			version = merchant_payment_routing.version + 1, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			in.MerchantID, s.env, in.Gateway, in.Fallback, in.ActorID)
	}
	if err != nil {
		return MerchantOverview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MerchantOverview{}, err
	}
	return s.MerchantOverview(ctx, in.MerchantID)
}

// Effective is the gateway a NEW QRIS payment of this merchant goes to right now, in the order the orchestrator uses:
// the merchant's override, then the platform's choice, then the merchant's oldest active account. "" = none can serve.
// Used to decide what a merchant should be shown (e.g. the QRISGate QRIS settings); the orchestrator stays the authority.
func (s *Service) Effective(ctx context.Context, merchantID string) (string, error) {
	opts, err := s.merchantOptions(ctx, merchantID)
	if err != nil {
		return "", err
	}
	usable := func(code string) bool {
		for _, o := range opts {
			if o.Code == code {
				return true
			}
		}
		return false
	}
	if g, _, err := s.MerchantPreferred(ctx, merchantID); err != nil {
		return "", err
	} else if g != "" && usable(g) {
		return g, nil
	}
	if g, err := s.Preferred(ctx); err != nil {
		return "", err
	} else if g != "" && usable(g) {
		return g, nil
	}
	var first string
	err = s.db.QueryRow(ctx, `SELECT gateway FROM merchant_gateway_accounts
		WHERE merchant_id::text = $1 AND environment = $2 AND activation_status = 'active' AND gateway_ref IS NOT NULL
		ORDER BY created_at LIMIT 1`, merchantID, s.env).Scan(&first)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return first, err
}
