package multiline

import (
	"errors"
	"fmt"
)

var (
	ErrNoJob            = errors.New("multiline: no due job")
	ErrRunning          = errors.New("multiline: scheduler already running")
	ErrInvalid          = errors.New("multiline: invalid input")
	ErrDuplicateJob     = errors.New("multiline: duplicate job ID")
	ErrDuplicateHandler = errors.New("multiline: duplicate handler registration")
	ErrLeaseLost        = errors.New("multiline: lease expired or ownership lost")
)

// ValidationError identifies an invalid public input. Use errors.As to inspect
// Field and errors.Is(err, ErrInvalid) to classify it without matching messages.
type ValidationError struct{ Field, Reason string }

func (e *ValidationError) Error() string {
	return fmt.Sprintf("multiline: invalid %s: %s", e.Field, e.Reason)
}
func (e *ValidationError) Unwrap() error { return ErrInvalid }

// Permanent marks an execution error as non-retryable while preserving its cause.
// Passing nil returns nil. Wrapped permanent errors remain classifiable.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err}
}

type permanentError struct{ error }

func (e *permanentError) Unwrap() error { return e.error }

// IsPermanent reports whether err or a wrapped cause was marked Permanent.
func IsPermanent(err error) bool { var target *permanentError; return errors.As(err, &target) }
