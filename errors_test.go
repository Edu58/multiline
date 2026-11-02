package multiline

import (
	"errors"
	"fmt"
	"testing"
)

func TestPermanentPreservesCauseAndClassification(t *testing.T) {
	cause := errors.New("bad recipient")
	wrapped := fmt.Errorf("send: %w", Permanent(cause))
	if !IsPermanent(wrapped) || !errors.Is(wrapped, cause) {
		t.Fatalf("classification lost: %v", wrapped)
	}
	if Permanent(nil) != nil || IsPermanent(cause) || IsPermanent(nil) {
		t.Fatal("incorrect permanent classification")
	}
}
