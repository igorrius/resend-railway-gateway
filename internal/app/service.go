package app

import (
	"context"
	"strings"
	"time"

	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

// Service orchestrates handling incoming email messages and delegating to the email provider.
// It handles validation, timeout management, and structured logging.
type Service struct {
	sender  domain.OutboundEmailSender
	logger  domain.MessageLogger
	timeout time.Duration
}

// NewService creates a new Service instance with the given dependencies.
// - sender: Implementation of the email sender
// - logger: Logger for structured logging
// - timeout: Maximum duration to wait for email delivery (including retries)
func NewService(sender domain.OutboundEmailSender, logger domain.MessageLogger, timeout time.Duration) *Service {
	return &Service{sender: sender, logger: logger, timeout: timeout}
}

// HandleEmail validates and sends the email, bounding the whole operation
// (including provider retries) with the configured timeout.
//
// It returns the underlying error unchanged so callers can classify it
// (see domain.IsTransient). Logs never include the message body or full
// recipient addresses — recipients are reduced to their domains.
func (s *Service) HandleEmail(email domain.Email) error {
	if err := email.Validate(); err != nil {
		s.logger.Error("validation_failed", map[string]any{
			"from":       email.From,
			"to_domains": recipientDomains(email.To),
			"error":      err.Error(),
		})
		return err
	}

	// The send uses a fresh, timeout-bounded context (not tied to shutdown) so
	// that in-flight sends can drain during graceful shutdown rather than being
	// cancelled mid-flight.
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	start := time.Now()
	res, err := s.sender.Send(ctx, email)
	latency := time.Since(start)

	fields := map[string]any{
		"message_id": messageID(email),
		"from":       email.From,
		"to_domains": recipientDomains(email.To),
		"resend_id":  res.ID,
		"latency_ms": latency.Milliseconds(),
	}
	if err != nil {
		fields["error"] = err.Error()
		if domain.IsTransient(err) {
			fields["outcome"] = "transient_error"
		} else {
			fields["outcome"] = "permanent_error"
		}
		s.logger.Error("send_failed", fields)
		return err
	}

	fields["outcome"] = "ok"
	s.logger.Info("send_ok", fields)
	return nil
}

// messageID extracts the Message-ID header if present (headers are canonicalized
// to "Message-Id" by the MIME parser). It is safe to log for tracing.
func messageID(email domain.Email) string {
	for _, k := range []string{"Message-Id", "Message-ID", "Message-id"} {
		if v, ok := email.Headers[k]; ok && v != "" {
			return v
		}
	}
	return ""
}

// recipientDomains reduces recipient addresses to their (unique) domains so that
// logs never leak full recipient PII.
func recipientDomains(addrs []string) []string {
	seen := make(map[string]struct{}, len(addrs))
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		d := domainOf(a)
		if d == "" {
			d = "unknown"
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// domainOf extracts the domain portion of an email address, tolerating the
// "Display Name <local@domain>" form.
func domainOf(addr string) string {
	addr = strings.TrimSpace(addr)
	if i := strings.LastIndex(addr, "<"); i >= 0 {
		if j := strings.Index(addr[i:], ">"); j >= 0 {
			addr = addr[i+1 : i+j]
		}
	}
	addr = strings.Trim(addr, "<> ")
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return ""
	}
	return strings.ToLower(addr[at+1:])
}
