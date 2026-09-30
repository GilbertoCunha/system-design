package observability

import (
	"context"
	"errors"
	"time"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/shortener"
	"github.com/prometheus/client_golang/prometheus"
)

// Call runs calls to one dependency under its timeout and records how long
// each took. A call that runs out of time becomes a shortener.Overloaded.
type Call struct {
	Dependency string                   // "postgres" or "redis"
	Timeout    time.Duration            // per call
	Duration   *prometheus.HistogramVec // labels: query, outcome
}

func (c Call) Do(ctx context.Context, op string, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	start := time.Now()
	err := fn(ctx)
	elapsed := time.Since(start)
	c.Duration.WithLabelValues(op, Outcome(err)).Observe(elapsed.Seconds())

	if errors.Is(err, context.DeadlineExceeded) {
		return &shortener.Overloaded{
			Dependency: c.Dependency,
			Operation:  op,
			Timeout:    c.Timeout,
			Elapsed:    elapsed,
			Err:        err,
		}
	}
	return err
}
