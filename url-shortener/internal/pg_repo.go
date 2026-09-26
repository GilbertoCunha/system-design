package internal

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/database"
	"github.com/IBM/pgxpoolprometheus"
	pgx "github.com/jackc/pgx/v5"
	pgxpool "github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type PgMetrics struct {
	pgQueryDurationSeconds       *prometheus.HistogramVec
	pgPoolAcquireDurationSeconds *prometheus.HistogramVec
	pgUrlCollisionTotal          prometheus.Counter
}

type PgUrlRepo struct {
	pool    *pgxpool.Pool
	logger  *slog.Logger
	config  *AppConfig
	metrics *PgMetrics
}

func NewPgUrlRepo(ctx context.Context, c *AppConfig, logger *slog.Logger, reg prometheus.Registerer) (*PgUrlRepo, error) {
	config, err := pgxpool.ParseConfig(c.Postgres.Uri)
	if err != nil {
		return nil, err
	}
	config.MaxConns = int32(c.Postgres.Pool.MaxConns)
	config.MinConns = int32(c.Postgres.Pool.MinConns)

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	collector := pgxpoolprometheus.NewCollector(pool, map[string]string{})
	reg.MustRegister(collector)

	// Create prometheus metrics
	metrics := &PgMetrics{
		pgQueryDurationSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name: "pg_query_duration_seconds",
			Help: "Postgres query duration in seconds",
			// Three quarters of queries finish under 5ms, one bucket in the
			// defaults. Sub-millisecond to 100ms in fine steps, then up to the
			// 2s query timeout, which is the last bucket that can fill.
			Buckets: []float64{
				.0005, .001, .002, .003, .005, .0075, .01, .015, .025, .05, .075,
				.1, .25, .5, 1, 2,
			},
		},
			[]string{"query", "outcome"},
		),
		// The pool's own gauges (acquired, idle) are snapshots taken at scrape
		// time and miss exhaustion that lasts milliseconds. Every acquire lands
		// in this histogram, so waiting for a connection can't hide between
		// scrapes. Finer buckets than the default at the bottom: an acquire
		// from a pool with idle connections takes well under a millisecond.
		pgPoolAcquireDurationSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pg_pool_acquire_duration_seconds",
			Help:    "Time spent waiting for a connection from the Postgres pool, in seconds",
			Buckets: []float64{.0001, .00025, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		},
			[]string{"outcome"},
		),
		pgUrlCollisionTotal: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "short_url_collisions_total",
			Help: "Total number of short url collisions",
		}),
	}
	// Every series at zero before traffic: see Metrics.Initialize
	for _, outcome := range queryOutcomes {
		for _, query := range []string{"get_long_url", "put_short_url"} {
			metrics.pgQueryDurationSeconds.WithLabelValues(query, outcome)
		}
		if outcome != "not_found" {
			metrics.pgPoolAcquireDurationSeconds.WithLabelValues(outcome)
		}
	}
	repo := &PgUrlRepo{pool: pool, logger: logger, config: c, metrics: metrics}

	return repo, nil
}

func (r *PgUrlRepo) GetLongUrl(ctx context.Context, shortUrl string) (string, error) {
	conn, err := r.AcquireConn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Release()

	queryCtx, cancel := context.WithTimeout(
		ctx,
		time.Duration(r.config.Postgres.Timeouts.QueryTimeoutMs)*time.Millisecond,
	)
	defer cancel()

	start := time.Now()
	longUrl, err := database.New(conn).GetLongUrl(queryCtx, shortUrl)
	outcome := queryOutcome(err)
	elapsed := time.Since(start).Seconds()
	r.metrics.pgQueryDurationSeconds.With(prometheus.Labels{
		"query":   "get_long_url",
		"outcome": outcome,
	}).Observe(elapsed)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", &ShortUrlNotFound{shortUrl: shortUrl}
	case errors.Is(err, context.DeadlineExceeded):
		return "", &Overloaded{
			Dependency: "postgres",
			Operation:  "get_long_url",
			Timeout:    time.Duration(r.config.Postgres.Timeouts.QueryTimeoutMs) * time.Millisecond,
			Elapsed:    time.Duration(elapsed * float64(time.Second)),
			Err:        err,
		}
	case err != nil:
		return "", err
	default:
		return longUrl, nil
	}
}

func (r *PgUrlRepo) PutShortUrl(ctx context.Context, shortUrl string, longUrl string) error {
	conn, err := r.AcquireConn(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	queryCtx, cancel := context.WithTimeout(
		ctx,
		time.Duration(r.config.Postgres.Timeouts.QueryTimeoutMs)*time.Millisecond,
	)
	defer cancel()
	start := time.Now()

	queryLongUrl, err := database.New(conn).PutShortUrl(
		queryCtx,
		database.PutShortUrlParams{ShortUrl: shortUrl, LongUrl: longUrl},
	)
	outcome := queryOutcome(err)
	elapsed := time.Since(start).Seconds()
	r.metrics.pgQueryDurationSeconds.With(prometheus.Labels{
		"query":   "put_short_url",
		"outcome": outcome,
	}).Observe(elapsed)

	// Error handling
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return &Overloaded{
			Dependency: "postgres",
			Operation:  "put_short_url",
			Timeout:    time.Duration(r.config.Postgres.Timeouts.QueryTimeoutMs) * time.Millisecond,
			Elapsed:    time.Duration(elapsed * float64(time.Second)),
			Err:        err,
		}
	case err != nil:
		return err
	}

	// If queryLongUrl is returned, a collision happened
	// 1. If it's the same as longUrl, then this URL has already been shortened
	// 2. If it's a different longUrl, then an actual collision occurred
	if queryLongUrl != longUrl {
		r.metrics.pgUrlCollisionTotal.Inc()
		return &ShortUrlCollision{longUrl1: longUrl, longUrl2: queryLongUrl}
	}

	return nil
}

// Every value queryOutcome returns
var queryOutcomes = []string{"ok", "not_found", "timeout", "canceled", "error"}

func queryOutcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, pgx.ErrNoRows):
		return "not_found"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error"
	}
}

func (r *PgUrlRepo) AcquireConn(ctx context.Context) (*pgxpool.Conn, error) {
	timeout := time.Duration(r.config.Postgres.Timeouts.AcquireTimeoutMs) * time.Millisecond
	acqCtx, cancel := context.WithTimeout(ctx, timeout)
	start := time.Now()
	conn, err := r.pool.Acquire(acqCtx)
	elapsed := time.Since(start)
	defer cancel()
	r.metrics.pgPoolAcquireDurationSeconds.With(prometheus.Labels{
		"outcome": queryOutcome(err),
	}).Observe(elapsed.Seconds())

	// Check for context deadline exceeded error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return nil, &Overloaded{
			Dependency: "postgres",
			Operation:  "acquire_conn",
			Timeout:    timeout,
			Elapsed:    elapsed,
			Err:        err,
		}
	case err != nil:
		return nil, err
	default:
		return conn, nil
	}
}

func (r *PgUrlRepo) Close() {
	r.pool.Close()
}
