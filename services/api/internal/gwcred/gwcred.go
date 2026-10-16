// Package gwcred stores platform-level gateway credentials (e.g. DOKU client id, secret key and
// private key) encrypted in the database, so DEWA can rotate them from the panel instead of
// editing .env on the server. Values are write-only: after saving, only a masked preview is
// ever returned. Without a row for a gateway the server keeps using its environment variables.
package gwcred

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
)

var (
	ErrNotFound = errors.New("gwcred: gateway has no credential form")
	// ErrConflict: someone saved a newer version since the form was loaded.
	ErrConflict = errors.New("gwcred: credentials changed since you opened the form")
	// ErrWrongEnv: only the environment the server is wired to can be tested or applied live.
	ErrWrongEnv = errors.New("gwcred: environment differs from the server's")
	// ErrIncomplete: required fields are still missing.
	ErrIncomplete = errors.New("gwcred: credentials are incomplete")
	// ErrNoFactory: no way to build an adapter for this gateway on this server.
	ErrNoFactory = errors.New("gwcred: gateway cannot be applied live")
)

// ValidationError carries a message that is safe to return to the admin.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

// Factory builds a ready adapter from stored credentials. It lives in cmd/api, the only place
// allowed to import a concrete gateway package (PRD §0 #11).
type Factory func(creds map[string]string, env string) (gateway.Adapter, error)

// Live is where an adapter is swapped in; satisfied by *gateway.Registry.
type Live interface{ Replace(a gateway.Adapter) }

type Service struct {
	db        *pgxpool.Pool
	box       *secretbox.Box
	env       string // the environment this server's adapters are wired to
	live      Live
	factories map[string]Factory
}

func NewService(db *pgxpool.Pool, box *secretbox.Box, env string, live Live, factories map[string]Factory) *Service {
	return &Service{db: db, box: box, env: env, live: live, factories: factories}
}

// SetLive wires the registry that Apply swaps adapters into. It is separate from NewService
// because the registry itself is built from these credentials at startup.
func (s *Service) SetLive(l Live) { s.live = l }

// Env is the environment this server's adapters are wired to.
func (s *Service) Env() string { return s.env }

func aad(gw, env string) []byte { return []byte("gwcred/" + gw + "/" + env) }

func validEnv(env string) bool { return env == "sandbox" || env == "production" }

// FieldStatus is one field as the panel sees it. Value is empty for secrets.
type FieldStatus struct {
	Field
	Set     bool   `json:"set"`
	Preview string `json:"preview,omitempty"`
}

type Status struct {
	Gateway     string        `json:"gateway"`
	Label       string        `json:"label"`
	Environment string        `json:"environment"`
	Version     int           `json:"version"`
	Configured  bool          `json:"configured"` // a row with at least one value exists
	Complete    bool          `json:"complete"`   // every required field is set
	UpdatedAt   *time.Time    `json:"updated_at,omitempty"`
	Fields      []FieldStatus `json:"fields"`
}

func (s *Service) open(sealed []byte, gw, env string) (map[string]string, error) {
	out := map[string]string{}
	if len(sealed) == 0 {
		return out, nil
	}
	plain, err := s.box.Open(sealed, aad(gw, env))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, secretbox.ErrCorrupt
	}
	return out, nil
}

func statusOf(sc Schema, env string, vals map[string]string, version int, at *time.Time) Status {
	st := Status{Gateway: sc.Gateway, Label: sc.Label, Environment: env, Version: version, UpdatedAt: at,
		Configured: len(vals) > 0, Complete: true, Fields: make([]FieldStatus, 0, len(sc.Fields))}
	for _, f := range sc.Fields {
		v, set := vals[f.Key]
		set = set && v != ""
		fs := FieldStatus{Field: f, Set: set}
		if set {
			fs.Preview = f.preview(v)
		}
		if f.Required && !set {
			st.Complete = false
		}
		st.Fields = append(st.Fields, fs)
	}
	return st
}

