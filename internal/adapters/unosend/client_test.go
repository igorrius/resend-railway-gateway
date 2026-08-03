package unosend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/igorrius/unosend-railway-gateway/internal/domain"
)

type nopLogger struct{}

func (nopLogger) Info(string, map[string]any)  {}
func (nopLogger) Error(string, map[string]any) {}
func (nopLogger) Debug(string, map[string]any) {}

func TestSendMapsRequestAndReturnsID(t *testing.T) {
	var got sendRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer un_test" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email-123"}`))
	}))
	defer srv.Close()
	client := NewClient("un_test", srv.URL+"/v1/emails", 0, nopLogger{})
	email := domain.Email{From: "sender@example.com", To: []string{"to@example.com"}, Cc: []string{"cc@example.com"}, Bcc: []string{"bcc@example.com"}, ReplyTo: "reply@example.com", Subject: "hello", Text: "plain", HTML: "<b>html</b>", Headers: map[string]string{"X-Test": "yes"}, Attachments: []domain.Attachment{{Filename: "a.txt", Content: []byte("data"), ContentType: "text/plain", ContentID: "cid"}}, Tags: []domain.Tag{{Name: "type", Value: "test"}}}
	res, err := client.Send(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "email-123" {
		t.Errorf("id = %q", res.ID)
	}
	if got.From != email.From || len(got.To) != 1 || got.ReplyTo != email.ReplyTo || got.Text != email.Text || got.HTML != email.HTML {
		t.Errorf("payload mismatch: %+v", got)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Content != "ZGF0YQ==" {
		t.Errorf("attachment = %+v", got.Attachments)
	}
	if len(got.Tags) != 1 || got.Tags[0].Name != "type" {
		t.Errorf("tags = %+v", got.Tags)
	}
}

func TestSendClassifiesStatuses(t *testing.T) {
	for _, tc := range []struct {
		status    int
		transient bool
	}{{http.StatusTooManyRequests, true}, {http.StatusServiceUnavailable, true}, {http.StatusUnprocessableEntity, false}, {http.StatusUnauthorized, false}} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			}))
			defer srv.Close()
			c := NewClient("key", srv.URL, 0, nopLogger{})
			_, err := c.Send(context.Background(), domain.Email{From: "a@b.com", To: []string{"c@d.com"}, Subject: "s", Text: "t"})
			if err == nil {
				t.Fatal("expected error")
			}
			if domain.IsTransient(err) != tc.transient {
				t.Errorf("transient = %v, want %v: %v", domain.IsTransient(err), tc.transient, err)
			}
		})
	}
}

func TestClassifySendErrorContextDeadlineIsTransient(t *testing.T) {
	if !classifySendError(context.DeadlineExceeded).Transient {
		t.Error("deadline should be transient")
	}
}
