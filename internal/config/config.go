package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the SMTP gateway service.
type Config struct {
	ResendAPIKey   string
	SMTPListerAddr string
	SendTimeout    time.Duration
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load reads configuration from environment variables and returns a Config struct.
// Returns an error if RESEND_API_KEY is not set or if timeout configuration is invalid.
func Load() (Config, error) {
	key := os.Getenv("RESEND_API_KEY")
	if key == "" {
		return Config{}, fmt.Errorf("RESEND_API_KEY is required")
	}
	addr := getenv("SMTP_LISTEN_ADDR", ":2525")
	timeoutStr := getenv("SEND_TIMEOUT_SECONDS", "15")
	tSec, err := strconv.Atoi(timeoutStr)
	if err != nil || tSec <= 0 {
		tSec = 15
	}
	return Config{
		ResendAPIKey:   key,
		SMTPListerAddr: addr,
		SendTimeout:    time.Duration(tSec) * time.Second,
	}, nil
}
