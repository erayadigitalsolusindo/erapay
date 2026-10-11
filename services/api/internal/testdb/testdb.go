// Package testdb gives integration tests an isolated PostgreSQL database.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/migrate"
)

const envURL = "ERAPAY_TEST_DATABASE_URL"

// Create makes an empty, uniquely named database on the server named by
// ERAPAY_TEST_DATABASE_URL (the role needs CREATEDB) and returns its DSN. The
// database is dropped on cleanup. The test is skipped when the variable is unset.
func Create(t testing.TB) string {
	t.Helper()
	admin := os.Getenv(envURL)
	if admin == "" {
		t.Skipf("%s not set; skipping PostgreSQL integration test", envURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ap, err := pgxpool.New(ctx, admin)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "erapay_test_" + hex.EncodeToString(b)
	if _, err := ap.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		ap.Close()
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = ap.Exec(c, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		ap.Close()
	})

	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("parse %s: %v", envURL, err)
	}
	u.Path = "/" + name
	return u.String()
}

// New is Create plus all migrations applied, returning a connected pool.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := Create(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m, err := migrate.New(dsn)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer m.Close()
	if err := m.Up(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
