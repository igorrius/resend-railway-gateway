package domain

// OutboundEmailSender is a port for sending emails to an external provider.
// Implementations of this interface handle the actual delivery of emails
// through services like Resend, SendGrid, etc.
type OutboundEmailSender interface {
	Send(email Email) error
}

// MessageLogger abstracts logging in the domain/app layers.
// It provides structured logging with key-value pairs for better observability.
type MessageLogger interface {
	Info(msg string, fields map[string]any)
	Error(msg string, fields map[string]any)
}
