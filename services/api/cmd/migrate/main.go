// Perintah migrasi database. Pemakaian:
//
//	go run ./cmd/migrate up        # terapkan semua migration
//	go run ./cmd/migrate status    # lihat status
//	go run ./cmd/migrate down      # batalkan 1 migration terakhir (lokal saja)
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/erayadigitalsolusindo/erapay/services/api/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("pakai: migrate up|down|status")
	}
	cmd := os.Args[1]
	url := os.Getenv("ERAPAY_DATABASE_URL")
	if url == "" {
		return fmt.Errorf("ERAPAY_DATABASE_URL wajib diisi")
	}
	// Migration destruktif tidak boleh jalan di produksi tanpa persetujuan (PRD §3).
	if cmd == "down" && os.Getenv("ERAPAY_ENV") == "production" {
		return fmt.Errorf("down dilarang di production")
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	switch cmd {
	case "up":
		return goose.Up(db, ".")
	case "down":
		return goose.Down(db, ".")
	case "status":
		return goose.Status(db, ".")
	}
	return fmt.Errorf("perintah tidak dikenal: %s", cmd)
}
