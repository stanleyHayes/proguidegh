// Package config loads environment-based configuration (spec §25).
// Defaults are safe for local development; secrets have no defaults.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the runtime configuration for the API and worker.
type Config struct {
	// AppEnv is one of local | staging | production.
	AppEnv string

	// DatabaseURL is the PostgreSQL connection string (system of record).
	DatabaseURL string
	// RedisURL is the Redis connection string (ephemeral state only).
	RedisURL string

	// JWTOrSessionSecret signs sessions/tokens. Must be set per environment.
	JWTOrSessionSecret string

	// APIPort is the HTTP port for the API server.
	APIPort int
	// WorkerPort is the HTTP port for the worker health endpoint.
	WorkerPort int
	// WorkerTickInterval is how often the worker job loop ticks.
	WorkerTickInterval time.Duration

	// PaymentProvider selects the payment adapter (paystack | mock).
	PaymentProvider string
	// PaystackSecretKey activates the real Paystack adapter when set
	// (PAYMENT_PROVIDER=paystack). Empty means the deterministic mock
	// adapter is used (spec §31.29: build against mocks when blocked on
	// credentials).
	PaystackSecretKey string
	// PaystackWebhookSecret verifies Paystack webhook signatures; falls back
	// to the secret key when unset (Paystack's default signing behaviour).
	PaystackWebhookSecret string
	// MockWebhookSecret signs mock-provider webhooks. The default is a
	// documented dev-only value; set it per environment like any secret.
	MockWebhookSecret string

	// StorageProvider selects the object-storage adapter (local | r2).
	StorageProvider string
	// UploadsDir is the local adapter's root directory (dev only).
	UploadsDir string
	// R2* configure the Cloudflare R2 adapter (production target).
	R2Endpoint        string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketPrivate   string

	// PayoutAccountKey encrypts payout-account destination references at
	// rest (AES-256-GCM). When empty, the key is derived from
	// JWTOrSessionSecret — acceptable for local dev; set it per environment
	// like any secret.
	PayoutAccountKey string

	// CORSAllowedOrigins lists the web-app origins permitted to make
	// credentialed cross-origin calls (comma-separated). Defaults cover the
	// local Next.js dev servers; set the production domains per environment.
	CORSAllowedOrigins []string

	// SentryDSN enables error reporting when set.
	SentryDSN string
}

// Load reads configuration from the environment, applying local defaults.
func Load() Config {
	return Config{
		AppEnv:             get("APP_ENV", "local"),
		DatabaseURL:        get("DATABASE_URL", "postgres://proguidegh:proguidegh@localhost:5432/proguidegh?sslmode=disable"),
		RedisURL:           get("REDIS_URL", "redis://localhost:6379"),
		JWTOrSessionSecret: get("JWT_OR_SESSION_SECRET", ""),
		// Render injects PORT (default 10000) and health-checks that port; it
		// does not know about API_PORT. Precedence is explicit override, then
		// the platform-injected PORT, then the local default — so a blueprint
		// deploy binds correctly with no extra configuration, and local dev is
		// unchanged.
		APIPort:               getInt("API_PORT", getInt("PORT", 8080)),
		WorkerPort:            getInt("WORKER_PORT", getInt("PORT", 8081)),
		WorkerTickInterval:    getDuration("WORKER_TICK_INTERVAL", 10*time.Second),
		PaymentProvider:       get("PAYMENT_PROVIDER", "paystack"),
		PaystackSecretKey:     get("PAYSTACK_SECRET_KEY", ""),
		PaystackWebhookSecret: get("PAYSTACK_WEBHOOK_SECRET", ""),
		MockWebhookSecret:     get("MOCK_WEBHOOK_SECRET", "dev-mock-webhook-secret"),
		StorageProvider:       get("STORAGE_PROVIDER", "local"),
		UploadsDir:            get("STORAGE_LOCAL_DIR", "./data/uploads"),
		R2Endpoint:            get("R2_ENDPOINT", ""),
		R2AccessKeyID:         get("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:     get("R2_SECRET_ACCESS_KEY", ""),
		R2BucketPrivate:       get("R2_BUCKET_PRIVATE", ""),
		PayoutAccountKey:      get("PAYOUT_ACCOUNT_KEY", ""),
		CORSAllowedOrigins:    getList("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:3001,http://localhost:3002"),
		SentryDSN:             get("SENTRY_DSN", ""),
	}
}

// IsLocal reports whether the app runs in the local environment.
func (c Config) IsLocal() bool { return c.AppEnv == "local" }

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

func getList(key, def string) []string {
	raw := get(key, def)
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func getDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
