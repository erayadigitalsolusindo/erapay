package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

var (
	// ErrMerchantHasFinancial: tenant sudah punya transaksi/payout/refund/ledger (append-only); tidak boleh dihapus.
	ErrMerchantHasFinancial = errors.New("merchant has financial records")
)

// DeleteMerchant menghapus tenant beserta SEMUA datanya (gateway, API key, webhook, KYC, sesi, dst.)
// dan user pemiliknya yang tidak terhubung ke tenant lain, dalam satu transaksi. Dirancang untuk tenant
// spam/draf: ditolak bila ada jejak keuangan. Anak tabel ditemukan dari katalog FK Postgres.
func (s *Service) DeleteMerchant(ctx context.Context, merchantID, actor, requestID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var name string
	if err := tx.QueryRow(ctx, `SELECT display_name FROM merchants WHERE id = $1 FOR UPDATE`, merchantID).Scan(&name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMerchantNotFound
		}
		return err
	}
	// The sandbox twin (if any) goes with its merchant, but only while it too has no financial trace
	// (the ledger is append-only, so a twin with sandbox journals can never be removed).
	var twin string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM merchants WHERE parent_id = $1 FOR UPDATE`, merchantID).Scan(&twin); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	for _, id := range []string{merchantID, twin} {
		if id == "" {
			continue
		}
		var fin bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM payments WHERE merchant_id = $1)
			    OR EXISTS (SELECT 1 FROM payouts WHERE merchant_id = $1)
			    OR EXISTS (SELECT 1 FROM refunds WHERE merchant_id = $1)
			    OR EXISTS (SELECT 1 FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id WHERE a.merchant_id = $1)`,
			id).Scan(&fin); err != nil {
			return err
		}
		if fin {
			return ErrMerchantHasFinancial
		}
	}
	if twin != "" {
		if err := cascadeDelete(ctx, tx, "merchants", "id = $1", twin, 0); err != nil {
			return err
		}
	}

	// User yang hanya terhubung ke tenant ini (bukan staf platform) ikut dihapus.
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT u FROM (
		  SELECT user_id AS u FROM merchant_users WHERE merchant_id = $1
		  UNION SELECT owner_user_id FROM merchant_external_links WHERE merchant_id = $1) x
		WHERE NOT EXISTS (SELECT 1 FROM merchant_users mu WHERE mu.user_id = u AND mu.merchant_id <> $1)
		  AND NOT EXISTS (SELECT 1 FROM merchant_external_links l WHERE l.owner_user_id = u AND l.merchant_id <> $1)
		  AND NOT EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = u)`, merchantID)
	if err != nil {
		return err
	}
	var users []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			rows.Close()
			return err
		}
		users = append(users, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if err := cascadeDelete(ctx, tx, "merchants", "id = $1", merchantID, 0); err != nil {
		return err
	}
	removed := 0
	for _, u := range users {
		if _, err := deleteUserTx(ctx, tx, u, actor, requestID); err != nil {
			if errors.Is(err, ErrUserHasFinancial) || errors.Is(err, ErrUserProtected) {
				continue // akun dipertahankan; sudah tak terhubung ke tenant mana pun
			}
			return err
		}
		removed++
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actor, Action: "platform.merchant.deleted", EntityType: "merchant",
		EntityID: merchantID, RequestID: requestID, Metadata: map[string]any{"name": name, "users_deleted": removed}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// cascadeDelete menghapus baris tbl yang cocok dengan where, setelah semua baris anak (FK satu kolom)
// dihapus rekursif. FK ke diri sendiri diurus oleh DELETE itu sendiri.
func cascadeDelete(ctx context.Context, tx pgx.Tx, tbl, where, arg string, depth int) error {
	if depth > 8 {
		return fmt.Errorf("cascade too deep at %s", tbl)
	}
	rows, err := tx.Query(ctx, `
		SELECT c.conrelid::regclass::text, a.attname, fa.attname
		FROM pg_constraint c
		JOIN pg_attribute a  ON a.attrelid = c.conrelid  AND a.attnum = c.conkey[1]
		JOIN pg_attribute fa ON fa.attrelid = c.confrelid AND fa.attnum = c.confkey[1]
		WHERE c.contype = 'f' AND c.confrelid = $1::regclass AND cardinality(c.conkey) = 1 AND c.conrelid <> c.confrelid`, tbl)
	if err != nil {
		return err
	}
	type fk struct{ child, col, ref string }
	var fks []fk
	for rows.Next() {
		var f fk
		if err := rows.Scan(&f.child, &f.col, &f.ref); err != nil {
			rows.Close()
			return err
		}
		fks = append(fks, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	// Nama berasal dari katalog Postgres (bukan input user); identifier di-quote.
	qt := pgx.Identifier{tbl}.Sanitize()
	for _, f := range fks {
		sub := fmt.Sprintf(`%s IN (SELECT %s FROM %s WHERE %s)`, pgx.Identifier{f.col}.Sanitize(), pgx.Identifier{f.ref}.Sanitize(), qt, where)
		if err := cascadeDelete(ctx, tx, f.child, sub, arg, depth+1); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s`, qt, where), arg); err != nil {
		return fmt.Errorf("delete %s: %w", tbl, err)
	}
	return nil
}
