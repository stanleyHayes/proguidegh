// Package config is the worker's minimal environment-based configuration.
// It mirrors the API's platform config (spec §25) without importing across
// modules; defaults are safe for local development.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds the runtime configuration for the worker.
type Config struct {
	// AppEnv is one of local | staging | production.
	AppEnv string
	// DatabaseURL is the PostgreSQL connection string (system of record).
	DatabaseURL string
	// RedisURL is the Redis connection string (queues/locks).
	RedisURL string
	// WorkerPort is the HTTP port for the health endpoint.
	WorkerPort int
	// TickInterval is how often the job-runner loop ticks.
	TickInterval time.Duration
}

// Load reads configuration from the environment, applying local defaults.
func Load() Config {
	return Config{
		AppEnv:       get("APP_ENV", "local"),
		DatabaseURL:  get("DATABASE_URL", "postgres://proguidegh:proguidegh@localhost:5432/proguidegh?sslmode=disable"),
		RedisURL:     get("REDIS_URL", "redis://localhost:6379"),
		WorkerPort:   getInt("WORKER_PORT", 8081),
		TickInterval: getDuration("WORKER_TICK_INTERVAL", 10*time.Second),
	}
}

func get(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
