package resend

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

type nopLogger struct{}

func (nopLogger) Info(string, map[string]any)  {}
func (nopLogger) Error(string, map[string]any) {}
func (nopLogger) Debug(string, map[string]any) {}

// newTestClient points a real Client at a test server URL.
func newTestClient(t *testing.T, serverURL string, maxRetries int) *Client {
	t.Helper()
	c := NewClient("re_test_key", maxRetries, nopLogger{})
	base, err := url.Parse(serverURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	c.client.BaseURL = base
	return c
}

func TestSend_SuccessReturnsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"email-123"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 0)
	email, _ := domain.NewEmail("from@example.com", []string{"to@example.com"}, "Subject", "text", "", nil)

	res, err := c.Send(context.Background(), email)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ID != "email-123" {
		t.Errorf("ID = %q, want email-123", res.ID)
	}
}

// TestSend_MapsDomainEmailToRequest verifies the MIME->Resend mapping performed
// by the client: domain.Email fields must land in the outbound Resend payload.
func TestSend_MapsDomainEmailToRequest(t *testing.T) {
	var payload struct {
		From        string            `json:"from"`
		To          []string          `json:"to"`
		Cc          []string          `json:"cc"`
		Bcc         []string          `json:"bcc"`
		ReplyTo     string            `json:"reply_to"`
		Subject     string            `json:"subject"`
		Html        string            `json:"html"`
		Text        string            `json:"text"`
		Headers     map[string]string `json:"headers"`
		Attachments []struct {
			Content     []int  `json:"content"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			ContentId   string `json:"content_id"`
		} `json:"attachments"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 0)
	email := domain.Email{
		From:    "sender@example.com",
		To:      []string{"a@example.com", "b@example.com"},
		Cc:      []string{"cc@example.com"},
		Bcc:     []string{"bcc@example.com"},
		ReplyTo: "reply@example.com",
		Subject: "Hello",
		Text:    "plain body",
		HTML:    "<b>rich</b>",
		Headers: map[string]string{"X-Custom": "v"},
		Attachments: []domain.Attachment{{
			Filename:    "f.txt",
			Content:     []byte("data"),
			ContentType: "text/plain",
			ContentID:   "<cid1>",
		}},
	}

	if _, err := c.Send(context.Background(), email); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payload.From != "sender@example.com" {
		t.Errorf("from = %q", payload.From)
	}
	if len(payload.To) != 2 || payload.To[0] != "a@example.com" {
		t.Errorf("to = %v", payload.To)
	}
	if len(payload.Cc) != 1 || payload.Cc[0] != "cc@example.com" {
		t.Errorf("cc = %v", payload.Cc)
	}
	if len(payload.Bcc) != 1 || payload.Bcc[0] != "bcc@example.com" {
		t.Errorf("bcc = %v", payload.Bcc)
	}
	if payload.ReplyTo != "reply@example.com" {
		t.Errorf("reply_to = %q", payload.ReplyTo)
	}
	if payload.Subject != "Hello" || payload.Text != "plain body" || payload.Html != "<b>rich</b>" {
		t.Errorf("subject/text/html mismatch: %+v", payload)
	}
	if payload.Headers["X-Custom"] != "v" {
		t.Errorf("headers = %v", payload.Headers)
	}
	if len(payload.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(payload.Attachments))
	}
	att := payload.Attachments[0]
	if att.Filename != "f.txt" || att.ContentType != "text/plain" || att.ContentId != "<cid1>" {
		t.Errorf("attachment metadata mismatch: %+v", att)
	}
	if len(att.Content) != len("data") {
		t.Errorf("attachment content length = %d, want 4", len(att.Content))
	}
}

func TestSend_TransientErrorClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"server error"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 0) // 0 retries -> single attempt, no sleeps
	email, _ := domain.NewEmail("from@example.com", []string{"to@example.com"}, "s", "t", "", nil)

	_, err := c.Send(context.Background(), email)
	if err == nil {
		t.Fatal("expected error")
	}
	if !domain.IsTransient(err) {
		t.Errorf("expected transient classification, got %v", err)
	}
}

func TestSend_PermanentErrorClassification(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"invalid from address","name":"validation_error"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL, 3)
	email, _ := domain.NewEmail("from@example.com", []string{"to@example.com"}, "s", "t", "", nil)

	_, err := c.Send(context.Background(), email)
	if err == nil {
		t.Fatal("expected error")
	}
	if domain.IsTransient(err) {
		t.Errorf("422 should be permanent, got transient: %v", err)
	}
}

func TestClassifySendError_ContextDeadlineIsTransient(t *testing.T) {
	se := classifySendError(context.DeadlineExceeded)
	if !se.Transient {
		t.Error("context deadline should be transient")
	}
}
