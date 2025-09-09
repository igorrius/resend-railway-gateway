package domain

// OutboundEmailSender is a port for sending emails to an external provider.
type OutboundEmailSender interface {
	Send(email Email) error
}

// MessageLogger abstracts logging in the domain/app layers.
type MessageLogger interface {
	Info(msg string, fields map[string]any)
	Error(msg string, fields map[string]any)
}
