package observability

import (
	"context"
	"errors"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/shortener"
)

// Every value Outcome returns
var Outcomes = []string{"ok", "not_found", "timeout", "canceled", "error"}

// Outcome is the outcome label for a dependency call's error. Stores return
// shortener.ErrNotFound for a missing key, whatever their driver calls it.
func Outcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, shortener.ErrNotFound):
		return "not_found"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error"
	}
}
