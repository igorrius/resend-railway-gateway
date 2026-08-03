package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the SMTP gateway service.
type Config struct {
	// UnosendAPIKey is the Unosend API key used to authenticate outbound sends.
	UnosendAPIKey string
	// UnosendAPIURL is the endpoint used for outbound email requests.
	UnosendAPIURL string
	// SMTPListenAddr is the address the SMTP listener binds to (default ":2525").
	SMTPListenAddr string
	// HealthAddr is the address the HTTP health server binds to (default ":8080").
	// On Railway, the injected $PORT overrides this so the platform can healthcheck
	// the service over HTTP.
	HealthAddr string
	// SendTimeout is the maximum duration to wait for a single Unosend delivery
	// (including retries).
	SendTimeout time.Duration
	// MaxRetries is the maximum number of retries (in addition to the first
	// attempt) for transient Unosend failures (429 / 5xx / network errors).
	MaxRetries int

	// RequireAuth enforces SMTP AUTH before MAIL FROM is accepted. Enabled by
	// default so the relay is never left open. Disable only when the listener is
	// bound to a trusted private network.
	RequireAuth bool
	// SMTPUser and SMTPPass are the credentials accepted over SMTP AUTH PLAIN.
	SMTPUser string
	SMTPPass string

	// AllowedFromDomains, when non-empty, restricts the envelope MAIL FROM domain
	// to this allowlist. Domains are stored lower-cased.
	AllowedFromDomains []string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// normalizeAddr turns a bare port (e.g. "2525") into ":2525" while leaving
// host:port and ":port" forms untouched.
func normalizeAddr(addr string) string {
	if addr == "" {
		return addr
	}
	if addr[0] != ':' && !strings.Contains(addr, ":") {
		if _, err := strconv.Atoi(addr); err == nil {
			return ":" + addr
		}
	}
	return addr
}

// parseBool interprets common truthy/falsy strings, falling back to def.
func parseBool(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// parseDomains splits a comma-separated domain list into a normalized slice.
func parseDomains(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		p = strings.TrimPrefix(p, "@")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// HealthAddrFromEnv resolves the HTTP health address from HEALTH_ADDR / PORT
// without requiring the full configuration to be valid. Railway injects $PORT
// for the public HTTP port it healthchecks, so it takes precedence over
// HEALTH_ADDR when present. Used by both Load and the healthcheck subcommand.
func HealthAddrFromEnv() string {
	addr := normalizeAddr(getenv("HEALTH_ADDR", ":8080"))
	if v := strings.TrimSpace(os.Getenv("PORT")); v != "" {
		addr = normalizeAddr(v)
	}
	return addr
}

// Load reads configuration from environment variables and returns a Config struct.
//
// It fails fast (returns an error, never panics) when required configuration is
// missing or inconsistent:
//   - UNOSEND_API_KEY must be present and non-empty.
//   - When SMTP AUTH is required (the default), SMTP_USER and SMTP_PASS must be set.
func Load() (Config, error) {
	key := strings.TrimSpace(os.Getenv("UNOSEND_API_KEY"))
	if key == "" {
		return Config{}, fmt.Errorf("UNOSEND_API_KEY is required")
	}

	addr := normalizeAddr(getenv("SMTP_LISTEN_ADDR", ":2525"))

	healthAddr := HealthAddrFromEnv()

	timeoutStr := getenv("SEND_TIMEOUT_SECONDS", "15")
	tSec, err := strconv.Atoi(timeoutStr)
	if err != nil || tSec <= 0 {
		tSec = 15
	}

	maxRetries := 3
	if v := strings.TrimSpace(os.Getenv("SEND_MAX_RETRIES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxRetries = n
		}
	}

	requireAuth := parseBool(os.Getenv("SMTP_REQUIRE_AUTH"), true)
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	if requireAuth && (user == "" || pass == "") {
		return Config{}, fmt.Errorf(
			"SMTP AUTH is required by default: set SMTP_USER and SMTP_PASS, " +
				"or set SMTP_REQUIRE_AUTH=false only when binding to a trusted private network")
	}

	return Config{
		UnosendAPIKey:      key,
		UnosendAPIURL:      strings.TrimRight(getenv("UNOSEND_API_URL", "https://api.unosend.co/v1/emails"), "/"),
		SMTPListenAddr:     addr,
		HealthAddr:         healthAddr,
		SendTimeout:        time.Duration(tSec) * time.Second,
		MaxRetries:         maxRetries,
		RequireAuth:        requireAuth,
		SMTPUser:           user,
		SMTPPass:           pass,
		AllowedFromDomains: parseDomains(os.Getenv("ALLOWED_FROM_DOMAINS")),
	}, nil
}
