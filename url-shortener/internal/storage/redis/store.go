package redis

import (
	"context"
	"errors"
	"time"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/config"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/observability"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/shortener"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/extra/redisprometheus/v9"
	goredis "github.com/redis/go-redis/v9"
)

type Store struct {
	client *goredis.Client
	// One slot per pool connection, taken before every command and given back
	// after it. go-redis then never queues: the wait for a free connection
	// happens here instead, where it's timed exactly, timeouts included
	// (go-redis' own wait counters skip waits that time out). Waiting senders
	// on a channel are served first come, first served.
	slots   chan struct{}
	timeout time.Duration
	query   observability.Call
	metrics *metrics
}

var _ shortener.Store = (*Store)(nil)

func New(cfg config.Redis, reg prometheus.Registerer) (*Store, error) {
	opts, err := goredis.ParseURL(cfg.Uri)
	if err != nil {
		return nil, err
	}
	opts.ContextTimeoutEnabled = true
	opts.PoolSize = cfg.Pool.Size

	client := goredis.NewClient(opts)
	reg.MustRegister(redisprometheus.NewCollector("redis", "", client))
	reg.MustRegister(newPoolCollector(client, cfg.Timeouts.Query))

	m := newMetrics(reg)
	return &Store{
		client:  client,
		slots:   make(chan struct{}, client.Options().PoolSize),
		timeout: cfg.Timeouts.Query,
		query: observability.Call{
			Dependency: "redis",
			Timeout:    cfg.Timeouts.Query,
			Duration:   m.queryDuration,
		},
		metrics: m,
	}, nil
}

func (s *Store) GetLongUrl(ctx context.Context, code string) (longUrl string, err error) {
	err = s.withSlot(ctx, func(ctx context.Context) error {
		return s.query.Do(ctx, "get_long_url", func(ctx context.Context) error {
			longUrl, err = s.client.Get(ctx, code).Result()
			if errors.Is(err, goredis.Nil) {
				return shortener.ErrNotFound
			}
			return err
		})
	})
	return longUrl, err
}

func (s *Store) PutShortUrl(ctx context.Context, code, longUrl string) error {
	return s.withSlot(ctx, func(ctx context.Context) error {
		return s.query.Do(ctx, "put_short_url", func(ctx context.Context) error {
			return s.client.Set(ctx, code, longUrl, 0).Err()
		})
	})
}

// Runs fn holding a connection slot. The query timeout covers both waiting
// for the slot and the command.
func (s *Store) withSlot(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	acquired, err := s.acquireConn(ctx)
	if err != nil {
		return err
	}
	defer s.releaseConn(acquired)
	return fn(ctx)
}

// Waits for a free connection slot until ctx is done, and returns when it got one
func (s *Store) acquireConn(ctx context.Context) (time.Time, error) {
	start := time.Now()
	select {
	case s.slots <- struct{}{}:
		acquired := time.Now()
		s.metrics.acquireDuration.WithLabelValues("ok").Observe(acquired.Sub(start).Seconds())
		return acquired, nil
	case <-ctx.Done():
		elapsed := time.Since(start)
		err := ctx.Err()
		s.metrics.acquireDuration.WithLabelValues(observability.Outcome(err)).Observe(elapsed.Seconds())
		if errors.Is(err, context.DeadlineExceeded) {
			return time.Time{}, &shortener.Overloaded{
				Dependency: "redis",
				Operation:  "acquire_conn",
				Timeout:    s.timeout,
				Elapsed:    elapsed,
				Err:        err,
			}
		}
		return time.Time{}, err
	}
}

// Gives the slot back and records how long it was held
func (s *Store) releaseConn(acquired time.Time) {
	<-s.slots
	s.metrics.connHeldSeconds.Add(time.Since(acquired).Seconds())
}

func (s *Store) Close() error {
	return s.client.Close()
}
