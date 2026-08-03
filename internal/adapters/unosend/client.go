package unosend

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/igorrius/unosend-railway-gateway/internal/domain"
)

// Client implements the Unosend email API adapter.
type Client struct {
	httpClient *http.Client
	apiKey     string
	endpoint   string
	logger     domain.MessageLogger
}

// NewClient creates an authenticated Unosend client. apiURL is configurable to
// support Unosend-compatible/self-hosted endpoints and local integration tests.
func NewClient(apiKey, apiURL string, maxRetries int, logger domain.MessageLogger) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 5 * time.Minute, Transport: newRetryTransport(http.DefaultTransport, maxRetries)},
		apiKey:     apiKey, endpoint: strings.TrimRight(apiURL, "/"), logger: logger,
	}
}

type attachment struct {
	Filename    string `json:"filename"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
	ContentID   string `json:"content_id,omitempty"`
}
type tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type sendRequest struct {
	From        string            `json:"from"`
	To          []string          `json:"to"`
	Cc          []string          `json:"cc,omitempty"`
	Bcc         []string          `json:"bcc,omitempty"`
	ReplyTo     string            `json:"reply_to,omitempty"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html,omitempty"`
	Text        string            `json:"text,omitempty"`
	Attachments []attachment      `json:"attachments,omitempty"`
	Tags        []tag             `json:"tags,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}
type sendResponse struct {
	ID string `json:"id"`
}
type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string { return fmt.Sprintf("unosend: HTTP %d: %s", e.status, e.message) }

// Send maps the domain email to Unosend's JSON API and returns its message ID.
func (c *Client) Send(ctx context.Context, email domain.Email) (domain.SendResult, error) {
	c.logger.Debug("unosend_request_start", map[string]any{"to_count": len(email.To), "cc_count": len(email.Cc), "bcc_count": len(email.Bcc), "text_size": len(email.Text), "html_size": len(email.HTML), "attachments": len(email.Attachments)})
	payload := sendRequest{From: email.From, To: email.To, Cc: email.Cc, Bcc: email.Bcc, ReplyTo: email.ReplyTo, Subject: email.Subject, HTML: email.HTML, Text: email.Text, Headers: email.Headers}
	for _, a := range email.Attachments {
		payload.Attachments = append(payload.Attachments, attachment{Filename: a.Filename, Content: base64.StdEncoding.EncodeToString(a.Content), ContentType: a.ContentType, ContentID: a.ContentID})
	}
	for _, t := range email.Tags {
		payload.Tags = append(payload.Tags, tag{Name: t.Name, Value: t.Value})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.SendResult{}, classifySendError(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.SendResult{}, classifySendError(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.SendResult{}, classifySendError(err)
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return domain.SendResult{}, classifySendError(readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var detail struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(responseBody, &detail)
		msg := detail.Message
		if msg == "" {
			msg = detail.Error
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return domain.SendResult{}, classifySendError(&apiError{status: resp.StatusCode, message: msg})
	}
	var result sendResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return domain.SendResult{}, &domain.SendError{Transient: false, Err: fmt.Errorf("decode Unosend response: %w", err)}
	}
	c.logger.Debug("unosend_request_success", map[string]any{"id": result.ID})
	return domain.SendResult{ID: result.ID}, nil
}

func classifySendError(err error) *domain.SendError {
	if te, ok := asTransient(err); ok {
		return &domain.SendError{Transient: true, StatusCode: te.statusCode, Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &domain.SendError{Transient: true, Err: err}
	}
	var ae *apiError
	if errors.As(err, &ae) {
		return &domain.SendError{Transient: ae.status == http.StatusTooManyRequests || ae.status >= 500, StatusCode: ae.status, Err: err}
	}
	return &domain.SendError{Transient: false, Err: err}
}

var _ domain.OutboundEmailSender = (*Client)(nil)
