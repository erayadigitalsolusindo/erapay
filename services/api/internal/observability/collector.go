// Package observability also samples queue depths periodically from Postgres.
package observability

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StartCollector samples gauges that would otherwise require a scrape-time DB query.
func StartCollector(ctx context.Context, pool *pgxpool.Pool, every time.Duration) {
	if every <= 0 {
		every = 30 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	// One immediate sample so /metrics is populated before the first tick.
	sampleOnce(ctx, pool)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sampleOnce(ctx, pool)
		}
	}
}

func sampleOnce(ctx context.Context, pool *pgxpool.Pool) {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var pendingUnknown, staleCreated int
	_ = pool.QueryRow(cctx, `
		SELECT count(*) FILTER (WHERE status IN ('pending','unknown') AND gateway_reference IS NOT NULL),
		       count(*) FILTER (WHERE status = 'created' AND created_at < now() - interval '2 minutes')
		FROM payments`).Scan(&pendingUnknown, &staleCreated)
	SetPaymentQueue(pendingUnknown, staleCreated)

	var webhookPending, webhookDead int
	_ = pool.QueryRow(cctx, `
		SELECT count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'dead')
		FROM webhook_deliveries`).Scan(&webhookPending, &webhookDead)
	SetWebhookQueue(webhookPending, webhookDead)

	var openExceptions int
	_ = pool.QueryRow(cctx, `SELECT count(*) FROM reconciliation_exceptions WHERE status = 'open'`).Scan(&openExceptions)
	SetReconciliationOpen(openExceptions)

	var payoutsPending int
	_ = pool.QueryRow(cctx, `SELECT count(*) FROM payouts WHERE status IN ('pending','approved')`).Scan(&payoutsPending)
	SetPayoutsPending(payoutsPending)

	var ledgerTotal int
	_ = pool.QueryRow(cctx, `SELECT count(*) FROM ledger_entries`).Scan(&ledgerTotal)
	// Ledger is append-only; set absolute value rather than incrementing.
	// Reset then add to avoid double counting across samples.
	ledgerEntriesTotal.Store(int64(ledgerTotal))
}
