package auth

import (
	"context"
	"time"
)

// PlatformStats adalah ringkasan operasional untuk dashboard DEWA (hanya angka agregat, tanpa data pribadi).
type PlatformStats struct {
	GeneratedAt time.Time `json:"generated_at"`
	DBLatencyMS int64     `json:"db_latency_ms"`

	Users struct {
		Total    int `json:"total"`
		Active   int `json:"active"`
		Disabled int `json:"disabled"`
		Staff    int `json:"staff"` // punya peran platform
		MFA      int `json:"mfa_enabled"`
	} `json:"users"`
	Merchants map[string]int `json:"merchants_by_status"`
	RBAC      struct {
		Roles       int `json:"roles"`
		Permissions int `json:"permissions"`
		Grants      int `json:"grants"`
		Menus       int `json:"menus"`
	} `json:"rbac"`
	Sessions struct {
		Active         int `json:"active"`
		Platform       int `json:"platform"`
		Impersonation  int `json:"impersonation"`
		FailedLogins1h int `json:"failed_logins_1h"`
		FailedLogins24 int `json:"failed_logins_24h"`
	} `json:"sessions"`
	Payments struct {
		Total24h     int   `json:"total_24h"`
		Succeeded24h int   `json:"succeeded_24h"`
		Failed24h    int   `json:"failed_24h"`
		Pending      int   `json:"pending"`
		Volume24h    int64 `json:"volume_24h"` // integer minor units
	} `json:"payments"`
	Webhooks struct {
		Pending int `json:"pending"`
		Dead24h int `json:"dead_24h"`
	} `json:"webhooks"`
	Audit struct {
		Events24h int `json:"events_24h"`
		Series    []struct {
			Hour  time.Time `json:"hour"`
			Count int       `json:"count"`
		} `json:"series_24h"`
		Recent []struct {
			At     time.Time `json:"at"`
			Action string    `json:"action"`
			Entity string    `json:"entity_type"`
		} `json:"recent"`
	} `json:"audit"`
}

// PlatformStats mengumpulkan angka dashboard. Setiap kueri berdiri sendiri: kegagalan satu blok tidak
// menggugurkan blok lain (angka tetap 0), kecuali koneksi DB itu sendiri.
func (s *Service) PlatformStats(ctx context.Context) (PlatformStats, error) {
	var st PlatformStats
	st.GeneratedAt = s.now().UTC()
	t0 := time.Now()
	if err := s.db.Ping(ctx); err != nil {
		return st, err
	}
	st.DBLatencyMS = time.Since(t0).Milliseconds()

	_ = s.db.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE status = 'active'), count(*) FILTER (WHERE status = 'disabled'),
		       count(*) FILTER (WHERE EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = users.id)),
		       count(*) FILTER (WHERE mfa_enabled)
		FROM users`).Scan(&st.Users.Total, &st.Users.Active, &st.Users.Disabled, &st.Users.Staff, &st.Users.MFA)

	st.Merchants = map[string]int{}
	if rows, err := s.db.Query(ctx, `SELECT onboarding_status, count(*) FROM merchants GROUP BY 1`); err == nil {
		for rows.Next() {
			var k string
			var n int
			if rows.Scan(&k, &n) == nil {
				st.Merchants[k] = n
			}
		}
		rows.Close()
	}

	_ = s.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM roles), (SELECT count(*) FROM permissions),
		(SELECT count(*) FROM role_permissions), (SELECT count(*) FROM menu_items)`).
		Scan(&st.RBAC.Roles, &st.RBAC.Permissions, &st.RBAC.Grants, &st.RBAC.Menus)

	_ = s.db.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE merchant_id IS NULL), count(*) FILTER (WHERE impersonator_id IS NOT NULL)
		FROM sessions WHERE revoked_at IS NULL AND expires_at > now()`).
		Scan(&st.Sessions.Active, &st.Sessions.Platform, &st.Sessions.Impersonation)
	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE created_at > now() - interval '1 hour'), count(*)
		FROM auth_attempts WHERE NOT succeeded AND created_at > now() - interval '24 hours'`).
		Scan(&st.Sessions.FailedLogins1h, &st.Sessions.FailedLogins24)

	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE created_at > now() - interval '24 hours'),
		       count(*) FILTER (WHERE created_at > now() - interval '24 hours' AND status = 'succeeded'),
		       count(*) FILTER (WHERE created_at > now() - interval '24 hours' AND status IN ('failed','expired','cancelled')),
		       count(*) FILTER (WHERE status IN ('created','pending','unknown')),
		       COALESCE(sum(amount) FILTER (WHERE created_at > now() - interval '24 hours' AND status = 'succeeded'), 0)
		FROM payments`).
		Scan(&st.Payments.Total24h, &st.Payments.Succeeded24h, &st.Payments.Failed24h, &st.Payments.Pending, &st.Payments.Volume24h)

	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'dead' AND created_at > now() - interval '24 hours')
		FROM webhook_deliveries`).Scan(&st.Webhooks.Pending, &st.Webhooks.Dead24h)

	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE created_at > now() - interval '24 hours'`).Scan(&st.Audit.Events24h)
	if rows, err := s.db.Query(ctx, `
		SELECT h, count(a.id) FROM generate_series(date_trunc('hour', now()) - interval '23 hours', date_trunc('hour', now()), interval '1 hour') h
		LEFT JOIN audit_logs a ON date_trunc('hour', a.created_at) = h
		GROUP BY h ORDER BY h`); err == nil {
		for rows.Next() {
			var p struct {
				Hour  time.Time `json:"hour"`
				Count int       `json:"count"`
			}
			if rows.Scan(&p.Hour, &p.Count) == nil {
				st.Audit.Series = append(st.Audit.Series, p)
			}
		}
		rows.Close()
	}
	if rows, err := s.db.Query(ctx, `SELECT created_at, action, entity_type FROM audit_logs ORDER BY created_at DESC LIMIT 8`); err == nil {
		for rows.Next() {
			var p struct {
				At     time.Time `json:"at"`
				Action string    `json:"action"`
				Entity string    `json:"entity_type"`
			}
			if rows.Scan(&p.At, &p.Action, &p.Entity) == nil {
				st.Audit.Recent = append(st.Audit.Recent, p)
			}
		}
		rows.Close()
	}
	return st, nil
}
