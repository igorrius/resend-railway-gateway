package resend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// transientError marks a Resend failure that persisted after all retries were
// exhausted (rate limiting, provider 5xx, or repeated network errors). It is
// returned by retryTransport so that the resend adapter can classify the send
// as transient and surface an SMTP 4xx to the client.
type transientError struct {
	statusCode int
	attempts   int
	err        error
}

func (e *transientError) Error() string {
	if e.statusCode > 0 {
		return fmt.Sprintf("resend: transient failure after %d attempt(s): HTTP %d", e.attempts, e.statusCode)
	}
	return fmt.Sprintf("resend: transient failure after %d attempt(s): %v", e.attempts, e.err)
}

func (e *transientError) Unwrap() error { return e.err }

// retryTransport is an http.RoundTripper that retries idempotent-by-key Resend
// requests on 429 and 5xx responses (and network errors) using exponential
// backoff with full jitter, honoring the Retry-After header when present.
//
// On success or a permanent (non-retryable) response it returns the response
// untouched so the Resend SDK can decode it. When retries are exhausted on a
// retryable condition it returns a *transientError.
type retryTransport struct {
	base       http.RoundTripper
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration

	// injectable for deterministic tests
	randInt63n func(n int64) int64
	sleep      func(ctx context.Context, d time.Duration) error
}

func newRetryTransport(base http.RoundTripper, maxRetries int) *retryTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &retryTransport{
		base:       base,
		maxRetries: maxRetries,
		baseDelay:  200 * time.Millisecond,
		maxDelay:   30 * time.Second,
		randInt63n: rand.Int63n,
		sleep:      ctxSleep,
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// Buffer the request body once so every attempt can replay it. The Resend
	// SDK sets Body but not GetBody, so we must capture it here.
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		body = b
	}

	maxAttempts := t.maxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastStatus int
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, rtErr := t.base.RoundTrip(cloneWithBody(req, ctx, body))

		var retryAfter time.Duration = -1
		switch {
		case rtErr != nil:
			// Network/transport error (resp is nil per RoundTripper contract).
			lastErr, lastStatus = rtErr, 0
		case isRetryableStatus(resp.StatusCode):
			lastErr, lastStatus = nil, resp.StatusCode
			retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
			drainClose(resp.Body)
		default:
			// 2xx success or a permanent non-2xx: let the SDK handle it.
			return resp, nil
		}

		if attempt == maxAttempts {
			break
		}
		if werr := t.wait(ctx, attempt, retryAfter); werr != nil {
			return nil, werr
		}
	}

	return nil, &transientError{statusCode: lastStatus, attempts: maxAttempts, err: lastErr}
}

// wait sleeps before the next attempt, honoring Retry-After when provided and
// otherwise using capped exponential backoff with full jitter.
func (t *retryTransport) wait(ctx context.Context, attempt int, retryAfter time.Duration) error {
	var delay time.Duration
	if retryAfter >= 0 {
		delay = retryAfter
	} else {
		delay = t.backoff(attempt)
	}
	if delay > t.maxDelay {
		delay = t.maxDelay
	}
	return t.sleep(ctx, delay)
}

// backoff returns a full-jitter delay in [0, min(maxDelay, base*2^(attempt-1))].
func (t *retryTransport) backoff(attempt int) time.Duration {
	ceil := t.baseDelay
	// base * 2^(attempt-1), guarding against overflow.
	for i := 1; i < attempt; i++ {
		ceil <<= 1
		if ceil <= 0 || ceil > t.maxDelay {
			ceil = t.maxDelay
			break
		}
	}
	if ceil > t.maxDelay {
		ceil = t.maxDelay
	}
	if ceil <= 0 {
		return 0
	}
	return time.Duration(t.randInt63n(int64(ceil) + 1))
}

func cloneWithBody(req *http.Request, ctx context.Context, body []byte) *http.Request {
	r := req.Clone(ctx)
	if body != nil {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	return r
}

func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// parseRetryAfter parses an HTTP Retry-After header (delta-seconds or HTTP-date).
// It returns a negative duration when the header is absent or unparseable.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return -1
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if tm, err := http.ParseTime(v); err == nil {
		if d := time.Until(tm); d > 0 {
			return d
		}
		return 0
	}
	return -1
}

// ctxSleep sleeps for d or until ctx is done, whichever comes first.
func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func drainClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(rc, 4<<10))
	_ = rc.Close()
}

// asTransient reports whether err (possibly wrapped by the SDK/http client) is a
// transientError, returning it when found.
func asTransient(err error) (*transientError, bool) {
	var te *transientError
	if errors.As(err, &te) {
		return te, true
	}
	return nil, false
}
