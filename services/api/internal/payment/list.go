package payment

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ListFilter narrows a tenant's payments. All fields are optional; MerchantID is required and
// comes from the authenticated principal only.
type ListFilter struct {
	MerchantID string
	Status     Status
	Reference  string     // exact merchant_reference
	From       *time.Time // created_at >= From
	To         *time.Time // created_at < To
	Cursor     string
	Limit      int
}

const (
	defaultListLimit = 25
	maxListLimit     = 100
)

var validStatuses = map[Status]bool{Created: true, Pending: true, Succeeded: true, Failed: true, Expired: true, Cancelled: true, Unknown: true}

// ValidStatus reports whether s is a known payment status (used to validate query parameters).
func ValidStatus(s Status) bool { return validStatuses[s] }

// List returns one page, newest first, with keyset pagination on (created_at, id) so pages stay
// stable while new payments arrive. The QR payload is never part of a list (only the detail view).
func (s *Service) List(ctx context.Context, f ListFilter) ([]Payment, string, error) {
	if f.Limit < 1 || f.Limit > maxListLimit {
		f.Limit = defaultListLimit
	}
	if f.Status != "" && !ValidStatus(f.Status) {
		return nil, "", fmt.Errorf("%w: unknown status", ErrValidation)
	}
	var curAt *time.Time
	var curID *string // NULL when there is no cursor (an empty string would fail the uuid cast)
	if f.Cursor != "" {
		at, id, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("%w: invalid cursor", ErrValidation)
		}
		curAt, curID = &at, &id
	}
	rows, err := s.db.Query(ctx, `SELECT `+cols+` FROM payments
		WHERE merchant_id = $1
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR merchant_reference = $3)
		  AND ($4::timestamptz IS NULL OR created_at >= $4)
		  AND ($5::timestamptz IS NULL OR created_at < $5)
		  AND ($6::timestamptz IS NULL OR (created_at, id) < ($6, $7::uuid))
		ORDER BY created_at DESC, id DESC LIMIT $8`,
		f.MerchantID, string(f.Status), f.Reference, f.From, f.To, curAt, curID, f.Limit+1)
	if err != nil {
		return nil, "", err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Payment, error) { return scan(r) })
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > f.Limit {
		items = items[:f.Limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	for i := range items {
		items[i].QRCode = nil
	}
	return items, next, nil
}

func encodeCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMicro(), 10) + "|" + id))
}

func decodeCursor(c string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, "", err
	}
	us, id, ok := strings.Cut(string(b), "|")
	if !ok || !uuidRe.MatchString(id) {
		return time.Time{}, "", fmt.Errorf("bad cursor")
	}
	n, err := strconv.ParseInt(us, 10, 64)
	if err != nil {
		return time.Time{}, "", err
	}
	return time.UnixMicro(n).UTC(), id, nil
}

// ---- summary (Overview) ----

// ReportTimezone buckets days. PRD calls for the outlet's timezone; outlets have none yet, so the
// business timezone is fixed (WIB) and reported in the response.
const ReportTimezone = "Asia/Jakarta"

const maxSummaryDays = 366

type SummaryTotals struct {
	Payments    int64 `json:"payments"`
	Succeeded   int64 `json:"succeeded"`
	Pending     int64 `json:"pending"`
	Failed      int64 `json:"failed"`
	Expired     int64 `json:"expired"`
	Cancelled   int64 `json:"cancelled"`
	Unknown     int64 `json:"unknown"`
	Gross       int64 `json:"gross"` // succeeded payments only
	MDRFee      int64 `json:"mdr_fee"`
	PlatformFee int64 `json:"platform_fee"`
	MerchantNet int64 `json:"merchant_net"`
	Refunded    int64 `json:"refunded"` // refunds returned to customers in the period
}

type DailyPoint struct {
	Date  string `json:"date"` // YYYY-MM-DD in ReportTimezone
	Count int64  `json:"count"`
	Gross int64  `json:"gross"`
}

