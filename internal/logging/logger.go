package logging

import (
	"context"
	"log/slog"
)

// StdLogger wraps a *slog.Logger and adapts the domain MessageLogger
// interface (Info, Error with map[string]any fields) to slog's Attr API.
type StdLogger struct{ L *slog.Logger }

// New returns a new StdLogger wrapping the provided slog.Logger.
func New(l *slog.Logger) *StdLogger { return &StdLogger{L: l} }

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

func (l *StdLogger) Info(msg string, fields map[string]any) {
	l.L.LogAttrs(context.Background(), slog.LevelInfo, msg, l.mapAttrs(fields)...)
}

func (l *StdLogger) Error(msg string, fields map[string]any) {
	l.L.LogAttrs(context.Background(), slog.LevelError, msg, l.mapAttrs(fields)...)
}
