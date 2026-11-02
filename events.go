package multiline

import (
	"github.com/google/uuid"
	"time"
)

// EventKind identifies a scheduler operation or persisted outcome.
type EventKind string

const (
	EventClaimed        EventKind = "claimed"
	EventClaimFailed    EventKind = "claim_failed"
	EventRenewed        EventKind = "renewed"
	EventRenewFailed    EventKind = "renew_failed"
	EventLeaseLost      EventKind = "lease_lost"
	EventCompleted      EventKind = "completed"
	EventRetryScheduled EventKind = "retry_scheduled"
	EventFailed         EventKind = "failed"
	EventFinishFailed   EventKind = "finish_failed"
	EventRecovered      EventKind = "recovered"
	EventRecoveryFailed EventKind = "recovery_failed"
)

// Event contains operation timing and persisted outcomes, never job payloads or
// claim tokens. Err is the handler error for retry/failed outcomes and a
// repository error for failed operations. Recovery counts refer to one batch.
type Event struct {
	Kind     EventKind
	JobID    uuid.UUID
	Attempt  int32
	Duration time.Duration
	Err      error
	RetryAt  *time.Time
	Recovery RecoveryResult
}

// Observer receives concurrent, synchronous callbacks. It must return promptly;
// blocking observers delay execution and lease renewal. Panics are contained and
// optionally logged. Events are best-effort telemetry, not a durable audit log.
type Observer func(Event)

func (s *Scheduler) observe(event Event) {
	if s.opts.Observer == nil {
		return
	}
	defer func() {
		if p := recover(); p != nil && s.opts.Logger != nil {
			s.opts.Logger.Error("observer panic", "panic", p)
		}
	}()
	s.opts.Observer(event)
}
