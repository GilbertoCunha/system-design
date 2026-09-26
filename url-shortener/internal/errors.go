package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
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

// Log fields for an error: the dependency, operation and timings when it is
// an Overloaded, so logs can be filtered and counted by them.
func errorAttrs(err error) []any {
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

type InvalidUrl struct {
	url     string
	message string
}

func (e *InvalidUrl) Error() string {
	return fmt.Sprintf("ERROR InvalidUrl: %s - %s", e.url, e.message)
}

type InvalidShortUrl struct {
	shortUrl string
}

func (e *InvalidShortUrl) Error() string {
	return fmt.Sprintf("ERROR InvalidShortUrl: short url %s is not an MD5 hash.", e.shortUrl)
}

type ShortUrlCollision struct {
	longUrl1 string
	longUrl2 string
}

func (e *ShortUrlCollision) Error() string {
	return fmt.Sprintf("ERROR ShortUrlCollision: %s and %s have the same MD5 hash", e.longUrl1, e.longUrl2)
}

type ShortUrlNotFound struct {
	shortUrl string
}

func (e *ShortUrlNotFound) Error() string {
	return fmt.Sprintf("ERROR ShortUrlNotFound: %s", e.shortUrl)
}

func HttpErrorHandler(w http.ResponseWriter, err error, logger *slog.Logger) bool {
	if _, ok := errors.AsType[*Overloaded](err); ok {
		logger.Error("dependency timeout", errorAttrs(err)...)
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(503)
		return true
	} else if errors.Is(err, context.Canceled) {
		w.WriteHeader(499)
		return true
	} else if err != nil {
		logger.Error("internal server error", "error", err.Error())
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return true
	}
	return false
}
