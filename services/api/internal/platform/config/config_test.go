package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	// Ensure no ambient env leaks into the default assertions.
	for _, k := range []string{"APP_ENV", "DATABASE_URL", "REDIS_URL", "API_PORT", "WORKER_PORT", "PAYMENT_PROVIDER"} {
		t.Setenv(k, "")
	}

	c := Load()
	if c.AppEnv != "local" {
		t.Errorf("AppEnv = %q, want %q", c.AppEnv, "local")
	}
	if c.APIPort != 8080 {
		t.Errorf("APIPort = %d, want 8080", c.APIPort)
	}
	if c.WorkerPort != 8081 {
		t.Errorf("WorkerPort = %d, want 8081", c.WorkerPort)
	}
	if c.PaymentProvider != "paystack" {
		t.Errorf("PaymentProvider = %q, want %q", c.PaymentProvider, "paystack")
	}
	if !c.IsLocal() {
		t.Error("IsLocal() = false, want true")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("API_PORT", "9090")
	t.Setenv("WORKER_TICK_INTERVAL", "30s")

	c := Load()
	if c.AppEnv != "production" {
		t.Errorf("AppEnv = %q, want %q", c.AppEnv, "production")
	}
	if c.IsLocal() {
		t.Error("IsLocal() = true, want false")
	}
	if c.APIPort != 9090 {
		t.Errorf("APIPort = %d, want 9090", c.APIPort)
	}
	if c.WorkerTickInterval.Seconds() != 30 {
		t.Errorf("WorkerTickInterval = %v, want 30s", c.WorkerTickInterval)
	}
}
