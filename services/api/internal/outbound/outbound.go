// Package outbound delivers signed webhooks to merchants (PRD webhook-outbound.md).
package outbound

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
)

var (
	ErrNotFound      = errors.New("outbound: not found")
	ErrTooManyEndpts = errors.New("outbound: endpoint limit reached")
	ErrNotRedeliver  = errors.New("outbound: delivery cannot be redelivered")
)

const (
	MaxActiveEndpoints = 5
	sendTimeout        = 10 * time.Second
	leaseDuration      = 2 * time.Minute // an in-flight attempt that crashes is retried after this
)

func randBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

type Service struct {
	db           *pgxpool.Pool
	box          *secretbox.Box
	log          *slog.Logger
	resolver     Resolver
	client       *http.Client
	allowPrivate bool // tests only: lets httptest servers on loopback receive deliveries
	now          func() time.Time
}

func NewService(db *pgxpool.Pool, box *secretbox.Box, log *slog.Logger) *Service {
	return &Service{db: db, box: box, log: log, client: safeClient(sendTimeout, false), now: time.Now}
}

// EnqueueTx records the event and one pending delivery per active endpoint of the merchant,
// inside the caller's transaction. A second call for the same (type, resource) is a no-op,
// so a repeated gateway notification cannot notify the merchant twice (PRD §0 #5).
// data must already be minimal: no secrets, no payer PII.
func EnqueueTx(ctx context.Context, tx pgx.Tx, merchantID, eventType, resourceID string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var eventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO webhook_events (merchant_id, type, resource_id, data) VALUES ($1,$2,$3,$4)
		ON CONFLICT (type, resource_id) DO NOTHING RETURNING id::text`,
		merchantID, eventType, resourceID, b).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO webhook_deliveries (merchant_id, event_id, endpoint_id)
		SELECT merchant_id, $2, id FROM webhook_endpoints WHERE merchant_id = $1 AND status = 'active'`,
		merchantID, eventID)
	return err
}

// ---- endpoint management (all queries are tenant-scoped) ----

type Endpoint struct {
	ID         string     `json:"id"`
	URL        string     `json:"url"`
	Status     string     `json:"status"`
	SecretHint string     `json:"secret_hint"`
	CreatedAt  time.Time  `json:"created_at"`
	DisabledAt *time.Time `json:"disabled_at"`
	Secret     string     `json:"secret,omitempty"` // only in the create / rotate response
}

const endpointCols = `id::text, url, status, secret_hint, created_at, disabled_at`

