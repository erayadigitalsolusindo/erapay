package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

var (
	ErrUserNotFound = errors.New("user not found")
	// ErrUserProtected: tidak boleh menghapus diri sendiri atau pemegang rbac:manage terakhir.
	ErrUserProtected = errors.New("user is protected")
	// ErrUserHasFinancial: user punya jejak keuangan (payout/refund) atau memiliki tenant; dihapus lewat jalur lain.
	ErrUserHasFinancial = errors.New("user has financial records")
)

// Tabel yang barisnya tidak boleh ikut terhapus (jejak keuangan / kepemilikan tenant).
// Jika user masih direferensikan di sini, hapus ditolak (nonaktifkan user sebagai gantinya).
var userDeleteBlockers = map[string]bool{
	"payouts":                 true,
	"refunds":                 true,
	"merchant_external_links": true,
	"pricing_plans":           true, // append-only (trigger)
	"settlement_batches":      true, // append-only (trigger)
}

// DeleteUser menghapus user beserta seluruh relasinya dalam satu transaksi.
// Kolom FK ke users yang NOT NULL (sesi, peran, token) dihapus barisnya; yang nullable
// (created_by, approved_by, dst.) di-NULL-kan agar riwayat tetap ada. audit_logs append-only
// dan tidak punya FK, jadi tetap utuh. Mengembalikan jumlah baris per tabel yang tersentuh.
func (s *Service) DeleteUser(ctx context.Context, userID, actor, requestID string) (map[string]int64, error) {
	if userID == actor {
		return nil, fmt.Errorf("%w: cannot delete yourself", ErrUserProtected)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var email string
	if err := tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	// Jangan sisakan platform tanpa pemegang rbac:manage (akan terkunci dari panel).
	var holdsManage bool
	var holders int
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM user_roles ur JOIN role_permissions rp ON rp.role_id = ur.role_id
		               WHERE ur.user_id = $1 AND rp.permission_code = 'rbac:manage'),
		       (SELECT count(DISTINCT ur.user_id) FROM user_roles ur JOIN role_permissions rp ON rp.role_id = ur.role_id
		        WHERE rp.permission_code = 'rbac:manage')`, userID).Scan(&holdsManage, &holders); err != nil {
		return nil, err
	}
	if holdsManage && holders <= 1 {
		return nil, fmt.Errorf("%w: last rbac:manage holder", ErrUserProtected)
	}

	rows, err := tx.Query(ctx, `
		SELECT c.conrelid::regclass::text, a.attname, a.attnotnull
		FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		WHERE c.contype = 'f' AND c.confrelid = 'users'::regclass AND cardinality(c.conkey) = 1
		ORDER BY 1, 2`)
	if err != nil {
		return nil, err
	}
	type fk struct {
		tbl, col string
		notNull  bool
	}
	var fks []fk
	for rows.Next() {
		var f fk
		if err := rows.Scan(&f.tbl, &f.col, &f.notNull); err != nil {
			rows.Close()
			return nil, err
		}
		fks = append(fks, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	touched := map[string]int64{}
	for _, f := range fks {
		// Nama tabel/kolom berasal dari katalog Postgres (bukan input user); identifier di-quote.
		tbl, col := pgx.Identifier{f.tbl}.Sanitize(), pgx.Identifier{f.col}.Sanitize()
		if userDeleteBlockers[f.tbl] {
			var n int64
			if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s = $1`, tbl, col), userID).Scan(&n); err != nil {
				return nil, err
			}
			if n > 0 {
				return nil, fmt.Errorf("%w: %s", ErrUserHasFinancial, f.tbl)
			}
			continue
		}
		var n int64
		if f.notNull {
			t, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s = $1`, tbl, col), userID)
			if err != nil {
				return nil, fmt.Errorf("delete %s: %w", f.tbl, err)
			}
			n = t.RowsAffected()
		} else {
			t, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s = NULL WHERE %s = $1`, tbl, col, col), userID)
			if err != nil {
				return nil, fmt.Errorf("nullify %s: %w", f.tbl, err)
			}
			n = t.RowsAffected()
		}
		if n > 0 {
			touched[f.tbl+"."+f.col] += n
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		return nil, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actor, Action: "platform.user.deleted", EntityType: "user",
		EntityID: userID, RequestID: requestID, Metadata: map[string]any{"email": email, "relations": touched}}); err != nil {
		return nil, err
	}
	return touched, tx.Commit(ctx)
}
