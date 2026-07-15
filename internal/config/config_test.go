package config

import (
	"testing"
	"time"
)

// clearEnv resets every env var Load reads so each test starts from a known state.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"RESEND_API_KEY", "SMTP_LISTEN_ADDR", "HEALTH_ADDR", "PORT",
		"SEND_TIMEOUT_SECONDS", "SEND_MAX_RETRIES", "SMTP_REQUIRE_AUTH",
		"SMTP_USER", "SMTP_PASS", "ALLOWED_FROM_DOMAINS",
	} {
		t.Setenv(k, "")
	}
}

func TestLoad_MissingAPIKey(t *testing.T) {
	clearEnv(t)
	if _, err := Load(); err == nil {
		t.Fatal("expected error when RESEND_API_KEY is missing")
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("RESEND_API_KEY", "re_test")
	// AUTH is required by default, so credentials are needed for a clean load.
	t.Setenv("SMTP_USER", "user")
	t.Setenv("SMTP_PASS", "pass")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SMTPListenAddr != ":2525" {
		t.Errorf("SMTPListenAddr default = %q, want :2525", cfg.SMTPListenAddr)
	}
	if cfg.HealthAddr != ":8080" {
		t.Errorf("HealthAddr default = %q, want :8080", cfg.HealthAddr)
	}
	if cfg.SendTimeout != 15*time.Second {
		t.Errorf("SendTimeout default = %v, want 15s", cfg.SendTimeout)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries default = %d, want 3", cfg.MaxRetries)
	}
	if !cfg.RequireAuth {
		t.Error("RequireAuth default should be true")
	}
}

func TestLoad_RequireAuthWithoutCreds(t *testing.T) {
	clearEnv(t)
	t.Setenv("RESEND_API_KEY", "re_test")
	// No SMTP_USER / SMTP_PASS -> must fail fast.
	if _, err := Load(); err == nil {
		t.Fatal("expected error when AUTH required but credentials missing")
	}
}

func TestLoad_AuthDisabledAllowsNoCreds(t *testing.T) {
	clearEnv(t)
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("SMTP_REQUIRE_AUTH", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RequireAuth {
		t.Error("RequireAuth should be false when SMTP_REQUIRE_AUTH=false")
	}
}

func TestLoad_PortOverridesHealthAddr(t *testing.T) {
	clearEnv(t)
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("SMTP_REQUIRE_AUTH", "false")
	t.Setenv("HEALTH_ADDR", ":8080")
	t.Setenv("PORT", "9000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HealthAddr != ":9000" {
		t.Errorf("HealthAddr = %q, want :9000 (PORT should override)", cfg.HealthAddr)
	}
	// SMTP listener must be untouched by PORT.
	if cfg.SMTPListenAddr != ":2525" {
		t.Errorf("SMTPListenAddr = %q, want :2525", cfg.SMTPListenAddr)
	}
}

func TestLoad_BarePortNormalized(t *testing.T) {
	clearEnv(t)
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("SMTP_REQUIRE_AUTH", "false")
	t.Setenv("SMTP_LISTEN_ADDR", "2626")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SMTPListenAddr != ":2626" {
		t.Errorf("SMTPListenAddr = %q, want :2626", cfg.SMTPListenAddr)
	}
}

func TestLoad_AllowedFromDomains(t *testing.T) {
	clearEnv(t)
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("SMTP_REQUIRE_AUTH", "false")
	t.Setenv("ALLOWED_FROM_DOMAINS", " Example.com, @foo.org ,,bar.net")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"example.com", "foo.org", "bar.net"}
	if len(cfg.AllowedFromDomains) != len(want) {
		t.Fatalf("AllowedFromDomains = %v, want %v", cfg.AllowedFromDomains, want)
	}
	for i, d := range want {
		if cfg.AllowedFromDomains[i] != d {
			t.Errorf("AllowedFromDomains[%d] = %q, want %q", i, cfg.AllowedFromDomains[i], d)
		}
	}
}

func TestLoad_MaxRetriesParsing(t *testing.T) {
	clearEnv(t)
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("SMTP_REQUIRE_AUTH", "false")
	t.Setenv("SEND_MAX_RETRIES", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}
}
