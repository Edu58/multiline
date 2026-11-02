package multiline

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestChainOrderAndErrorPropagation(t *testing.T) {
	var events []string
	wrapper := func(name string) Middleware {
		return func(next Handler) Handler {
			return func(ctx context.Context, execution Execution) error {
				events = append(events, name+" before")
				err := next(ctx, execution)
				events = append(events, name+" after")
				return err
			}
		}
	}
	sentinel := errors.New("handler failed")
	handler := Chain(func(context.Context, Execution) error { events = append(events, "handler"); return sentinel }, wrapper("outer"), wrapper("inner"))
	if err := handler(t.Context(), Execution{}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	want := []string{"outer before", "inner before", "handler", "inner after", "outer after"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v", events)
	}
}
