package app

import (
	"errors"
	"testing"
	"time"

	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

type fakeSender struct{ err error }

func (f fakeSender) Send(_ domain.Email) error { return f.err }

type nopLogger struct{}

func (nopLogger) Info(string, map[string]any)  {}
func (nopLogger) Error(string, map[string]any) {}

func TestHandleEmail_OK(t *testing.T) {
	svc := NewService(fakeSender{}, nopLogger{}, time.Second)
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
