package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	resendclient "github.com/igorrius/resend-railway-gateway/internal/adapters/resend"
	smtpserver "github.com/igorrius/resend-railway-gateway/internal/adapters/smtp"
	"github.com/igorrius/resend-railway-gateway/internal/app"
	"github.com/igorrius/resend-railway-gateway/internal/config"
)

type stdLogger struct{}

func (l stdLogger) Info(msg string, fields map[string]any) {
	attrs := make([]slog.Attr, 0, len(fields))
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}
func (l stdLogger) Error(msg string, fields map[string]any) {
	attrs := make([]slog.Attr, 0, len(fields))
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	slog.Default().LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config_load_failed", "error", err)
		os.Exit(1)
	}
	// optional override for Railway dynamic ports
	if v := os.Getenv("PORT"); v != "" {
		cfg.SMTPListerAddr = ":" + v
	}

	sender := resendclient.NewClient(cfg.ResendAPIKey)
	svc := app.NewService(sender, stdLogger{}, cfg.SendTimeout)
	server := smtpserver.NewServer(cfg.SMTPListerAddr, svc)
	slog.Info("smtp_listen", "addr", cfg.SMTPListerAddr)
	if err := server.ListenAndServe(); err != nil {
		slog.Error("smtp_server_error", "error", err)
		os.Exit(1)
	}
	_ = time.Now()
}
