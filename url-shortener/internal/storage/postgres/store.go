package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/config"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/observability"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/shortener"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/storage/postgres/sqlc"
	"github.com/IBM/pgxpoolprometheus"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type Store struct {
	pool           *pgxpool.Pool
	acquireTimeout time.Duration
	query          observability.Call
	metrics        *metrics
}

var _ shortener.Store = (*Store)(nil)

func New(ctx context.Context, cfg config.Postgres, reg prometheus.Registerer) (*Store, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.Uri)
	if err != nil {
		return nil, err
	}
	poolCfg.MaxConns = int32(cfg.Pool.MaxConns)
	poolCfg.MinConns = int32(cfg.Pool.MinConns)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	reg.MustRegister(pgxpoolprometheus.NewCollector(pool, map[string]string{}))

	m := newMetrics(cfg, reg)
	return &Store{
		pool:           pool,
		acquireTimeout: cfg.Timeouts.Acquire,
		query: observability.Call{
			Dependency: "postgres",
			Timeout:    cfg.Timeouts.Query,
			Duration:   m.queryDuration,
		},
		metrics: m,
	}, nil
}

func (s *Store) GetLongUrl(ctx context.Context, code string) (longUrl string, err error) {
	conn, err := s.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer s.release(conn, time.Now())

	err = s.query.Do(ctx, "get_long_url", func(ctx context.Context) error {
		longUrl, err = sqlc.New(conn).GetLongUrl(ctx, code)
		if errors.Is(err, pgx.ErrNoRows) {
			return shortener.ErrNotFound
		}
		return err
	})
	return longUrl, err
}

func (s *Store) PutShortUrl(ctx context.Context, code, longUrl string) error {
	conn, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer s.release(conn, time.Now())

	// The query returns the long URL the short one maps to after it ran: the
	// one given, or the one already stored under the same short URL
	var stored string
	err = s.query.Do(ctx, "put_short_url", func(ctx context.Context) error {
		stored, err = sqlc.New(conn).PutShortUrl(ctx, sqlc.PutShortUrlParams{ShortUrl: code, LongUrl: longUrl})
		return err
	})
	if err != nil {
		return err
	}
	if stored != longUrl {
		s.metrics.collisions.Inc()
		return shortener.ErrCollision
	}
	return nil
}

func (s *Store) acquire(ctx context.Context) (*pgxpool.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, s.acquireTimeout)
	defer cancel()

	start := time.Now()
	conn, err := s.pool.Acquire(ctx)
	elapsed := time.Since(start)
	s.metrics.acquireDuration.WithLabelValues(observability.Outcome(err)).Observe(elapsed.Seconds())

	if errors.Is(err, context.DeadlineExceeded) {
		return nil, &shortener.Overloaded{
			Dependency: "postgres",
			Operation:  "acquire_conn",
			Timeout:    s.acquireTimeout,
			Elapsed:    elapsed,
			Err:        err,
		}
	}
	return conn, err
}

// Returns conn to the pool and records how long it was held
func (s *Store) release(conn *pgxpool.Conn, acquired time.Time) {
	conn.Release()
	s.metrics.connHeldSeconds.Add(time.Since(acquired).Seconds())
}

func (s *Store) Close() {
	s.pool.Close()
}
