package smtp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	goSMTP "github.com/emersion/go-smtp"
	"github.com/igorrius/resend-railway-gateway/internal/app"
	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

type fakeSender struct{ err error }

func (f fakeSender) Send(_ context.Context, _ domain.Email) (domain.SendResult, error) {
	return domain.SendResult{ID: "id"}, f.err
}

type nopLogger struct{}

func (nopLogger) Info(string, map[string]any)  {}
func (nopLogger) Error(string, map[string]any) {}
func (nopLogger) Debug(string, map[string]any) {}

func newTestService() *app.Service {
	return app.NewService(fakeSender{}, nopLogger{}, time.Second)
}

// --- Unit-level checks of the session state machine ---

func TestSession_MailRejectedWithoutAuth(t *testing.T) {
	s := &Session{service: newTestService(), opts: Options{RequireAuth: true, Username: "u", Password: "p"}}
	err := s.Mail("a@example.com", nil)
	if !errors.Is(err, goSMTP.ErrAuthRequired) {
		t.Fatalf("Mail without auth = %v, want ErrAuthRequired", err)
	}
}

func TestSession_AuthPlainSucceedsThenMailAllowed(t *testing.T) {
	s := &Session{service: newTestService(), opts: Options{RequireAuth: true, Username: "u", Password: "p"}}
	srv, err := s.Auth(sasl.Plain)
	if err != nil {
		t.Fatalf("Auth(PLAIN): %v", err)
	}
	// PLAIN initial response: \x00<user>\x00<pass>
	_, done, err := srv.Next([]byte("\x00u\x00p"))
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !done {
		t.Fatal("expected auth to complete")
	}
	if !s.authenticated {
		t.Fatal("session should be authenticated")
	}
	if err := s.Mail("a@example.com", nil); err != nil {
		t.Fatalf("Mail after auth = %v, want nil", err)
	}
}

func TestSession_AuthPlainWrongPassword(t *testing.T) {
	s := &Session{service: newTestService(), opts: Options{RequireAuth: true, Username: "u", Password: "p"}}
	srv, _ := s.Auth(sasl.Plain)
	if _, _, err := srv.Next([]byte("\x00u\x00wrong")); err == nil {
		t.Fatal("expected auth failure with wrong password")
	}
	if s.authenticated {
		t.Fatal("session must not be authenticated after failed auth")
	}
}

func TestSession_NoAuthWhenNotRequired(t *testing.T) {
	s := &Session{service: newTestService(), opts: Options{RequireAuth: false}}
	if mechs := s.AuthMechanisms(); len(mechs) != 0 {
		t.Errorf("AuthMechanisms = %v, want none when auth not required", mechs)
	}
	if err := s.Mail("a@example.com", nil); err != nil {
		t.Fatalf("Mail without auth (auth disabled) = %v, want nil", err)
	}
}

func TestSession_AllowlistRejectsForeignDomain(t *testing.T) {
	s := &Session{
		service: newTestService(),
		opts:    Options{RequireAuth: false, AllowedFromDomains: []string{"allowed.com"}},
	}
	if err := s.Mail("x@evil.com", nil); !errors.Is(err, errSenderDomainNotAllowed) {
		t.Fatalf("Mail from foreign domain = %v, want errSenderDomainNotAllowed", err)
	}
	if err := s.Mail("x@allowed.com", nil); err != nil {
		t.Fatalf("Mail from allowed domain = %v, want nil", err)
	}
}

func TestToSMTPError(t *testing.T) {
	transient := toSMTPError(&domain.SendError{Transient: true, Err: errors.New("x")})
	if se, ok := transient.(*goSMTP.SMTPError); !ok || se.Code/100 != 4 {
		t.Errorf("transient mapped to %v, want 4xx", transient)
	}
	permanent := toSMTPError(&domain.SendError{Transient: false, Err: errors.New("x")})
	if se, ok := permanent.(*goSMTP.SMTPError); !ok || se.Code/100 != 5 {
		t.Errorf("permanent mapped to %v, want 5xx", permanent)
	}
}

func TestAddressDomain(t *testing.T) {
	cases := map[string]string{
		"a@b.com":   "b.com",
		"<a@B.com>": "b.com",
		"bad":       "",
	}
	for in, want := range cases {
		if got := addressDomain(in); got != want {
			t.Errorf("addressDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- Integration: real listener, real SMTP client ---

func startServer(t *testing.T, opts Options) (addr string, cleanup func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := NewServer(l.Addr().String(), newTestService(), opts)
	go func() { _ = srv.Serve(l) }()
	return l.Addr().String(), func() { _ = srv.Close() }
}

func TestIntegration_UnauthenticatedMailRejected(t *testing.T) {
	addr, cleanup := startServer(t, Options{RequireAuth: true, Username: "u", Password: "p"})
	defer cleanup()

	c, err := goSMTP.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.Hello("localhost"); err != nil {
		t.Fatalf("hello: %v", err)
	}
	if err := c.Mail("a@example.com", nil); err == nil {
		t.Fatal("expected MAIL FROM to be rejected before AUTH")
	}
}

func TestIntegration_AuthenticatedSendSucceeds(t *testing.T) {
	addr, cleanup := startServer(t, Options{RequireAuth: true, Username: "u", Password: "p"})
	defer cleanup()

	c, err := goSMTP.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	if err := c.Hello("localhost"); err != nil {
		t.Fatalf("hello: %v", err)
	}
	if err := c.Auth(sasl.NewPlainClient("", "u", "p")); err != nil {
		t.Fatalf("auth: %v", err)
	}
	if err := c.Mail("a@example.com", nil); err != nil {
		t.Fatalf("mail after auth: %v", err)
	}
	if err := c.Rcpt("b@example.com", nil); err != nil {
		t.Fatalf("rcpt: %v", err)
	}
	w, err := c.Data()
	if err != nil {
		t.Fatalf("data: %v", err)
	}
	if _, err := w.Write([]byte("Subject: hi\r\n\r\nbody\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("data close: %v", err)
	}
}
