package multiline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWorkerRegistryValidation(t *testing.T) {
	handler := func(context.Context, Execution) error { return nil }
	for _, tc := range []struct {
		name    string
		handler Handler
		field   string
	}{
		{"", handler, "type"}, {" ", handler, "type"}, {strings.Repeat("x", 21), handler, "type"},
		{"bad\x00name", handler, "type"}, {string([]byte{0xff}), handler, "type"}, {"email", nil, "handler"},
	} {
		registry := NewWorkerRegistry()
		err := registry.Register(tc.name, tc.handler)
		var detail *ValidationError
		if !errors.Is(err, ErrInvalid) || !errors.As(err, &detail) || detail.Field != tc.field {
			t.Fatalf("registration = %v", err)
		}
	}
	var registry *WorkerRegistry
	if err := registry.Register("email", handler); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil registry = %v", err)
	}
}

func TestWorkerRegistryDuplicatePreservesOriginal(t *testing.T) {
	var registry WorkerRegistry // The zero value is ready for registration.
	originalCalled, replacementCalled := false, false
	if err := registry.Register("email", func(context.Context, Execution) error { originalCalled = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("email", func(context.Context, Execution) error { replacementCalled = true; return nil }); !errors.Is(err, ErrDuplicateHandler) {
		t.Fatalf("duplicate = %v", err)
	}
	dispatch, err := registry.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatch.handle(t.Context(), Execution{Type: "email"}); err != nil {
		t.Fatal(err)
	}
	if !originalCalled || replacementCalled {
		t.Fatal("duplicate registration replaced the original handler")
	}
}

func TestSchedulerSnapshotsRegistry(t *testing.T) {
	registry := NewWorkerRegistry()
	originalCalls, laterCalls := 0, 0
	if err := registry.Register("email", func(context.Context, Execution) error { originalCalls++; return nil }); err != nil {
		t.Fatal(err)
	}
	scheduler, err := NewScheduler(&memoryRepository{}, registry, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("report", func(context.Context, Execution) error { laterCalls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.attempts.execute(t.Context(), Execution{Type: "email"}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.attempts.execute(t.Context(), Execution{Type: "report"}); !IsPermanent(err) {
		t.Fatalf("old scheduler observed late registration: %v", err)
	}
	next, err := NewScheduler(&memoryRepository{}, registry, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := next.attempts.execute(t.Context(), Execution{Type: "report"}); err != nil {
		t.Fatal(err)
	}
	if originalCalls != 1 || laterCalls != 1 {
		t.Fatalf("calls = %d, %d", originalCalls, laterCalls)
	}
}

func TestWorkerRegistryConcurrentDuplicatesAndSnapshots(t *testing.T) {
	registry := NewWorkerRegistry()
	handler := func(context.Context, Execution) error { return nil }
	if err := registry.Register("base", handler); err != nil {
		t.Fatal(err)
	}
	var registered atomic.Int32
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			if err := registry.Register("shared", handler); err == nil {
				registered.Add(1)
			} else if !errors.Is(err, ErrDuplicateHandler) {
				t.Error(err)
			}
			if err := registry.Register(fmt.Sprintf("type-%d", i), handler); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			snapshot, err := registry.snapshot()
			if err != nil {
				t.Error(err)
				return
			}
			if err := snapshot.handle(t.Context(), Execution{Type: "base"}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if registered.Load() != 1 {
		t.Fatalf("accepted %d registrations of one key", registered.Load())
	}
	snapshot, err := registry.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for i := range 32 {
		if err := snapshot.handle(t.Context(), Execution{Type: fmt.Sprintf("type-%d", i)}); err != nil {
			t.Error(err)
		}
	}
}

func TestWorkerOptionsDefaultsValidationAndCopy(t *testing.T) {
	handler := func(context.Context, Execution) error { return nil }
	registry := NewWorkerRegistry()
	if err := registry.Register("default", handler); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("zero", handler, WorkerOptions{}); err != nil {
		t.Fatal(err)
	}
	options := []WorkerOptions{{Concurrency: 3}}
	if err := registry.Register("custom", handler, options...); err != nil {
		t.Fatal(err)
	}
	options[0].Concurrency = 99
	if err := registry.Register("negative", handler, WorkerOptions{Concurrency: -1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative workers = %v", err)
	}
	if err := registry.Register("ambiguous", handler, WorkerOptions{}, WorkerOptions{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("multiple options = %v", err)
	}
	snapshot, err := registry.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{"default": 1, "zero": 1, "custom": 3} {
		if got := snapshot.handlers[name].concurrency; got != want {
			t.Fatalf("%s worker count = %d, want %d", name, got, want)
		}
	}
}