type Summary struct {
	From     string        `json:"from"` // inclusive, YYYY-MM-DD
	To       string        `json:"to"`   // inclusive, YYYY-MM-DD
	Timezone string        `json:"timezone"`
	Currency string        `json:"currency"`
	Totals   SummaryTotals `json:"totals"`
	Daily    []DailyPoint  `json:"daily"`
}

// Summarize aggregates one tenant over [from, to] (inclusive dates in ReportTimezone). Status counts
// and fee totals are by payment creation day; the daily series counts succeeded payments by the day they
// succeeded, so it shows when money actually came in. Every figure is derived from payment rows.
func (s *Service) Summarize(ctx context.Context, merchantID, currency string, from, to time.Time) (Summary, error) {
	loc, err := time.LoadLocation(ReportTimezone)
	if err != nil {
		return Summary{}, err
	}
	y1, m1, d1 := from.Date()
	y2, m2, d2 := to.Date()
	start := time.Date(y1, m1, d1, 0, 0, 0, 0, loc)
	end := time.Date(y2, m2, d2, 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	if !end.After(start) {
		return Summary{}, fmt.Errorf("%w: to must not be before from", ErrValidation)
	}
	if end.Sub(start) > maxSummaryDays*24*time.Hour {
		return Summary{}, fmt.Errorf("%w: period is limited to %d days", ErrValidation, maxSummaryDays)
	}
	out := Summary{From: start.Format("2006-01-02"), To: end.AddDate(0, 0, -1).Format("2006-01-02"),
		Timezone: ReportTimezone, Currency: currency, Daily: []DailyPoint{}}

	t := &out.Totals
	err = s.db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'succeeded'), count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'failed'), count(*) FILTER (WHERE status = 'expired'),
		       count(*) FILTER (WHERE status = 'cancelled'), count(*) FILTER (WHERE status IN ('unknown','created')),
		       COALESCE(SUM(amount) FILTER (WHERE status = 'succeeded'), 0),
		       COALESCE(SUM(mdr_fee) FILTER (WHERE status = 'succeeded'), 0),
		       COALESCE(SUM(platform_fee) FILTER (WHERE status = 'succeeded'), 0),
		       COALESCE(SUM(merchant_net) FILTER (WHERE status = 'succeeded'), 0)
		FROM payments WHERE merchant_id = $1 AND currency = $2 AND created_at >= $3 AND created_at < $4`,
		merchantID, currency, start, end).
		Scan(&t.Payments, &t.Succeeded, &t.Pending, &t.Failed, &t.Expired, &t.Cancelled, &t.Unknown,
			&t.Gross, &t.MDRFee, &t.PlatformFee, &t.MerchantNet)
	if err != nil {
		return Summary{}, err
	}
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(SUM(amount), 0) FROM refunds
		WHERE merchant_id = $1 AND currency = $2 AND status = 'refunded' AND refunded_at >= $3 AND refunded_at < $4`,
		merchantID, currency, start, end).Scan(&t.Refunded); err != nil {
		return Summary{}, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT to_char(d::date, 'YYYY-MM-DD'), COALESCE(x.n, 0), COALESCE(x.g, 0)
		FROM generate_series($3::date, $4::date, interval '1 day') AS d
		LEFT JOIN (
		    SELECT (final_at AT TIME ZONE $5)::date AS day, count(*) AS n, SUM(amount) AS g
		    FROM payments
		    WHERE merchant_id = $1 AND currency = $2 AND status = 'succeeded' AND final_at >= $6 AND final_at < $7
		    GROUP BY 1) x ON x.day = d::date
		ORDER BY d`,
		merchantID, currency, out.From, out.To, ReportTimezone, start, end)
	if err != nil {
		return Summary{}, err
	}
	out.Daily, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (DailyPoint, error) {
		var p DailyPoint
		return p, r.Scan(&p.Date, &p.Count, &p.Gross)
	})
	return out, err
}