// Status returns the masked state of one gateway/environment.
func (s *Service) Status(ctx context.Context, gw, env string) (Status, error) {
	sc, ok := SchemaOf(gw)
	if !ok || !validEnv(env) {
		return Status{}, ErrNotFound
	}
	var sealed []byte
	var version int
	var at time.Time
	err := s.db.QueryRow(ctx, `SELECT sealed, version, updated_at FROM gateway_credentials WHERE gateway = $1 AND environment = $2`, gw, env).
		Scan(&sealed, &version, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return statusOf(sc, env, nil, 0, nil), nil
	}
	if err != nil {
		return Status{}, err
	}
	vals, err := s.open(sealed, gw, env)
	if err != nil {
		return Status{}, err
	}
	return statusOf(sc, env, vals, version, &at), nil
}

// Load returns the decrypted credentials, for building an adapter. ok=false when none are stored
// or they are incomplete, so the caller falls back to the environment.
func (s *Service) Load(ctx context.Context, gw, env string) (map[string]string, bool, error) {
	sc, found := SchemaOf(gw)
	if !found || !validEnv(env) {
		return nil, false, ErrNotFound
	}
	var sealed []byte
	err := s.db.QueryRow(ctx, `SELECT sealed FROM gateway_credentials WHERE gateway = $1 AND environment = $2`, gw, env).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	vals, err := s.open(sealed, gw, env)
	if err != nil {
		return nil, false, err
	}
	for _, f := range sc.Fields {
		if f.Required && vals[f.Key] == "" {
			return nil, false, nil
		}
	}
	return vals, true, nil
}

type SetInput struct {
	Gateway       string
	Environment   string
	Values        map[string]string // empty value = keep what is stored
	Clear         []string          // optional fields to remove
	ExpectVersion int               // the version the form was loaded with; 0 for a new row
	Note          string
	ActorID       string
	RequestID     string
}

