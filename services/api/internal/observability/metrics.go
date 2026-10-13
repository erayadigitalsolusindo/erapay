// Package observability exposes Prometheus-style metrics without an external dependency.
// It intentionally uses the exposition text format directly so no prom client library is vendored.
// PRD NFR §8 requires rate, latency, error, gateway latency/error per gateway, webhook, queue age.
package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---- HTTP ----

var (
	httpRequestsTotal = newCounterVec() // labels: method, route, code
	httpDurationMs    = newHistogramVec([]float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000})
	readyzOK          atomic.Bool
)

func ObserveHTTP(method, route, code string, d time.Duration) {
	httpRequestsTotal.Inc(method, route, code)
	httpDurationMs.Observe(route, float64(d.Milliseconds()))
}

func SetReadyz(ok bool) { readyzOK.Store(ok) }

// ---- Gateway (create / inquiry) ----

var (
	gatewayRequestsTotal = newCounterVec() // labels: gateway, operation, outcome
	gatewayDurationMs    = newHistogramVec([]float64{50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000})
)

func ObserveGateway(gateway, operation, outcome string, d time.Duration) {
	gatewayRequestsTotal.Inc(gateway, operation, outcome)
	gatewayDurationMs.Observe(gateway, float64(d.Milliseconds()))
}

// ---- Payment queue depth (gauges sampled on poll) ----

var (
	pendingUnknownGauge atomic.Int64
	staleCreatedGauge   atomic.Int64
)

func SetPaymentQueue(pendingUnknown, staleCreated int) {
	pendingUnknownGauge.Store(int64(pendingUnknown))
	staleCreatedGauge.Store(int64(staleCreated))
}

// ---- Webhook outbound ----

var webhookPendingGauge atomic.Int64
var webhookDeadGauge atomic.Int64
var webhookDeadCounter atomic.Int64

func SetWebhookQueue(pending, dead int) {
	webhookPendingGauge.Store(int64(pending))
	webhookDeadGauge.Store(int64(dead))
}

func IncWebhookDead() { webhookDeadCounter.Add(1) }

// ---- Settlement / reconciliation ----

var reconciliationOpenGauge atomic.Int64

func SetReconciliationOpen(n int) { reconciliationOpenGauge.Store(int64(n)) }

// ---- Payout ----

var payoutsPendingGauge atomic.Int64

func SetPayoutsPending(n int) { payoutsPendingGauge.Store(int64(n)) }

// ---- Ledger ----

var ledgerEntriesTotal atomic.Int64

func IncLedgerEntries(n int) { ledgerEntriesTotal.Add(int64(n)) }

// Gather renders the metrics in Prometheus exposition format.
func Gather() string {
	var b strings.Builder
	// HELP / TYPE are optional but useful for Grafana.
	writeCounter(&b, "erapay_http_requests_total", "Total HTTP requests.", httpRequestsTotal)
	writeHistogram(&b, "erapay_http_duration_ms", "HTTP request latency in milliseconds.", httpDurationMs)
	b.WriteString(fmt.Sprintf("erapay_readyz_ok %d\n", boolToInt(readyzOK.Load())))

	writeCounter(&b, "erapay_gateway_requests_total", "Gateway requests by outcome.", gatewayRequestsTotal)
	writeHistogram(&b, "erapay_gateway_duration_ms", "Gateway round-trip in milliseconds.", gatewayDurationMs)

	b.WriteString(fmt.Sprintf("erapay_payments_pending_unknown %d\n", pendingUnknownGauge.Load()))
	b.WriteString(fmt.Sprintf("erapay_payments_stale_created %d\n", staleCreatedGauge.Load()))
	b.WriteString(fmt.Sprintf("erapay_webhook_pending %d\n", webhookPendingGauge.Load()))
	b.WriteString(fmt.Sprintf("erapay_webhook_dead %d\n", webhookDeadGauge.Load()))
	b.WriteString(fmt.Sprintf("erapay_webhook_dead_total %d\n", webhookDeadCounter.Load()))
	b.WriteString(fmt.Sprintf("erapay_reconciliation_exceptions_open %d\n", reconciliationOpenGauge.Load()))
	b.WriteString(fmt.Sprintf("erapay_payouts_pending %d\n", payoutsPendingGauge.Load()))
	b.WriteString(fmt.Sprintf("erapay_ledger_entries_total %d\n", ledgerEntriesTotal.Load()))
	return b.String()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- tiny metric primitives (no external deps) ----

type counterVec struct {
	mu   sync.Mutex
	keys map[string]int64
}

func newCounterVec() *counterVec { return &counterVec{keys: map[string]int64{}} }

func (c *counterVec) Inc(labels ...string) {
	k := strings.Join(labels, "\x1f")
	c.mu.Lock()
	c.keys[k]++
	c.mu.Unlock()
}

func (c *counterVec) Snapshot() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int64, len(c.keys))
	for k, v := range c.keys {
		out[k] = v
	}
	return out
}

