package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

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
