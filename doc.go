// Package multiline provides a PostgreSQL-backed job scheduler using a timing
// wheel. Applications supply their own store, worker registry,
// dispatcher, logger, and context; the library does not start an HTTP server or
// manage process signals.
//
// This project is under active development. Scheduling and dispatch behavior
// should be evaluated before production use.
package multiline
