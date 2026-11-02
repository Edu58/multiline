package multiline

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDispatcherPassesExecutionContextAndError(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(t.Context(), key{}, "request")
	expected := Execution{ID: uuid.New(), Type: "email", Attempt: 2}
	sentinel := errors.New("delivery failed")
	registry := NewWorkerRegistry()
	if err := registry.Register("email", func(ctx context.Context, actual Execution) error {
		if ctx.Value(key{}) != "request" || actual.ID != expected.ID || actual.Attempt != expected.Attempt {
			t.Errorf("handler input = %+v", actual)
		}
		return sentinel
	}); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := registry.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.handle(ctx, expected); !errors.Is(err, sentinel) {
		t.Fatalf("handler error = %v", err)
	}
	// Names are case-sensitive and missing types fail permanently.
	if err := dispatcher.handle(ctx, Execution{Type: "EMAIL"}); !IsPermanent(err) {
		t.Fatalf("missing handler = %v", err)
	}
}
