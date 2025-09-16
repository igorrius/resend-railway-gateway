package main

import (
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	resendclient "github.com/igorrius/resend-railway-gateway/internal/adapters/resend"
	smtpserver "github.com/igorrius/resend-railway-gateway/internal/adapters/smtp"
	"github.com/igorrius/resend-railway-gateway/internal/app"
	"github.com/igorrius/resend-railway-gateway/internal/config"
	"github.com/igorrius/resend-railway-gateway/internal/logging"
)

func main() {
	// create root slog logger using environment-based configuration
	root := logging.NewConfiguredLogger()
	slog.SetDefault(root)

	cfg, err := config.Load()
	if err != nil {
		root.Error("config_load_failed", "error", err)
		os.Exit(1)
	}
	// optional override for Railway dynamic ports
	if v := os.Getenv("PORT"); v != "" {
		cfg.SMTPListerAddr = ":" + v
	}

	sender := resendclient.NewClient(cfg.ResendAPIKey)
	svc := app.NewService(sender, logging.New(root), cfg.SendTimeout)
	server := smtpserver.NewServer(cfg.SMTPListerAddr, svc)

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	errCh := make(chan error, 1)

	// Start server in a separate goroutine
	go func() {
		root.Info("smtp_listen", "addr", cfg.SMTPListerAddr)
		errCh <- server.ListenAndServe()
	}()

	var exitCode int
	select {
	case sig := <-sigCh:
		root.Info("shutdown_signal_received", "signal", sig.String())
		// Attempt graceful shutdown by closing the server listener
		if cerr := server.Close(); cerr != nil {
			root.Error("smtp_server_close_error", "error", cerr)
		}
		// Wait for ListenAndServe to return; treat closing of the listener as normal
		err = <-errCh
		if err != nil && !errors.Is(err, os.ErrClosed) {
			// go-smtp may return specific errors on close; log but exit 0 since shutdown was requested
			root.Info("smtp_server_stopped", "error", err)
		}
		exitCode = 0
	case err = <-errCh:
		if err != nil {
			root.Error("smtp_server_error", "error", err)
			exitCode = 1
		} else {
			exitCode = 0
		}
	}

	os.Exit(exitCode)
}
