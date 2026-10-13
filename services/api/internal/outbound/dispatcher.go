package outbound

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/observability"
)

type job struct {
	deliveryID, eventID, endpointID string
	attempts                        int
}

// DispatchOnce claims due deliveries and sends them. The claim pushes next_attempt_at forward
// by a lease under FOR UPDATE SKIP LOCKED, so several instances never send the same delivery
// concurrently, and an instance that crashes mid-send is retried after the lease (at-least-once).
func (s *Service) DispatchOnce(ctx context.Context, limit int) (int, error) {
	rows, err := s.db.Query(ctx, `
		UPDATE webhook_deliveries SET next_attempt_at = now() + $2::interval
		WHERE id IN (SELECT id FROM webhook_deliveries
		             WHERE status = 'pending' AND next_attempt_at <= now()
		             ORDER BY next_attempt_at LIMIT $1 FOR UPDATE SKIP LOCKED)
		RETURNING id::text, event_id::text, endpoint_id::text, attempts`,
		limit, fmt.Sprintf("%d seconds", int(leaseDuration.Seconds())))
	if err != nil {
		return 0, err
	}
	jobs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (job, error) {
		var j job
		return j, r.Scan(&j.deliveryID, &j.eventID, &j.endpointID, &j.attempts)
	})
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		if err := s.send(ctx, j); err != nil {
			s.log.Error("webhook delivery", "delivery_id", j.deliveryID, "err", err)
		}
	}
	return len(jobs), nil
}

func (s *Service) RunDispatcher(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.DispatchOnce(ctx, 20); err != nil && ctx.Err() == nil {
				s.log.Error("webhook dispatch", "err", err)
			}
		}
	}
}

type outcome struct {
	httpStatus int
	latencyMS  int
	errKind    string // "" on success; a short category, never raw error text
}

func (s *Service) send(ctx context.Context, j job) error {
	var (
		url, epStatus, evType, merchantID string
		secretEnc                         []byte
		data                              json.RawMessage
		evCreated                         time.Time
	)
	err := s.db.QueryRow(ctx, `
		SELECT ep.url, ep.status, ep.secret_enc, e.type, e.data, e.created_at, e.merchant_id::text
		FROM webhook_endpoints ep, webhook_events e WHERE ep.id = $1 AND e.id = $2`,
		j.endpointID, j.eventID).Scan(&url, &epStatus, &secretEnc, &evType, &data, &evCreated, &merchantID)
	if err != nil {
		return err
	}
	if epStatus != "active" {
		_, err := s.db.Exec(ctx, `UPDATE webhook_deliveries SET status = 'cancelled' WHERE id = $1 AND status = 'pending'`, j.deliveryID)
		return err
	}
	secret, err := s.box.Open(secretEnc, []byte(j.endpointID))
	if err != nil {
		return s.finish(ctx, j, outcome{errKind: "secret_unreadable"})
	}

	// The envelope carries the event id so consumers can deduplicate (delivery is at-least-once).
	body, err := json.Marshal(map[string]any{"id": j.eventID, "type": evType, "created_at": evCreated.UTC(), "data": data})
	if err != nil {
		return err
	}
	return s.finish(ctx, j, s.post(ctx, url, string(secret), j, body))
}

func (s *Service) post(ctx context.Context, url, secret string, j job, body []byte) outcome {
	if err := ValidateURL(ctx, url, s.resolver, s.allowPrivate); err != nil {
		return outcome{errKind: "blocked_destination"}
	}
	rctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return outcome{errKind: "bad_request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "EraPay-Webhook/1")
	req.Header.Set(SignatureHeader, Sign(secret, s.now(), body))
	req.Header.Set("X-EraPay-Event-Id", j.eventID)
	req.Header.Set("X-EraPay-Delivery-Attempt", strconv.Itoa(j.attempts+1))

	start := time.Now()
	resp, err := s.client.Do(req)
	o := outcome{latencyMS: int(time.Since(start).Milliseconds())}
	if err != nil {
		switch {
		case errors.Is(err, ErrBlockedAddress):
			o.errKind = "blocked_destination"
		case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
			o.errKind = "timeout"
		default:
			o.errKind = "connection_error"
		}
		return o
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	o.httpStatus = resp.StatusCode
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		o.errKind = "redirect_not_followed"
	default:
		o.errKind = "http_" + strconv.Itoa(resp.StatusCode)
	}
	return o
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// finish records the attempt and settles the delivery in one transaction.
func (s *Service) finish(ctx context.Context, j job, o outcome) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	n := j.attempts + 1
	if _, err := tx.Exec(ctx, `
		INSERT INTO webhook_delivery_attempts (delivery_id, attempt_no, http_status, latency_ms, error)
		VALUES ($1,$2,NULLIF($3,0),$4,NULLIF($5,''))`, j.deliveryID, n, o.httpStatus, o.latencyMS, o.errKind); err != nil {
		return err
	}
	switch {
	case o.errKind == "":
		_, err = tx.Exec(ctx, `
			UPDATE webhook_deliveries SET status = 'succeeded', attempts = $2, delivered_at = now(),
			    last_http_status = $3, last_latency_ms = $4, last_error = NULL
			WHERE id = $1 AND status = 'pending'`, j.deliveryID, n, o.httpStatus, o.latencyMS)
	case n >= MaxAttempts:
		observability.IncWebhookDead()
		_, err = tx.Exec(ctx, `
			UPDATE webhook_deliveries SET status = 'dead', attempts = $2,
			    last_http_status = NULLIF($3,0), last_latency_ms = $4, last_error = $5
			WHERE id = $1 AND status = 'pending'`, j.deliveryID, n, o.httpStatus, o.latencyMS, o.errKind)
	default:
		wait := Backoff(n, jitter())
		_, err = tx.Exec(ctx, `
			UPDATE webhook_deliveries SET attempts = $2, next_attempt_at = now() + $6::interval,
			    last_http_status = NULLIF($3,0), last_latency_ms = $4, last_error = $5
			WHERE id = $1 AND status = 'pending'`, j.deliveryID, n, o.httpStatus, o.latencyMS, o.errKind,
			fmt.Sprintf("%d milliseconds", wait.Milliseconds()))
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
