package multiline

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNormalizeDefaultsAndOwnership(t *testing.T) {
	payload := []byte(`{"message":"hello"}`)
	input := EnqueueParams{Type: "email", RunAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.FixedZone("app", 3*3600)), Payload: payload}
	normalized, err := input.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ID == uuid.Nil || normalized.Timeout != 15*time.Second || normalized.MaxAttempts != 1 || normalized.RetryDelay != time.Second || normalized.MaxRetryDelay != time.Hour {
		t.Fatalf("defaults = %+v", normalized)
	}
	if normalized.RunAt.Location() != time.UTC || !normalized.RunAt.Equal(input.RunAt) {
		t.Fatalf("run time = %v", normalized.RunAt)
	}
	again, err := normalized.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalized, again) {
		t.Fatal("normalization changed an already normalized value")
	}
	if input.ID != uuid.Nil || input.Timeout != 0 || input.MaxAttempts != 0 {
		t.Fatal("modified caller input")
	}
	payload[0] = '['
	if normalized.Payload[0] != '{' {
		t.Fatal("normalized payload aliases caller memory")
	}
	normalized.Payload[0] = '['
	if again.Payload[0] != '{' {
		t.Fatal("normalizing did not make an independent copy")
	}
}

func TestNormalizeDurations(t *testing.T) {
	id := uuid.New()
	input := EnqueueParams{ID: id, Type: "test", RunAt: time.Now(), Timeout: time.Second + 1, RetryDelay: time.Millisecond + 1, MaxRetryDelay: 3*time.Millisecond + 1}
	actual, err := input.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if actual.ID != id || actual.Timeout != 2*time.Second || actual.RetryDelay != 2*time.Millisecond || actual.MaxRetryDelay != 4*time.Millisecond {
		t.Fatalf("rounded = %+v", actual)
	}
	input.RetryDelay = 24 * time.Hour
	input.MaxRetryDelay = 0
	actual, err = input.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if actual.MaxRetryDelay != 24*time.Hour {
		t.Fatalf("default maximum less than initial delay: %v", actual.MaxRetryDelay)
	}
	input.Payload = []byte{}
	actual, err = input.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if actual.Payload != nil {
		t.Fatal("empty payload was not normalized to nil")
	}
}

func TestNormalizePayloadBoundary(t *testing.T) {
	payload := []byte(`"` + strings.Repeat("x", MaxPayloadBytes-2) + `"`)
	input := EnqueueParams{Type: "test", RunAt: time.Now(), Payload: payload}
	actual, err := input.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Payload, payload) {
		t.Fatal("normalization changed JSON")
	}
	input.Payload = []byte(`"` + strings.Repeat("x", MaxPayloadBytes-1) + `"`)
	if _, err := input.Normalize(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized payload = %v", err)
	}
}

func TestNormalizeValidationFields(t *testing.T) {
	valid := EnqueueParams{Type: "test", RunAt: time.Now()}
	for _, tc := range []struct {
		field  string
		change func(*EnqueueParams)
	}{
		{"type", func(p *EnqueueParams) { p.Type = " " }},
		{"type", func(p *EnqueueParams) { p.Type = "bad\x00name" }},
		{"type", func(p *EnqueueParams) { p.Type = string([]byte{0xff}) }},
		{"type", func(p *EnqueueParams) { p.Type = strings.Repeat("x", 21) }},
		{"run_at", func(p *EnqueueParams) { p.RunAt = time.Time{} }},
		{"timeout", func(p *EnqueueParams) { p.Timeout = -1 }},
		{"timeout", func(p *EnqueueParams) { p.Timeout = time.Duration(2147483647)*time.Second + 1 }},
		{"payload", func(p *EnqueueParams) { p.Payload = []byte("{") }},
		{"payload", func(p *EnqueueParams) { p.Payload = []byte{'"', 0xff, '"'} }},
		{"max_attempts", func(p *EnqueueParams) { p.MaxAttempts = -1 }},
		{"max_attempts", func(p *EnqueueParams) { p.MaxAttempts = 1001 }},
		{"retry_delay", func(p *EnqueueParams) { p.RetryDelay = -1 }},
		{"retry_delay", func(p *EnqueueParams) { p.RetryDelay = 24*time.Hour + 1 }},
		{"max_retry_delay", func(p *EnqueueParams) { p.MaxRetryDelay = -1 }},
		{"max_retry_delay", func(p *EnqueueParams) { p.MaxRetryDelay = 24*time.Hour + 1 }},
		{"max_retry_delay", func(p *EnqueueParams) { p.MaxRetryDelay = time.Millisecond }},
	} {
		p := valid
		tc.change(&p)
		actual, err := p.Normalize()
		var detail *ValidationError
		if !errors.Is(err, ErrInvalid) || !errors.As(err, &detail) || detail.Field != tc.field {
			t.Fatalf("field %s: %v", tc.field, err)
		}
		if !reflect.DeepEqual(actual, EnqueueParams{}) {
			t.Fatal("returned partially normalized input on failure")
		}
	}
}
