// Package config loads runtime configuration from the environment.
// Secrets come only from env vars (never from code, fixtures, or logs).
package config

import (
	"errors"
	"os"
)

type Config struct {
	Env         string // development | sandbox | production
	HTTPAddr    string
	DatabaseURL string
	RedisURL    string
}

func Load() (Config, error) {
	c := Config{
		Env:         get("ERAPAY_ENV", "development"),
		HTTPAddr:    get("ERAPAY_HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("ERAPAY_DATABASE_URL"),
		RedisURL:    os.Getenv("ERAPAY_REDIS_URL"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("ERAPAY_DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return c, errors.New("ERAPAY_REDIS_URL is required")
	}
	return c, nil
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
