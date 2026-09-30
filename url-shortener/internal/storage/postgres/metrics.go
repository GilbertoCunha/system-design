package postgres

import (
	"github.com/GilbertoCunha/system-design/url-shortener/internal/config"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	queryDuration   *prometheus.HistogramVec
	acquireDuration *prometheus.HistogramVec
	connHeldSeconds prometheus.Counter
	collisions      prometheus.Counter
}

func newMetrics(cfg config.Postgres, reg prometheus.Registerer) *metrics {
	f := promauto.With(reg)
	m := &metrics{
		queryDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "pg_query_duration_seconds",
			Help: "Postgres query duration in seconds",
			// Three quarters of queries finish under 5ms, one bucket in the
			// defaults. Sub-millisecond to 100ms in fine steps, then up to the
			// 2s query timeout, which is the last bucket that can fill.
			Buckets: []float64{
				.0005, .001, .002, .003, .005, .0075, .01, .015, .025, .05, .075,
				.1, .25, .5, 1, 2,
			},
		}, []string{"query", "outcome"}),
		// The pool's own gauges (acquired, idle) are snapshots taken at scrape
		// time and miss exhaustion that lasts milliseconds. Every acquire lands
		// in this histogram, so waiting for a connection can't hide between
		// scrapes. Finer buckets than the default at the bottom: an acquire
		// from a pool with idle connections takes well under a millisecond.
		acquireDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pg_pool_acquire_duration_seconds",
			Help:    "Time spent waiting for a connection from the Postgres pool, in seconds",
			Buckets: []float64{.0001, .00025, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		}, []string{"outcome"}),
		// Its rate is the average number of connections in use (time held per
		// second), so divided by pgxpool_max_conns it's how full the pool is
		// on average. The pool's own "acquired" gauge is a snapshot per
		// scrape and misses how busy the pool is between them.
		connHeldSeconds: f.NewCounter(prometheus.CounterOpts{
			Name: "pg_pool_conn_held_seconds_total",
			Help: "Total time Postgres pool connections were held, from acquire to release",
		}),
		collisions: f.NewCounter(prometheus.CounterOpts{
			Name: "short_url_collisions_total",
			Help: "Total number of short url collisions",
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

	// Timeouts from the config, so dashboards draw them as limits instead of
	// hard-coding values that change here
	f.NewGauge(prometheus.GaugeOpts{
		Name: "pg_pool_acquire_timeout_seconds",
		Help: "Timeout for getting a connection from the Postgres pool, from the config",
	}).Set(cfg.Timeouts.Acquire.Seconds())
	f.NewGauge(prometheus.GaugeOpts{
		Name: "pg_query_timeout_seconds",
		Help: "Timeout for each Postgres query, from the config",
	}).Set(cfg.Timeouts.Query.Seconds())

	return m
}