func scanEndpoint(row pgx.Row) (Endpoint, error) {
	var e Endpoint
	err := row.Scan(&e.ID, &e.URL, &e.Status, &e.SecretHint, &e.CreatedAt, &e.DisabledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

type Actor struct{ UserID, RequestID string }

func (s *Service) sealSecret(endpointID, secret string) ([]byte, error) {
	return s.box.Seal([]byte(secret), []byte(endpointID))
}

func (s *Service) CreateEndpoint(ctx context.Context, merchantID, rawURL string, a Actor) (Endpoint, error) {
	if err := ValidateURL(ctx, rawURL, s.resolver, s.allowPrivate); err != nil {
		return Endpoint{}, err
	}
	secret, err := newSecret()
	if err != nil {
		return Endpoint{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Endpoint{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Serialize endpoint creation per merchant so the limit cannot be raced past.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM merchants WHERE id = $1 FOR NO KEY UPDATE`, merchantID); err != nil {
		return Endpoint{}, err
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM webhook_endpoints WHERE merchant_id = $1 AND status = 'active'`, merchantID).Scan(&n); err != nil {
		return Endpoint{}, err
	}
	if n >= MaxActiveEndpoints {
		return Endpoint{}, ErrTooManyEndpts
	}
	var id string
	if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		return Endpoint{}, err
	}
	sealed, err := s.sealSecret(id, secret)
	if err != nil {
		return Endpoint{}, err
	}
	e, err := scanEndpoint(tx.QueryRow(ctx, `
		INSERT INTO webhook_endpoints (id, merchant_id, url, secret_enc, secret_hint, created_by)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid) RETURNING `+endpointCols,
		id, merchantID, rawURL, sealed, secret[len(secret)-4:], a.UserID))
	if err != nil {
		return Endpoint{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "webhook_endpoint.create",
		EntityType: "webhook_endpoint", EntityID: id, Metadata: map[string]any{"merchant_id": merchantID, "url": rawURL}, RequestID: a.RequestID}); err != nil {
		return Endpoint{}, err
	}
	e.Secret = secret
	return e, tx.Commit(ctx)
}

func (s *Service) ListEndpoints(ctx context.Context, merchantID string) ([]Endpoint, error) {
	rows, err := s.db.Query(ctx, `SELECT `+endpointCols+` FROM webhook_endpoints WHERE merchant_id = $1 ORDER BY created_at DESC`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Endpoint{}
	for rows.Next() {
		var e Endpoint
		if err := rows.Scan(&e.ID, &e.URL, &e.Status, &e.SecretHint, &e.CreatedAt, &e.DisabledAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RotateSecret replaces the signing secret immediately; the new secret is returned once.
func (s *Service) RotateSecret(ctx context.Context, merchantID, id string, a Actor) (Endpoint, error) {
	secret, err := newSecret()
	if err != nil {
		return Endpoint{}, err
	}
	sealed, err := s.sealSecret(id, secret)
	if err != nil {
		return Endpoint{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Endpoint{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	e, err := scanEndpoint(tx.QueryRow(ctx, `
		UPDATE webhook_endpoints SET secret_enc = $3, secret_hint = $4
		WHERE id = $1 AND merchant_id = $2 AND status = 'active' RETURNING `+endpointCols,
		id, merchantID, sealed, secret[len(secret)-4:]))
	if err != nil {
		return Endpoint{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "webhook_endpoint.rotate_secret",
		EntityType: "webhook_endpoint", EntityID: id, Metadata: map[string]any{"merchant_id": merchantID}, RequestID: a.RequestID}); err != nil {
		return Endpoint{}, err
	}
	e.Secret = secret
	return e, tx.Commit(ctx)
}

// Disable stops future deliveries; pending ones are cancelled by the dispatcher.
func (s *Service) Disable(ctx context.Context, merchantID, id string, a Actor) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, `UPDATE webhook_endpoints SET status = 'disabled', disabled_at = now()
		WHERE id = $1 AND merchant_id = $2 AND status = 'active'`, id, merchantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE webhook_deliveries SET status = 'cancelled'
		WHERE endpoint_id = $1 AND status = 'pending'`, id); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "webhook_endpoint.disable",
		EntityType: "webhook_endpoint", EntityID: id, Metadata: map[string]any{"merchant_id": merchantID}, RequestID: a.RequestID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---- delivery history ----

type Delivery struct {
	ID             string     `json:"id"`
	EndpointID     string     `json:"endpoint_id"`
	EventID        string     `json:"event_id"`
	EventType      string     `json:"event_type"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	NextAttemptAt  *time.Time `json:"next_attempt_at"`
	LastHTTPStatus *int       `json:"last_http_status"`
	LastLatencyMS  *int       `json:"last_latency_ms"`
	LastError      *string    `json:"last_error"`
	RedeliveryOf   *string    `json:"redelivery_of"`
	DeliveredAt    *time.Time `json:"delivered_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type DeliveryFilter struct {
	EndpointID string
	Status     string
	Before     *time.Time // cursor: created_at of the last row of the previous page
	Limit      int
}

func (s *Service) ListDeliveries(ctx context.Context, merchantID string, f DeliveryFilter) ([]Delivery, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT d.id::text, d.endpoint_id::text, d.event_id::text, e.type, d.status, d.attempts,
		       CASE WHEN d.status = 'pending' THEN d.next_attempt_at END,
		       d.last_http_status, d.last_latency_ms, d.last_error, d.redelivery_of::text, d.delivered_at, d.created_at
		FROM webhook_deliveries d JOIN webhook_events e ON e.id = d.event_id
		WHERE d.merchant_id = $1
		  AND ($2 = '' OR d.endpoint_id::text = $2)
		  AND ($3 = '' OR d.status = $3)
		  AND ($4::timestamptz IS NULL OR d.created_at < $4)
		ORDER BY d.created_at DESC, d.id LIMIT $5`,
		merchantID, f.EndpointID, f.Status, f.Before, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Delivery{}
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.EndpointID, &d.EventID, &d.EventType, &d.Status, &d.Attempts, &d.NextAttemptAt,
			&d.LastHTTPStatus, &d.LastLatencyMS, &d.LastError, &d.RedeliveryOf, &d.DeliveredAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Redeliver queues a fresh delivery of the same event to the same (still active) endpoint.
// History of the original is kept; a pending delivery cannot be redelivered (it is already queued).
func (s *Service) Redeliver(ctx context.Context, merchantID, deliveryID string, a Actor) (Delivery, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Delivery{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var eventID, endpointID, status string
	err = tx.QueryRow(ctx, `
		SELECT d.event_id::text, d.endpoint_id::text, d.status
		FROM webhook_deliveries d JOIN webhook_endpoints ep ON ep.id = d.endpoint_id
		WHERE d.id = $1 AND d.merchant_id = $2 AND ep.status = 'active' FOR UPDATE OF d`,
		deliveryID, merchantID).Scan(&eventID, &endpointID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, ErrNotFound
	}
	if err != nil {
		return Delivery{}, err
	}
	if status == "pending" {
		return Delivery{}, ErrNotRedeliver
	}
	var nd Delivery
	err = tx.QueryRow(ctx, `
		INSERT INTO webhook_deliveries (merchant_id, event_id, endpoint_id, redelivery_of)
		VALUES ($1,$2,$3,$4) RETURNING id::text, created_at`, merchantID, eventID, endpointID, deliveryID).
		Scan(&nd.ID, &nd.CreatedAt)
	if err != nil {
		return Delivery{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "webhook_delivery.redeliver",
		EntityType: "webhook_delivery", EntityID: nd.ID, Metadata: map[string]any{"merchant_id": merchantID, "original": deliveryID}, RequestID: a.RequestID}); err != nil {
		return Delivery{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Delivery{}, err
	}
	nd.EndpointID, nd.EventID, nd.Status, nd.RedeliveryOf = endpointID, eventID, "pending", &deliveryID
	return nd, nil
}
