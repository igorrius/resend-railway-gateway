package domain

import "errors"

// SendError classifies a failed delivery attempt so upstream layers (e.g. the
// SMTP adapter) can decide whether to signal the client a temporary (4xx) or
// permanent (5xx) failure.
//
// Transient failures are conditions that may succeed if retried later
// (rate limiting, provider 5xx, network/timeout). Permanent failures are
// caused by the message itself (validation, authentication) and will not
// succeed on retry.
type SendError struct {
	// Transient indicates the failure may be resolved by retrying later.
	Transient bool
	// StatusCode is the upstream HTTP status that triggered the failure, if any.
	StatusCode int
	// Err is the underlying error.
	Err error
}

func (e *SendError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "send failed"
}

func (e *SendError) Unwrap() error { return e.Err }

// IsTransient reports whether err represents a transient send failure. Errors
// that are not classified SendErrors are treated as permanent.
func IsTransient(err error) bool {
	var se *SendError
	if errors.As(err, &se) {
		return se.Transient
	}
	return false
}
