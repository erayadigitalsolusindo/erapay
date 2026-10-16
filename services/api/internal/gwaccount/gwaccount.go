// Package gwaccount manages merchant_gateway_accounts: which gateway account (e.g. a DOKU
// merchant id) a merchant uses per environment, and whether it may take payments. Platform admins
// (DEWA) activate it by hand after confirming the id with the gateway; every change is audited.
package gwaccount

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var (
	ErrNotFound    = errors.New("gwaccount: not found")
	ErrUnsupported = errors.New("gwaccount: gateway cannot be tested")
	// ErrWrongEnv: the server talks to one gateway environment (ERAPAY_ENV); testing the other
	// one would silently test the wrong thing.
	ErrWrongEnv = errors.New("gwaccount: environment differs from the server's")
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

type Service struct {
	db  *pgxpool.Pool
	gws Gateways
	env string // "sandbox" | "production": the environment this server's adapters are wired to
}

func NewService(db *pgxpool.Pool, gws Gateways, env string) *Service {
	return &Service{db: db, gws: gws, env: env}
}

type Account struct {
	ID               string    `json:"id"`
	MerchantID       string    `json:"merchant_id"`
	Gateway          string    `json:"gateway"`
	Environment      string    `json:"environment"`
	GatewayRef       string    `json:"gateway_ref,omitempty"`
	ActivationStatus string    `json:"activation_status"`
	CreatedAt        time.Time `json:"created_at"`
}

// Overview is what the panel shows for one merchant: its accounts, the gateways this server has,
// and the environment this server is running in.
type Overview struct {
	ServerEnvironment string    `json:"server_environment"`
	Gateways          []string  `json:"gateways"`
	Accounts          []Account `json:"accounts"`
}

func (s *Service) List(ctx context.Context, merchantID string) (Overview, error) {
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM merchants WHERE id::text = $1)`, merchantID).Scan(&exists); err != nil {
		return Overview{}, err
	}
	if !exists {
		return Overview{}, ErrNotFound
	}
	rows, err := s.db.Query(ctx, `SELECT id::text, merchant_id::text, gateway, environment, coalesce(gateway_ref,''), activation_status, created_at
		FROM merchant_gateway_accounts WHERE merchant_id::text = $1 ORDER BY gateway, environment`, merchantID)
	if err != nil {
		return Overview{}, err
	}
	defer rows.Close()
	ov := Overview{ServerEnvironment: s.env, Gateways: s.gws.Codes(), Accounts: []Account{}}
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.MerchantID, &a.Gateway, &a.Environment, &a.GatewayRef, &a.ActivationStatus, &a.CreatedAt); err != nil {
			return Overview{}, err
		}
		ov.Accounts = append(ov.Accounts, a)
	}
	return ov, rows.Err()
}

// refRe: a gateway-side merchant id. Deliberately strict: it ends up in gateway requests.
var refRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type UpsertInput struct {
	MerchantID  string
	Gateway     string
	Environment string
	GatewayRef  string
	Status      string // pending_confirmation | active | suspended
	Note        string // why (required, goes to the audit log)
	ActorID     string
	RequestID   string
}

// Upsert creates or updates the account for (merchant, gateway, environment).
// Activating requires a gateway_ref; the merchant must already exist.
func (s *Service) Upsert(ctx context.Context, in UpsertInput) (Account, error) {
	in.Note = strings.TrimSpace(in.Note)
	in.GatewayRef = strings.TrimSpace(in.GatewayRef)
	if n := utf8.RuneCountInString(in.Note); n < 3 || n > 500 {
		return Account{}, invalid("note must be 3-500 characters")
	}
	if _, ok := s.gws.Get(in.Gateway); !ok {
		return Account{}, invalid("unknown gateway")
	}
	if in.Environment != "sandbox" && in.Environment != "production" {
		return Account{}, invalid("environment must be sandbox or production")
	}
	switch in.Status {
	case "pending_confirmation", "active", "suspended":
	default:
		return Account{}, invalid("status must be pending_confirmation, active or suspended")
	}
	if in.GatewayRef != "" && !refRe.MatchString(in.GatewayRef) {
		return Account{}, invalid("gateway_ref may only contain letters, digits, '.', '_' and '-' (max 64)")
	}
	if in.Status == "active" && in.GatewayRef == "" {
		return Account{}, invalid("gateway_ref is required to activate")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM merchants WHERE id::text = $1)`, in.MerchantID).Scan(&exists); err != nil {
		return Account{}, err
	}
	if !exists {
		return Account{}, ErrNotFound
	}
	var prevRef, prevStatus string
	err = tx.QueryRow(ctx, `SELECT coalesce(gateway_ref,''), activation_status FROM merchant_gateway_accounts
		WHERE merchant_id::text = $1 AND gateway = $2 AND environment = $3 FOR UPDATE`, in.MerchantID, in.Gateway, in.Environment).Scan(&prevRef, &prevStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Account{}, err
	}
	var a Account
	if err := tx.QueryRow(ctx, `INSERT INTO merchant_gateway_accounts (merchant_id, gateway, environment, gateway_ref, activation_status)
		VALUES ($1::uuid, $2, $3, NULLIF($4,''), $5)
		ON CONFLICT (merchant_id, gateway, environment)
		DO UPDATE SET gateway_ref = EXCLUDED.gateway_ref, activation_status = EXCLUDED.activation_status
		RETURNING id::text, merchant_id::text, gateway, environment, coalesce(gateway_ref,''), activation_status, created_at`,
		in.MerchantID, in.Gateway, in.Environment, in.GatewayRef, in.Status).
		Scan(&a.ID, &a.MerchantID, &a.Gateway, &a.Environment, &a.GatewayRef, &a.ActivationStatus, &a.CreatedAt); err != nil {
		return Account{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "gateway_account.set", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID, Metadata: map[string]any{
			"gateway": in.Gateway, "environment": in.Environment, "note": in.Note,
			"from_status": prevStatus, "to_status": in.Status, "from_ref": prevRef, "to_ref": in.GatewayRef,
		}}); err != nil {
		return Account{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Account{}, err
	}
	return a, nil
}

