package migrate_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/migrate"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

func userObjects(t *testing.T, dsn string) int {
	t.Helper()
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	var n int
	err = p.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM information_schema.tables
		        WHERE table_schema='public' AND table_name <> 'goose_db_version')
		     + (SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
		        WHERE n.nspname='public'
		        AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid=p.oid AND d.deptype='e'))`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Setiap migrasi harus bisa up, down penuh (bersih tanpa sisa objek), lalu up lagi.
func TestUpDownUp(t *testing.T) {
	dsn := testdb.Create(t)
	ctx := context.Background()
	m, err := migrate.New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if err := m.Up(ctx); err != nil {
		t.Fatalf("up: %v", err)
	}
	if userObjects(t, dsn) == 0 {
		t.Fatal("no objects after up")
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("up is not idempotent: %v", err)
	}
	if err := m.Reset(ctx); err != nil {
		t.Fatalf("down: %v", err)
	}
	if n := userObjects(t, dsn); n != 0 {
		t.Fatalf("down left %d objects behind", n)
	}
	if err := m.Up(ctx); err != nil {
		t.Fatalf("re-up: %v", err)
	}
}
