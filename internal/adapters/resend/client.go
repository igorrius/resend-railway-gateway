package resend

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/igorrius/resend-railway-gateway/internal/domain"
	resendgo "github.com/resend/resend-go/v2"
)

// Client implements the Resend API email sender adapter.
// It wraps the Resend Go SDK and adapts it to the domain OutboundEmailSender interface.
type Client struct {
	client *resendgo.Client
	logger domain.MessageLogger
}

// NewClient creates a new Resend client with the given API key.
//
// Outbound requests go through a retrying HTTP transport that retries 429/5xx
// responses and network errors up to maxRetries times with exponential backoff
// and jitter, honoring Retry-After.
func NewClient(apiKey string, maxRetries int, logger domain.MessageLogger) *Client {
	httpClient := &http.Client{
		// The overall deadline is enforced by the caller's context; keep a
		// generous ceiling as a safety net.
		Timeout:   5 * time.Minute,
		Transport: newRetryTransport(http.DefaultTransport, maxRetries),
	}
	return &Client{
		client: resendgo.NewCustomClient(httpClient, apiKey),
		logger: logger,
	}
}

// Send converts the domain Email to Resend's format and sends it via the API.
//
// On failure it returns a *domain.SendError classifying the failure as transient
// (retryable rate-limit / provider outage / timeout) or permanent (validation,
// authentication), letting the SMTP layer choose the right response code.
func (c *Client) Send(ctx context.Context, email domain.Email) (domain.SendResult, error) {
	c.logger.Debug("resend_request_start", map[string]any{
		"to_count":    len(email.To),
		"cc_count":    len(email.Cc),
		"bcc_count":   len(email.Bcc),
		"text_size":   len(email.Text),
		"html_size":   len(email.HTML),
		"attachments": len(email.Attachments),
	})

	attachments := make([]*resendgo.Attachment, 0, len(email.Attachments))
	for _, a := range email.Attachments {
		attachments = append(attachments, &resendgo.Attachment{
			Filename:    a.Filename,
			Content:     a.Content,
			ContentType: a.ContentType,
			ContentId:   a.ContentID,
		})
	}
	tags := make([]resendgo.Tag, 0, len(email.Tags))
	for _, t := range email.Tags {
		tags = append(tags, resendgo.Tag{Name: t.Name, Value: t.Value})
	}
	request := &resendgo.SendEmailRequest{
		From:        email.From,
		To:          email.To,
		Cc:          email.Cc,
		Bcc:         email.Bcc,
		ReplyTo:     email.ReplyTo,
		Subject:     email.Subject,
		Html:        email.HTML,
		Text:        email.Text,
		Attachments: attachments,
		Tags:        tags,
		Headers:     email.Headers,
	}

	resp, err := c.client.Emails.SendWithContext(ctx, request)
	if err != nil {
		return domain.SendResult{}, classifySendError(err)
	}

	c.logger.Debug("resend_request_success", map[string]any{"id": resp.Id})
	return domain.SendResult{ID: resp.Id}, nil
}

// classifySendError maps an error returned by the Resend SDK into a
// *domain.SendError, marking transient conditions as retryable.
func classifySendError(err error) *domain.SendError {
	if te, ok := asTransient(err); ok {
		return &domain.SendError{Transient: true, StatusCode: te.statusCode, Err: err}
	}
	// Context deadline/cancellation is a transient condition from the client's
	// perspective — the message can be retried.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &domain.SendError{Transient: true, Err: err}
	}
	// Everything else (validation, auth, bad request) is permanent.
	return &domain.SendError{Transient: false, Err: err}
}

var _ domain.OutboundEmailSender = (*Client)(nil)
