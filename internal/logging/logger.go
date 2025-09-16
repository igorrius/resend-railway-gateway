package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// StdLogger wraps a *slog.Logger and adapts the domain MessageLogger
// interface (Info, Error with map[string]any fields) to slog's Attr API.
type StdLogger struct{ L *slog.Logger }

// New returns a new StdLogger wrapping the provided slog.Logger.
func New(l *slog.Logger) *StdLogger { return &StdLogger{L: l} }

// mapAttrs converts a map of arbitrary key-value pairs to a slice of slog.Attr for structured logging.
func (l *StdLogger) mapAttrs(fields map[string]any) []slog.Attr {
	if len(fields) == 0 {
		return nil
	}
	attrs := make([]slog.Attr, 0, len(fields))
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	return attrs
}

// Info logs a message at the info level with optional structured fields provided as a map.
func (l *StdLogger) Info(msg string, fields map[string]any) {
	l.L.LogAttrs(context.Background(), slog.LevelInfo, msg, l.mapAttrs(fields)...)
}

// Error logs a message at the error level with optional structured fields provided as a map.
func (l *StdLogger) Error(msg string, fields map[string]any) {
	l.L.LogAttrs(context.Background(), slog.LevelError, msg, l.mapAttrs(fields)...)
}

// NewConfiguredLogger creates a new slog.Logger configured based on environment variables.
// It uses JSON handler for cloud environments and text handler for local development.
// Log level is controlled by LOG_LEVEL environment variable (default: INFO).
func NewConfiguredLogger() *slog.Logger {
	// Determine log level from environment variable
	logLevel := getLogLevel()

	// Determine if running in cloud environment
	isCloudRun := isCloudEnvironment()

	var handler slog.Handler

	if isCloudRun {
		// Use JSON handler for cloud environments
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})
	} else {
		// Use text handler for local development
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: logLevel,
		})
	}

	return slog.New(handler)
}

// getLogLevel parses the LOG_LEVEL environment variable and returns the corresponding slog.Level.
// Defaults to INFO if not set or invalid.
func getLogLevel() slog.Level {
	levelStr := strings.ToUpper(strings.TrimSpace(os.Getenv("LOG_LEVEL")))

	switch levelStr {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo // Default to INFO
	}
}

// isCloudEnvironment determines if the application is running in a cloud environment.
// It checks for common cloud environment indicators.
func isCloudEnvironment() bool {
	// Check for Google Cloud Run
	if os.Getenv("K_SERVICE") != "" || os.Getenv("K_REVISION") != "" {
		return true
	}

	// Check for Railway
	if os.Getenv("RAILWAY_ENVIRONMENT") != "" {
		return true
	}

	// Check for general cloud indicators
	if os.Getenv("PORT") != "" && os.Getenv("NODE_ENV") == "production" {
		return true
	}

	// Check for other common cloud environment variables
	cloudEnvVars := []string{
		"KUBERNETES_SERVICE_HOST",
		"AWS_LAMBDA_FUNCTION_NAME",
		"HEROKU_APP_NAME",
		"CF_INSTANCE_INDEX", // Cloud Foundry
	}

	for _, envVar := range cloudEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}

	return false
}