type histogramVec struct {
	bounds []float64
	mu     sync.Mutex
	// key = label value (route or gateway). For each key, counts per bucket + sum + count.
	buckets map[string][]int64
	sums    map[string]float64
	counts  map[string]int64
}

func newHistogramVec(bounds []float64) *histogramVec {
	return &histogramVec{bounds: bounds, buckets: map[string][]int64{}, sums: map[string]float64{}, counts: map[string]int64{}}
}

func (h *histogramVec) Observe(label string, v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.buckets[label]; !ok {
		h.buckets[label] = make([]int64, len(h.bounds))
	}
	b := h.buckets[label]
	for i, bound := range h.bounds {
		if v <= bound {
			b[i]++
		}
	}
	h.sums[label] += v
	h.counts[label]++
}

func (h *histogramVec) Snapshot() (bounds []float64, buckets map[string][]int64, sums map[string]float64, counts map[string]int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	bc := make(map[string][]int64, len(h.buckets))
	for k, v := range h.buckets {
		cp := make([]int64, len(v))
		copy(cp, v)
		bc[k] = cp
	}
	sm := make(map[string]float64, len(h.sums))
	for k, v := range h.sums {
		sm[k] = v
	}
	cc := make(map[string]int64, len(h.counts))
	for k, v := range h.counts {
		cc[k] = v
	}
	b := make([]float64, len(h.bounds))
	copy(b, h.bounds)
	return b, bc, sm, cc
}

// ---- exposition helpers ----

func writeCounter(b *strings.Builder, name, help string, c *counterVec) {
	b.WriteString(fmt.Sprintf("# HELP %s %s\n# TYPE %s counter\n", name, help, name))
	snap := c.Snapshot()
	keys := sortedKeys(snap)
	for _, k := range keys {
		labels := parseLabels(k)
		b.WriteString(fmt.Sprintf("%s%s %d\n", name, formatLabels(labels), snap[k]))
	}
}

func writeHistogram(b *strings.Builder, name, help string, h *histogramVec) {
	b.WriteString(fmt.Sprintf("# HELP %s %s\n# TYPE %s histogram\n", name, help, name))
	bounds, buckets, sums, counts := h.Snapshot()
	labels := sortedStringKeys(buckets)
	for _, lbl := range labels {
		bs := buckets[lbl]
		for i, bound := range bounds {
			b.WriteString(fmt.Sprintf("%s_bucket{le=\"%g\",label=\"%s\"} %d\n", name, bound, lbl, bs[i]))
		}
		// +Inf bucket
		b.WriteString(fmt.Sprintf("%s_bucket{le=\"+Inf\",label=\"%s\"} %d\n", name, lbl, counts[lbl]))
		b.WriteString(fmt.Sprintf("%s_sum{label=\"%s\"} %g\n", name, lbl, sums[lbl]))
		b.WriteString(fmt.Sprintf("%s_count{label=\"%s\"} %d\n", name, lbl, counts[lbl]))
	}
}

func formatLabels(labels []string) string {
	if len(labels) == 0 || (len(labels) == 1 && labels[0] == "") {
		return ""
	}
	// Heuristic: 3 labels = method,route,code; otherwise gateway,operation,outcome
	if len(labels) == 3 {
		// Detect HTTP vs gateway by whether first label looks like an HTTP method.
		switch labels[0] {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
			return fmt.Sprintf("{method=\"%s\",route=\"%s\",code=\"%s\"}", labels[0], labels[1], labels[2])
		}
		return fmt.Sprintf("{gateway=\"%s\",operation=\"%s\",outcome=\"%s\"}", labels[0], labels[1], labels[2])
	}
	if len(labels) == 1 {
		return fmt.Sprintf("{label=\"%s\"}", labels[0])
	}
	return fmt.Sprintf("{label=\"%s\"}", strings.Join(labels, ","))
}

func parseLabels(k string) []string {
	if k == "" {
		return nil
	}
	return strings.Split(k, "\x1f")
}

func sortedKeys(m map[string]int64) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedStringKeys[M any](m map[string]M) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
