package unosend

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func makeResp(code int, header http.Header, body string) *http.Response {
	h := header
	if h == nil {
		h = http.Header{}
	}
	return &http.Response{
		StatusCode: code,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newReq(t *testing.T, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://api.unosend.co/v1/emails", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// noSleep replaces the real backoff sleep in tests.
func noSleep(*retryTransport) func(context.Context, time.Duration) error {
	return func(context.Context, time.Duration) error { return nil }
}

func TestRetryTransport_RetriesThenSucceeds(t *testing.T) {
	var calls int
	base := rtFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != "payload" {
			t.Errorf("body not replayed on attempt %d: %q", calls, b)
		}
		calls++
		if calls < 3 {
			return makeResp(http.StatusTooManyRequests, nil, "rate limited"), nil
		}
		return makeResp(http.StatusOK, nil, "ok"), nil
	})
	tr := newRetryTransport(base, 3)
	var slept []time.Duration
	tr.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	tr.randInt63n = func(n int64) int64 { return n - 1 }

	resp, err := tr.RoundTrip(newReq(t, "payload"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
	if len(slept) != 2 {
		t.Errorf("backoff sleeps = %d, want 2", len(slept))
	}
}

func TestRetryTransport_ExhaustsRetriesReturnsTransient(t *testing.T) {
	var calls int
	base := rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return makeResp(http.StatusServiceUnavailable, nil, "down"), nil
	})
	tr := newRetryTransport(base, 2)
	tr.sleep = noSleep(tr)

	_, err := tr.RoundTrip(newReq(t, "x"))
	te, ok := asTransient(err)
	if !ok {
		t.Fatalf("expected transientError, got %v", err)
	}
	if te.statusCode != http.StatusServiceUnavailable {
		t.Errorf("statusCode = %d, want 503", te.statusCode)
	}
	if calls != 3 { // maxRetries(2) + 1
		t.Errorf("calls = %d, want 3", calls)
	}
	if te.attempts != 3 {
		t.Errorf("attempts = %d, want 3", te.attempts)
	}
}

func TestRetryTransport_PermanentNotRetried(t *testing.T) {
	var calls int
	base := rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return makeResp(http.StatusBadRequest, nil, "bad request"), nil
	})
	tr := newRetryTransport(base, 3)
	tr.sleep = func(context.Context, time.Duration) error {
		t.Fatal("sleep should not be called for permanent errors")
		return nil
	}

	resp, err := tr.RoundTrip(newReq(t, "x"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetryTransport_RespectsRetryAfter(t *testing.T) {
	var calls int
	base := rtFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			h := http.Header{}
			h.Set("Retry-After", "2")
			return makeResp(http.StatusTooManyRequests, h, "slow down"), nil
		}
		return makeResp(http.StatusOK, nil, "ok"), nil
	})
	tr := newRetryTransport(base, 3)
	var slept []time.Duration
	tr.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }

	if _, err := tr.RoundTrip(newReq(t, "x")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slept) != 1 {
		t.Fatalf("expected 1 sleep, got %d", len(slept))
	}
	if slept[0] != 2*time.Second {
		t.Errorf("Retry-After not honored: slept %v, want 2s", slept[0])
	}
}

func TestRetryTransport_ContextCancelDuringBackoff(t *testing.T) {
	base := rtFunc(func(r *http.Request) (*http.Response, error) {
		return makeResp(http.StatusInternalServerError, nil, "x"), nil
	})
	tr := newRetryTransport(base, 5)
	tr.sleep = func(context.Context, time.Duration) error { return context.Canceled }

	_, err := tr.RoundTrip(newReq(t, "x"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRetryTransport_NetworkErrorIsTransient(t *testing.T) {
	netErr := errors.New("dial tcp: connection refused")
	base := rtFunc(func(r *http.Request) (*http.Response, error) {
		return nil, netErr
	})
	tr := newRetryTransport(base, 1)
	tr.sleep = noSleep(tr)

	_, err := tr.RoundTrip(newReq(t, "x"))
	if _, ok := asTransient(err); !ok {
		t.Fatalf("expected transientError, got %v", err)
	}
	if !errors.Is(err, netErr) {
		t.Errorf("transient error should wrap the network error")
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", -1},
		{"5", 5 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"garbage", -1},
	}
	for _, c := range cases {
		if got := parseRetryAfter(c.in); got != c.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	// HTTP-date in the past should clamp to 0.
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(past); got != 0 {
		t.Errorf("parseRetryAfter(past date) = %v, want 0", got)
	}
	// HTTP-date in the future should be positive.
	future := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 {
		t.Errorf("parseRetryAfter(future date) = %v, want > 0", got)
	}
}

func TestBackoffWithinCap(t *testing.T) {
	tr := newRetryTransport(nil, 5)
	tr.randInt63n = func(n int64) int64 { return n - 1 } // max jitter
	for attempt := 1; attempt <= 10; attempt++ {
		if d := tr.backoff(attempt); d > tr.maxDelay {
			t.Errorf("backoff(%d) = %v exceeds maxDelay %v", attempt, d, tr.maxDelay)
		}
	}
}