type TestInput struct {
	MerchantID  string
	Gateway     string
	Environment string
	Deep        bool
	ActorID     string
	RequestID   string
}

type TestResult struct {
	OK    bool                     `json:"ok"`
	Deep  bool                     `json:"deep"`
	Steps []gateway.ConnectionStep `json:"steps"`
}

// Test checks the server's credentials for a gateway and, when deep, the merchant's gateway_ref.
// It never changes the account; it only writes an audit entry.
func (s *Service) Test(ctx context.Context, in TestInput) (TestResult, error) {
	if in.Environment != s.env {
		return TestResult{}, ErrWrongEnv
	}
	ad, ok := s.gws.Get(in.Gateway)
	if !ok {
		return TestResult{}, invalid("unknown gateway")
	}
	tester, ok := ad.(gateway.ConnectionTester)
	if !ok {
		return TestResult{}, ErrUnsupported
	}
	var ref string
	err := s.db.QueryRow(ctx, `SELECT coalesce(gateway_ref,'') FROM merchant_gateway_accounts
		WHERE merchant_id::text = $1 AND gateway = $2 AND environment = $3`, in.MerchantID, in.Gateway, in.Environment).Scan(&ref)
	if errors.Is(err, pgx.ErrNoRows) {
		if in.Deep {
			return TestResult{}, ErrNotFound // a deep test needs the merchant's account row
		}
	} else if err != nil {
		return TestResult{}, err
	}
	steps := tester.TestConnection(ctx, ref, in.Deep)
	res := TestResult{OK: true, Deep: in.Deep, Steps: steps}
	for _, st := range steps {
		if !st.OK {
			res.OK = false
		}
	}
	// Audit outside any transaction: the test has no state to roll back.
	if err := audit.Record(ctx, s.db, audit.Entry{ActorID: in.ActorID, Action: "gateway_account.tested", EntityType: "merchant",
		EntityID: in.MerchantID, RequestID: in.RequestID, Metadata: map[string]any{
			"gateway": in.Gateway, "environment": in.Environment, "deep": in.Deep, "ok": res.OK,
		}}); err != nil {
		return TestResult{}, err
	}
	return res, nil
}