// Set merges Values into the stored credentials. It validates every field, takes a row lock and
// checks ExpectVersion, so concurrent edits cannot silently overwrite each other. It does NOT
// change what the server uses: that needs Apply.
func (s *Service) Set(ctx context.Context, in SetInput) (Status, error) {
	sc, ok := SchemaOf(in.Gateway)
	if !ok || !validEnv(in.Environment) {
		return Status{}, ErrNotFound
	}
	in.Note = strings.TrimSpace(in.Note)
	if n := len([]rune(in.Note)); n < 3 || n > 500 {
		return Status{}, invalid("alasan wajib diisi (3-500 karakter)")
	}
	clean := map[string]string{}
	for k, v := range in.Values {
		f, ok := sc.field(k)
		if !ok {
			return Status{}, invalid("field %q tidak dikenal untuk %s", k, sc.Label)
		}
		v = f.normalize(v)
		if v == "" {
			continue
		}
		if err := f.check(v); err != nil {
			return Status{}, err
		}
		clean[k] = v
	}
	for _, k := range in.Clear {
		f, ok := sc.field(k)
		if !ok {
			return Status{}, invalid("field %q tidak dikenal untuk %s", k, sc.Label)
		}
		if f.Required {
			return Status{}, invalid("%s wajib dan tidak bisa dihapus", f.Label)
		}
		if _, both := clean[k]; both {
			return Status{}, invalid("%s tidak bisa diisi sekaligus dihapus", f.Label)
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Status{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `INSERT INTO gateway_credentials (gateway, environment, sealed, version) VALUES ($1, $2, ''::bytea, 0)
		ON CONFLICT DO NOTHING`, in.Gateway, in.Environment); err != nil {
		return Status{}, err
	}
	var sealed []byte
	var version int
	if err := tx.QueryRow(ctx, `SELECT sealed, version FROM gateway_credentials WHERE gateway = $1 AND environment = $2 FOR UPDATE`,
		in.Gateway, in.Environment).Scan(&sealed, &version); err != nil {
		return Status{}, err
	}
	if version != in.ExpectVersion {
		return Status{}, ErrConflict
	}
	vals, err := s.open(sealed, in.Gateway, in.Environment)
	if err != nil {
		return Status{}, err
	}
	changed := make([]string, 0, len(clean))
	for k, v := range clean {
		if vals[k] != v {
			vals[k] = v
			changed = append(changed, k)
		}
	}
	cleared := make([]string, 0, len(in.Clear))
	for _, k := range in.Clear {
		if _, had := vals[k]; had {
			delete(vals, k)
			cleared = append(cleared, k)
		}
	}
	sort.Strings(changed)
	sort.Strings(cleared)
	if len(changed) == 0 && len(cleared) == 0 {
		return Status{}, invalid("tidak ada perubahan")
	}
	plain, err := json.Marshal(vals)
	if err != nil {
		return Status{}, err
	}
	sealed, err = s.box.Seal(plain, aad(in.Gateway, in.Environment))
	if err != nil {
		return Status{}, err
	}
	var at time.Time
	if err := tx.QueryRow(ctx, `UPDATE gateway_credentials SET sealed = $3, version = version + 1, updated_by = NULLIF($4,'')::uuid, updated_at = now()
		WHERE gateway = $1 AND environment = $2 RETURNING updated_at`, in.Gateway, in.Environment, sealed, in.ActorID).Scan(&at); err != nil {
		return Status{}, err
	}
	// Field NAMES only: values never reach the audit log.
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: in.ActorID, Action: "gateway_credentials.set", EntityType: "gateway",
		EntityID: in.Gateway, RequestID: in.RequestID, Metadata: map[string]any{
			"environment": in.Environment, "changed": changed, "cleared": cleared, "version": version + 1, "note": in.Note,
		}}); err != nil {
		return Status{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Status{}, err
	}
	return statusOf(sc, in.Environment, vals, version+1, &at), nil
}

type TestResult struct {
	OK    bool                     `json:"ok"`
	Steps []gateway.ConnectionStep `json:"steps"`
}

// candidate builds an adapter from the stored credentials without touching the live one, and
// runs the credentials-only connection test (no merchant data, no money).
func (s *Service) candidate(ctx context.Context, gw, env string) (gateway.Adapter, TestResult, error) {
	if env != s.env {
		return nil, TestResult{}, ErrWrongEnv
	}
	fac := s.factories[gw]
	if fac == nil {
		return nil, TestResult{}, ErrNoFactory
	}
	vals, ok, err := s.Load(ctx, gw, env)
	if err != nil {
		return nil, TestResult{}, err
	}
	if !ok {
		return nil, TestResult{}, ErrIncomplete
	}
	ad, err := fac(vals, env)
	if err != nil {
		// Factory errors are about the shape of the credentials (bad PEM, wrong URL for the env);
		// they never contain the values themselves.
		return nil, TestResult{Steps: []gateway.ConnectionStep{{Name: "credentials", Message: err.Error()}}}, nil
	}
	tester, ok := ad.(gateway.ConnectionTester)
	if !ok {
		return ad, TestResult{OK: true, Steps: []gateway.ConnectionStep{{Name: "credentials", OK: true, Message: "gateway has no connection test"}}}, nil
	}
	res := TestResult{OK: true, Steps: tester.TestConnection(ctx, "", false)}
	for _, st := range res.Steps {
		if !st.OK {
			res.OK = false
		}
	}
	return ad, res, nil
}

type ActInput struct {
	Gateway     string
	Environment string
	ActorID     string
	RequestID   string
}

// Test checks the stored credentials against the gateway. The live adapter is not touched.
func (s *Service) Test(ctx context.Context, in ActInput) (TestResult, error) {
	_, res, err := s.candidate(ctx, in.Gateway, in.Environment)
	if err != nil {
		return TestResult{}, err
	}
	if err := audit.Record(ctx, s.db, audit.Entry{ActorID: in.ActorID, Action: "gateway_credentials.tested", EntityType: "gateway",
		EntityID: in.Gateway, RequestID: in.RequestID, Metadata: map[string]any{"environment": in.Environment, "ok": res.OK}}); err != nil {
		return TestResult{}, err
	}
	return res, nil
}

// Apply swaps the live adapter for one built from the stored credentials, but only when the
// connection test passes. A failing candidate leaves the working adapter untouched.
func (s *Service) Apply(ctx context.Context, in ActInput) (TestResult, bool, error) {
	ad, res, err := s.candidate(ctx, in.Gateway, in.Environment)
	if err != nil {
		return TestResult{}, false, err
	}
	applied := res.OK && ad != nil
	if applied {
		s.live.Replace(ad)
	}
	if err := audit.Record(ctx, s.db, audit.Entry{ActorID: in.ActorID, Action: "gateway_credentials.applied", EntityType: "gateway",
		EntityID: in.Gateway, RequestID: in.RequestID, Metadata: map[string]any{"environment": in.Environment, "ok": res.OK, "applied": applied}}); err != nil {
		return res, applied, err
	}
	return res, applied, nil
}
