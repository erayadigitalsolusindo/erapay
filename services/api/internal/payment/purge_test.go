package payment

import (
	"context"
	"errors"
	"testing"
)

func TestPurgeOnlyPaymentsThatNeverMovedMoney(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	if err := e.svc.Purge(ctx, e.create(t, "A", 10_000).ID, "", "x", "r"); !errors.Is(err, ErrValidation) {
		t.Fatalf("short reason: %v", err)
	}

	// Never reached a gateway: deletable. The event log goes with it; the audit row stays.
	p := e.create(t, "B", 10_000)
	if err := e.svc.Purge(ctx, p.ID, "", "salah buat dari kasir", "r1"); err != nil {
		t.Fatal(err)
	}
	var n int
	e.db.QueryRow(ctx, `SELECT count(*) FROM payments WHERE id=$1`, p.ID).Scan(&n) //nolint:errcheck
	var a int
	e.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='payment.purge' AND entity_id=$1`, p.ID).Scan(&a) //nolint:errcheck
	if n != 0 || a != 1 {
		t.Fatalf("payments=%d audit=%d", n, a)
	}
	if err := e.svc.Purge(ctx, p.ID, "", "sudah hilang", "r2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second purge: %v", err)
	}

	// Reached a gateway and may still be paid: kept.
	q := e.create(t, "C", 10_000)
	if _, err := e.db.Exec(ctx, `UPDATE payments SET status='pending', gateway_reference='GW-1' WHERE id=$1`, q.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Purge(ctx, q.ID, "", "coba hapus", "r3"); !errors.Is(err, ErrNotPurgeable) {
		t.Fatalf("pending: %v", err)
	}

	// Outside Purge, the database still refuses deletes.
	if _, err := e.db.Exec(ctx, `DELETE FROM payments WHERE id=$1`, q.ID); err == nil {
		t.Fatal("direct delete must stay blocked")
	}
}
