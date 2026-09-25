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
	pgQueryDurationSeconds *prometheus.HistogramVec
	pgUrlCollisionTotal    prometheus.Counter
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
		},
			[]string{"query", "outcome"},
		),
		pgUrlCollisionTotal: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "short_url_collisions_total",
			Help: "Total number of short url collisions",
		}),
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
		return "", &Overloaded{msg: err.Error()}
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
		return &Overloaded{msg: err.Error()}
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
	acqCtx, cancel := context.WithTimeout(
		ctx,
		time.Duration(r.config.Postgres.Timeouts.AcquireTimeoutMs)*time.Millisecond,
	)
	conn, err := r.pool.Acquire(acqCtx)
	defer cancel()

	// Check for context deadline exceeded error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return nil, &Overloaded{msg: err.Error()}
	case err != nil:
		return nil, err
	default:
		return conn, nil
	}
}

func (r *PgUrlRepo) Close() {
	r.pool.Close()
}
