package redis

import (
	"github.com/GilbertoCunha/system-design/url-shortener/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	queryDuration   *prometheus.HistogramVec
	acquireDuration *prometheus.HistogramVec
	connHeldSeconds prometheus.Counter
}

func newMetrics(reg prometheus.Registerer) *metrics {
	f := promauto.With(reg)
	m := &metrics{
		queryDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "redis_query_duration_seconds",
			Help: "Duration of redis queries",
			// Redis answers in well under a millisecond; with the defaults
			// 99% of commands fell into the first (5ms) bucket, so the p50 and
			// p99 were only the middle and edge of that bucket. Starts at
			// 100µs and stops at the 500ms query timeout.
			Buckets: []float64{
				.0001, .00025, .0005, .00075, .001, .0015, .0025, .005, .01, .025,
				.05, .1, .25, .5,
			},
		}, []string{"query", "outcome"}),
		// Same as the Postgres pool's: waiting and holding, both measured
		// here, add up to all the time a command spends on the pool
		acquireDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "redis_pool_acquire_duration_seconds",
			Help:    "Time spent waiting for a free Redis pool connection, in seconds",
			Buckets: []float64{.0001, .00025, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		}, []string{"outcome"}),
		connHeldSeconds: f.NewCounter(prometheus.CounterOpts{
			Name: "redis_pool_conn_held_seconds_total",
			Help: "Total time Redis pool connections were held, from acquire to release",
		}),
	}

	// Every series at zero before traffic: see httpapi's Metrics.Initialize
	for _, outcome := range observability.Outcomes {
		for _, query := range []string{"get_long_url", "put_short_url"} {
			m.queryDuration.WithLabelValues(query, outcome)
		}
		if outcome != "not_found" {
			m.acquireDuration.WithLabelValues(outcome)
		}
	}
	return m
}
