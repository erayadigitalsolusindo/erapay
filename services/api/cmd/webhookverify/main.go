// Command webhookverify checks a CAPTURED DOKU notification against the signature formula,
// using DOKU_CLIENT_ID / DOKU_SECRET_KEY from the environment. Ops/diagnostic tool: it prints
// only MATCH/MISMATCH and never any secret, signature or payload.
//
//	webhookverify <webhook_captures.id>
//
// If the formula in internal/gateway/doku/webhook.go were wrong, every real notification
// would be answered 400; run this once on the server against a captured sandbox payment.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/doku"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	url, cid, secret := os.Getenv("ERAPAY_DATABASE_URL"), os.Getenv("DOKU_CLIENT_ID"), os.Getenv("DOKU_SECRET_KEY")
	if len(args) != 1 || url == "" || cid == "" || secret == "" {
		return fmt.Errorf("usage: webhookverify <capture-id>  (needs ERAPAY_DATABASE_URL, DOKU_CLIENT_ID, DOKU_SECRET_KEY)")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return err
	}
	defer db.Close()

	var path string
	var hdrJSON []byte
	var body []byte
	if err := db.QueryRow(ctx, `SELECT path, headers, body FROM webhook_captures WHERE id = $1`, args[0]).
		Scan(&path, &hdrJSON, &body); err != nil {
		return fmt.Errorf("capture not found: %w", err)
	}
	hdr := map[string][]string{}
	if err := json.Unmarshal(hdrJSON, &hdr); err != nil {
		return err
	}
	get := func(k string) string {
		for name, v := range hdr {
			if strings.EqualFold(name, k) && len(v) > 0 {
				return v[0]
			}
		}
		return ""
	}
	rid, rts, got := get("Request-Id"), get("Request-Timestamp"), get("Signature")
	fmt.Printf("client-id header matches DOKU_CLIENT_ID: %v\n", get("Client-Id") == cid)

	// The documented formula first, then plausible variants to explain a mismatch.
	d := sha256.Sum256(body)
	digest := base64.StdEncoding.EncodeToString(d[:])
	hm := func(s string) string {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte(s))
		return "HMACSHA256=" + base64.StdEncoding.EncodeToString(m.Sum(nil))
	}
	comp := func(sep, target string) string {
		return strings.Join([]string{"Client-Id:" + get("Client-Id"), "Request-Id:" + rid,
			"Request-Timestamp:" + rts, "Request-Target:" + target, "Digest:" + digest}, sep)
	}
	if doku.WebhookSignature(secret, get("Client-Id"), rid, rts, path, body) == got {
		fmt.Println("RESULT: MATCH (documented formula, Request-Target = captured path)")
		return nil
	}
	for name, s := range map[string]string{
		"separator \\n + trailing newline":      comp("\n", path) + "\n",
		"literal backslash-n separator":         comp(`\n`, path),
		"Request-Target without /webhooks/doku": comp("\n", strings.TrimPrefix(path, "/webhooks/doku")),
	} {
		if hm(s) == got {
			fmt.Printf("RESULT: MATCH only with variant: %s -> update internal/gateway/doku/webhook.go\n", name)
			return nil
		}
	}
	fmt.Println("RESULT: MISMATCH (check DOKU_SECRET_KEY is the sandbox secret and the capture is a real DOKU notification)")
	os.Exit(2)
	return nil
}
