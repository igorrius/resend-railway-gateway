package domain

import "context"

// SendResult carries provider metadata about a successful send.
type SendResult struct {
	// ID is the provider-side identifier for the accepted message
	// (the provider's email/request id). Useful for tracing and support.
	ID string
}

// OutboundEmailSender is a port for sending emails to an external provider.
// Implementations of this interface handle the actual delivery of emails
// through services like Unosend, SendGrid, etc.
//
// The provided context bounds the whole send, including any internal retries;
// implementations must abort promptly when it is cancelled.
type OutboundEmailSender interface {
	Send(ctx context.Context, email Email) (SendResult, error)
}

// MessageLogger abstracts logging in the domain/app layers.
// It provides structured logging with key-value pairs for better observability.
type MessageLogger interface {
	Info(msg string, fields map[string]any)
	Error(msg string, fields map[string]any)
	Debug(msg string, fields map[string]any)
}
