package multiline

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxPayloadBytes bounds submitted JSON. Storage adapters may also enforce
	// this limit on their own JSON representation (PostgreSQL uses jsonb text).
	MaxPayloadBytes = 256 << 10
	// MaxErrorBytes bounds the UTF-8 error message retained for an attempt.
	MaxErrorBytes = 4 << 10
)

func invalid(field, reason string) error { return &ValidationError{Field: field, Reason: reason} }

// ValidateJobType checks the shared registration, enqueue, and claim key rules.
// Invalid keys return ErrInvalid with field type. Names are case-sensitive.
func ValidateJobType(name string) error {
	if !utf8.ValidString(name) || strings.ContainsRune(name, 0) || strings.TrimSpace(name) == "" || len(name) > 20 {
		return invalid("type", "must contain 1 to 20 bytes of nonblank UTF-8 text without NUL")
	}
	return nil
}

// Normalize validates scheduling input, supplies defaults, rounds durations to
// the library's persistence precision, and generates a missing ID. It returns
// an independent payload copy and a UTC RunAt without modifying the input.
// Call it before persistence in every adapter. Repeated calls preserve the ID
// and values of an already normalized result. Invalid input returns ErrInvalid
// with a ValidationError describing the field; no ID is generated on failure.
func (p EnqueueParams) Normalize() (EnqueueParams, error) {
	if err := ValidateJobType(p.Type); err != nil {
		return EnqueueParams{}, err
	}
	if p.RunAt.IsZero() {
		return EnqueueParams{}, invalid("run_at", "is required")
	}
	if p.Timeout < 0 || p.Timeout > time.Duration(2147483647)*time.Second {
		return EnqueueParams{}, invalid("timeout", "must be nonnegative and fit integer seconds")
	}
	if len(p.Payload) > MaxPayloadBytes {
		return EnqueueParams{}, invalid("payload", "exceeds 256 KiB")
	}
	if len(p.Payload) > 0 && (!utf8.Valid(p.Payload) || !json.Valid(p.Payload)) {
		return EnqueueParams{}, invalid("payload", "must be valid UTF-8 JSON")
	}
	if p.MaxAttempts < 0 || p.MaxAttempts > 1000 {
		return EnqueueParams{}, invalid("max_attempts", "must be between 1 and 1000, or zero for default")
	}
	if p.RetryDelay < 0 || p.RetryDelay > 24*time.Hour {
		return EnqueueParams{}, invalid("retry_delay", "must be nonnegative and at most 24 hours")
	}
	if p.MaxRetryDelay < 0 || p.MaxRetryDelay > 24*time.Hour {
		return EnqueueParams{}, invalid("max_retry_delay", "must be nonnegative and at most 24 hours")
	}
	if p.Timeout == 0 {
		p.Timeout = 15 * time.Second
	}
	if p.MaxAttempts == 0 {
		p.MaxAttempts = 1
	}
	if p.RetryDelay == 0 {
		p.RetryDelay = time.Second
	}
	if p.MaxRetryDelay == 0 {
		p.MaxRetryDelay = max(time.Hour, p.RetryDelay)
	}
	if p.MaxRetryDelay < p.RetryDelay {
		return EnqueueParams{}, invalid("max_retry_delay", "must not be less than retry_delay")
	}
	p.Timeout = (p.Timeout + time.Second - 1) / time.Second * time.Second
	p.RetryDelay = (p.RetryDelay + time.Millisecond - 1) / time.Millisecond * time.Millisecond
	p.MaxRetryDelay = (p.MaxRetryDelay + time.Millisecond - 1) / time.Millisecond * time.Millisecond
	p.RunAt = p.RunAt.UTC()
	if len(p.Payload) == 0 {
		p.Payload = nil
	} else {
		p.Payload = bytes.Clone(p.Payload)
	}
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return p, nil
}
