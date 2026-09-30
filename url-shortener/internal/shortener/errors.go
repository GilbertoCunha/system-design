package shortener

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound    = errors.New("short url not found")
	ErrInvalidUrl  = errors.New("invalid url")
	ErrInvalidCode = errors.New("invalid short url")
	// Two different long URLs hashed to the same short URL
	ErrCollision = errors.New("short url collision")
)

// Overloaded is returned when a dependency doesn't answer within its timeout.
// It records which one and what for, so a log line says more than
// "context deadline exceeded".
type Overloaded struct {
	Dependency string        // "postgres" or "redis"
	Operation  string        // "acquire_conn", or the query name
	Timeout    time.Duration // the limit that ran out
	Elapsed    time.Duration // how long the call actually took
	Err        error
}

func (e *Overloaded) Error() string {
	return fmt.Sprintf(
		"%s %s timed out after %s (limit %s): %v",
		e.Dependency, e.Operation, e.Elapsed.Round(time.Millisecond), e.Timeout, e.Err,
	)
}

func (e *Overloaded) Unwrap() error {
	return e.Err
}

// The cause the 503 this error becomes is counted under: the dependency, and
// whether the time ran out waiting for a pool connection or running the call.
func (e *Overloaded) Cause() string {
	if e.Operation == "acquire_conn" {
		return e.Dependency + "_acquire"
	}
	return e.Dependency + "_query"
}

// ErrorAttrs are the log fields for an error: the dependency, operation and
// timings when it is an Overloaded, so logs can be filtered and counted by them.
func ErrorAttrs(err error) []any {
	if e, ok := errors.AsType[*Overloaded](err); ok {
		return []any{
			"dependency", e.Dependency,
			"operation", e.Operation,
			"timeout_ms", e.Timeout.Milliseconds(),
			"elapsed_ms", e.Elapsed.Milliseconds(),
			"error", err.Error(),
		}
	}
	return []any{"error", err.Error()}
}
