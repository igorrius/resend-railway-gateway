package app

import (
	"context"
	"fmt"
	"time"

	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

// Service orchestrates handling incoming email messages and delegating to the email provider.
// It handles validation, timeout management, and error logging.
type Service struct {
	sender  domain.OutboundEmailSender
	logger  domain.MessageLogger
	timeout time.Duration
}

// NewService creates a new Service instance with the given dependencies.
// - sender: Implementation of the email sender
// - logger: Logger for structured logging
// - timeout: Maximum duration to wait for email delivery
func NewService(sender domain.OutboundEmailSender, logger domain.MessageLogger, timeout time.Duration) *Service {
	return &Service{sender: sender, logger: logger, timeout: timeout}
}

// HandleEmail validates and sends the email with context timeout.
// It performs the following steps:
// 1. Validates the email structure
// 2. Creates a context with timeout
// 3. Sends the email asynchronously
// 4. Returns an error if validation fails, send fails, or timeout occurs
func (s *Service) HandleEmail(email domain.Email) error {
	if err := email.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.sender.Send(email) }()

	select {
	case err := <-done:
		if err != nil {
			s.logger.Error("send_failed", map[string]any{"error": err})
			return fmt.Errorf("send failed: %w", err)
		}
		s.logger.Info("send_ok", map[string]any{"to": email.To})
		return nil
	case <-ctx.Done():
		s.logger.Error("send_timeout", map[string]any{"to": email.To})
		return ctx.Err()
	}
}
