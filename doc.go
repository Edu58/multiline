// Package multiline runs persistent one-shot jobs with bounded concurrency,
// renewable leases, token-fenced completion, and opt-in jittered retries.
//
// Applications supply a Repository and Handler functions, then call Run with a
// shutdown context. Intake stops on cancellation; active handlers receive the
// configured grace period before cancellation. A separate bounded recovery loop
// handles expired leases. The postgres package implements persistence using sqlc.
//
// EnqueueParams, Execution, Claim, and JobRecord separate creation, handler input,
// ownership, and inspection. Permanent marks non-retryable execution errors.
// Observer reports operational outcomes; Chain adds handler instrumentation.
//
// Handlers must honor cancellation and make external effects idempotent: leases
// protect database state, not exactly-once side effects.
package multiline
