package workers

import (
	"context"
	"testing"

	"github.com/Edu58/multiline/config"
)

type testHandler struct{ called bool }

func (h *testHandler) HandleJob(context.Context, *Job, ...any) error          { h.called = true; return nil }
func (*testHandler) CancelJob(context.Context, *Job, []byte, ...any) error    { return nil }
func (*testHandler) TerminateJob(context.Context, *Job, []byte, ...any) error { return nil }

func TestDispatcherResolvesHandlerName(t *testing.T) {
	registry := NewWorkerRegistry()
	handler := &testHandler{}
	if err := registry.Register("email", handler); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewDispatcher(&config.Config{WORKERS: map[string]string{"EMAIL": "email"}}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.HandleJob(t.Context(), &Job{JobType: "EMAIL"}); err != nil {
		t.Fatal(err)
	}
	if !handler.called {
		t.Fatal("registered handler was not called")
	}
}

func TestDispatcherRejectsMissingDependencies(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cfg      *config.Config
		registry *WorkerRegistry
	}{
		{"nil config", nil, NewWorkerRegistry()},
		{"nil registry", &config.Config{}, nil},
		{"missing handler", &config.Config{WORKERS: map[string]string{"EMAIL": "email"}}, NewWorkerRegistry()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewDispatcher(tc.cfg, tc.registry); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
