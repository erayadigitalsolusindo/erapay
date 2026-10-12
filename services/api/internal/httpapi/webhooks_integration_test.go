package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/webhook"
)

func TestWebhookCaptureStoresRawAndChangesNothing(t *testing.T) {
	db := testdb.New(t)
	mid, _ := testdb.SeedMerchant(t, db, "M")
	srv := httptest.NewServer(NewRouter(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: fakePinger{}, Redis: fakePinger{},
		Webhooks: webhook.NewStore(db),
	}))
	defer srv.Close()
	ctx := context.Background()

	post := func(path, body string, hdr map[string]string) int {
		req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(body))
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// Non-JSON content type and a nested path must both be accepted (gateway dictates the URL).
	raw := `{"originalPartnerReferenceNo":"x","amount":{"value":"1000.00"}}`
	if c := post("/webhooks/doku/v1.0/qr/qr-mpm-notify?a=1", raw, map[string]string{
		"Authorization": "Bearer SECRET-TOKEN", "X-EXTERNAL-ID": "ext-1", "Content-Type": "text/plain"}); c != 200 {
		t.Fatalf("status %d", c)
	}

	var gw, path, query string
	var body []byte
	var hdr string
	var present bool
	var alen int
	var asum, bsum string
	if err := db.QueryRow(ctx, `SELECT gateway, path, query, body, headers::text, auth_present, auth_len, auth_sha256, body_sha256
		FROM webhook_captures`).Scan(&gw, &path, &query, &body, &hdr, &present, &alen, &asum, &bsum); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte("Bearer SECRET-TOKEN"))
	if gw != "doku" || path != "/webhooks/doku/v1.0/qr/qr-mpm-notify" || query != "a=1" || string(body) != raw ||
		!present || alen != len("Bearer SECRET-TOKEN") || asum != hex.EncodeToString(h[:]) {
		t.Fatalf("capture mismatch: %s %s %s %q", gw, path, query, body)
	}
	if strings.Contains(hdr, "SECRET-TOKEN") || !strings.Contains(hdr, "ext-1") {
		t.Fatalf("authorization must be dropped, other headers kept: %s", hdr)
	}

	// Capture-only: no payment/ledger side effects exist or are created.
	var n int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM payments)+(SELECT count(*) FROM payment_events)
		+(SELECT count(*) FROM ledger_entries) WHERE $1::uuid IS NOT NULL`, mid).Scan(&n); err != nil || n != 0 {
		t.Fatalf("side effects: n=%d err=%v", n, err)
	}

	// Rows are append-only.
	if _, err := db.Exec(ctx, `UPDATE webhook_captures SET gateway = 'x'`); err == nil {
		t.Fatal("update must be forbidden")
	}

	// Oversized body is truncated, and concurrent duplicates are all kept (no dedupe in capture).
	big := strings.Repeat("a", webhook.MaxBody+500)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); post("/webhooks/doku", big, nil) }()
	}
	wg.Wait()
	var rows, trunc, maxLen int
	if err := db.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE body_truncated), max(length(body))
		FROM webhook_captures WHERE path = '/webhooks/doku'`).Scan(&rows, &trunc, &maxLen); err != nil {
		t.Fatal(err)
	}
	if rows != 8 || trunc != 8 || maxLen != webhook.MaxBody {
		t.Fatalf("rows=%d trunc=%d maxLen=%d", rows, trunc, maxLen)
	}
}

func TestWebhookRoutesDisabledByDefault(t *testing.T) {
	srv := httptest.NewServer(NewRouter(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: fakePinger{}, Redis: fakePinger{}}))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/webhooks/doku", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("want 404 when capture disabled, got %d", resp.StatusCode)
	}
}
