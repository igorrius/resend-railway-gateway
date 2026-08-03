package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/igorrius/unosend-railway-gateway/internal/adapters/health"
	smtpserver "github.com/igorrius/unosend-railway-gateway/internal/adapters/smtp"
	unosendclient "github.com/igorrius/unosend-railway-gateway/internal/adapters/unosend"
	"github.com/igorrius/unosend-railway-gateway/internal/app"
	"github.com/igorrius/unosend-railway-gateway/internal/config"
	"github.com/igorrius/unosend-railway-gateway/internal/logging"
)

func main() {
	os.Exit(run())
}

func run() int {
	root := logging.NewConfiguredLogger()
	slog.SetDefault(root)

	// Smoke-test subcommand: probe the local /health endpoint and exit 0/1.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck", "--check", "-check":
			return runHealthcheck(root)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		// Fail fast with a clear message and non-zero exit (no panic).
		root.Error("config_load_failed", "error", err)
		return 1
	}

	logger := logging.New(root)
	sender := unosendclient.NewClient(cfg.UnosendAPIKey, cfg.UnosendAPIURL, cfg.MaxRetries, logger)
	svc := app.NewService(sender, logger, cfg.SendTimeout)

	smtpSrv := smtpserver.NewServer(cfg.SMTPListenAddr, svc, smtpserver.Options{
		RequireAuth:        cfg.RequireAuth,
		Username:           cfg.SMTPUser,
		Password:           cfg.SMTPPass,
		AllowedFromDomains: cfg.AllowedFromDomains,
	})

	ready := func() bool { return strings.TrimSpace(cfg.UnosendAPIKey) != "" }
	healthSrv := health.NewServer(cfg.HealthAddr, ready)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run both listeners concurrently; a fatal error from either triggers shutdown.
	errCh := make(chan error, 2)
	go func() {
		root.Info("smtp_listen", "addr", cfg.SMTPListenAddr, "require_auth", cfg.RequireAuth)
		// Serve returns nil on graceful shutdown; any non-nil error is fatal.
		if err := smtpSrv.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("smtp server: %w", err)
		}
	}()
	go func() {
		root.Info("health_listen", "addr", cfg.HealthAddr)
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("health server: %w", err)
		}
	}()

	exitCode := 0
	select {
	case <-ctx.Done():
		root.Info("shutdown_signal_received")
	case err := <-errCh:
		root.Error("server_error", "error", err)
		exitCode = 1
	}

	// Stop intercepting signals so a second Ctrl-C can force-kill.
	stop()

	// Graceful shutdown: stop accepting new connections and drain in-flight
	// sends (bounded by the send timeout plus a small margin).
	grace := cfg.SendTimeout + 5*time.Second
	if grace > 30*time.Second {
		grace = 30 * time.Second
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := smtpSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, smtpserver.ErrServerClosed) {
			root.Error("smtp_shutdown_error", "error", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := healthSrv.Shutdown(shutdownCtx); err != nil {
			root.Error("health_shutdown_error", "error", err)
		}
	}()
	wg.Wait()

	root.Info("shutdown_complete")
	return exitCode
}

// runHealthcheck performs an HTTP GET against the local /health endpoint and
// returns 0 when it reports healthy (HTTP 200), non-zero otherwise. It does not
// require the full service configuration, so it is safe to use as a container
// HEALTHCHECK or manual smoke test.
func runHealthcheck(root *slog.Logger) int {
	addr := config.HealthAddrFromEnv()
	host := addr
	if strings.HasPrefix(addr, ":") {
		host = "127.0.0.1" + addr
	}
	url := "http://" + host + "/health"

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		root.Error("healthcheck_failed", "url", url, "error", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		root.Error("healthcheck_unhealthy", "url", url, "status", resp.StatusCode)
		return 1
	}
	root.Info("healthcheck_ok", "url", url)
	return 0
}
