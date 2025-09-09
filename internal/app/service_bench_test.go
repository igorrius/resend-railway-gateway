package app

import (
	"testing"
	"time"

	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

type benchSender struct{}

func (benchSender) Send(_ domain.Email) error { return nil }

type benchLogger struct{}

func (benchLogger) Info(string, map[string]any)  {}
func (benchLogger) Error(string, map[string]any) {}

func BenchmarkHandleEmail(b *testing.B) {
	svc := NewService(benchSender{}, benchLogger{}, time.Second)
	email, _ := domain.NewEmail("a@example.com", []string{"b@example.com"}, "subject", "text body", "<b>bold</b>", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.HandleEmail(email)
	}
}
