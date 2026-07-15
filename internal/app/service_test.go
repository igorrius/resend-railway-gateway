package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

type fakeSender struct {
	res domain.SendResult
	err error
}

func (f fakeSender) Send(_ context.Context, _ domain.Email) (domain.SendResult, error) {
	return f.res, f.err
}

type nopLogger struct{}

func (nopLogger) Info(string, map[string]any)  {}
func (nopLogger) Error(string, map[string]any) {}
func (nopLogger) Debug(string, map[string]any) {}

func TestHandleEmail_OK(t *testing.T) {
	svc := NewService(fakeSender{res: domain.SendResult{ID: "id-1"}}, nopLogger{}, time.Second)
	email, _ := domain.NewEmail("a@example.com", []string{"b@example.com"}, "hi", "text", "", nil)
	if err := svc.HandleEmail(email); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestHandleEmail_Error(t *testing.T) {
	svc := NewService(fakeSender{err: errors.New("boom")}, nopLogger{}, time.Second)
	email, _ := domain.NewEmail("a@example.com", []string{"b@example.com"}, "hi", "text", "", nil)
	if err := svc.HandleEmail(email); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleEmail_ValidationFailsPermanently(t *testing.T) {
	svc := NewService(fakeSender{}, nopLogger{}, time.Second)
	// No recipients -> validation error, which must be permanent.
	email := domain.Email{From: "a@example.com"}
	err := svc.HandleEmail(email)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if domain.IsTransient(err) {
		t.Error("validation error should be permanent, not transient")
	}
}

func TestHandleEmail_TransientPropagates(t *testing.T) {
	sendErr := &domain.SendError{Transient: true, Err: errors.New("503")}
	svc := NewService(fakeSender{err: sendErr}, nopLogger{}, time.Second)
	email, _ := domain.NewEmail("a@example.com", []string{"b@example.com"}, "hi", "text", "", nil)
	err := svc.HandleEmail(email)
	if !domain.IsTransient(err) {
		t.Errorf("expected transient error to propagate, got %v", err)
	}
}

// captureLogger records fields so we can assert PII-safety.
type captureLogger struct {
	fields map[string]any
}

func (c *captureLogger) Info(_ string, f map[string]any)  { c.fields = f }
func (c *captureLogger) Error(_ string, f map[string]any) { c.fields = f }
func (c *captureLogger) Debug(_ string, _ map[string]any) {}

func TestHandleEmail_LogsDomainsNotFullRecipients(t *testing.T) {
	log := &captureLogger{}
	svc := NewService(fakeSender{res: domain.SendResult{ID: "rid"}}, log, time.Second)
	email, _ := domain.NewEmail("sender@corp.example", []string{"alice@acme.test", "bob@acme.test"}, "s", "t", "", nil)
	email.Headers = map[string]string{"Message-Id": "<abc@corp.example>"}

	if err := svc.HandleEmail(email); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if log.fields == nil {
		t.Fatal("no fields captured")
	}
	doms, _ := log.fields["to_domains"].([]string)
	if len(doms) != 1 || doms[0] != "acme.test" {
		t.Errorf("to_domains = %v, want [acme.test]", doms)
	}
	if log.fields["resend_id"] != "rid" {
		t.Errorf("resend_id = %v, want rid", log.fields["resend_id"])
	}
	if log.fields["outcome"] != "ok" {
		t.Errorf("outcome = %v, want ok", log.fields["outcome"])
	}
	// Ensure no field leaks a full recipient address.
	for k, v := range log.fields {
		if s, ok := v.(string); ok && (s == "alice@acme.test" || s == "bob@acme.test") {
			t.Errorf("field %q leaked full recipient address: %q", k, s)
		}
	}
}

func TestDomainOf(t *testing.T) {
	cases := map[string]string{
		"a@b.com":              "b.com",
		"Name <x@Y.COM>":       "y.com",
		"bad":                  "",
		"nolocalpart@":         "",
		"  spaced@Domain.io  ": "domain.io",
	}
	for in, want := range cases {
		if got := domainOf(in); got != want {
			t.Errorf("domainOf(%q) = %q, want %q", in, got, want)
		}
	}
}
