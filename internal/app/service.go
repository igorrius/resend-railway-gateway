package app

import (
	"context"
	"fmt"
	"time"

	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

// Service orchestrates handling incoming messages and delegating to provider.
type Service struct {
	sender  domain.OutboundEmailSender
	logger  domain.MessageLogger
	timeout time.Duration
}

func NewService(sender domain.OutboundEmailSender, logger domain.MessageLogger, timeout time.Duration) *Service {
	return &Service{sender: sender, logger: logger, timeout: timeout}
}

// HandleEmail validates and sends the email with context timeout.
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
