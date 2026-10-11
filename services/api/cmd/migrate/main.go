// Command migrate applies or rolls back database migrations.
//
//	migrate up | down | status | reset
//
// reset rolls back everything and is refused when ERAPAY_ENV is sandbox/production.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/migrate"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: migrate up|down|status|reset")
	}
	url := os.Getenv("ERAPAY_DATABASE_URL")
	if url == "" {
		return fmt.Errorf("ERAPAY_DATABASE_URL is required")
	}
	r, err := migrate.New(url)
	if err != nil {
		return err
	}
	defer r.Close()
	ctx := context.Background()
	switch args[0] {
	case "up":
		return r.Up(ctx)
	case "down":
		return r.Down(ctx)
	case "reset":
		if env := os.Getenv("ERAPAY_ENV"); env != "" && env != "development" {
			return fmt.Errorf("reset refused when ERAPAY_ENV=%s", env)
		}
		return r.Reset(ctx)
	case "status":
		lines, err := r.Status(ctx)
		for _, l := range lines {
			fmt.Println(l)
		}
		return err
	}
	return fmt.Errorf("unknown command %q", args[0])
}
