package internal

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/extra/redisprometheus/v9"
	"github.com/redis/go-redis/v9"
)

type redisMetrics struct {
	redisQueryDurationSeconds       *prometheus.HistogramVec
	redisPoolAcquireDurationSeconds *prometheus.HistogramVec
	redisPoolConnHeldSecondsTotal   prometheus.Counter
}

type RedisUrlRepo struct {
	client *redis.Client
	// One slot per pool connection, taken before every command and given back
	// after it. go-redis then never queues: the wait for a free connection
	// happens here instead, where it's timed exactly, timeouts included
	// (go-redis' own wait counters skip waits that time out). Waiting senders
	// on a channel are served first come, first served.
	slots   chan struct{}
	logger  *slog.Logger
	config  *AppConfig
	metrics *redisMetrics
}

func NewRedisUrlRepo(c *AppConfig, logger *slog.Logger, reg prometheus.Registerer) (*RedisUrlRepo, error) {
	opts, err := redis.ParseURL(c.Redis.Uri)
	if err != nil {
		return nil, err
	}
	opts.ContextTimeoutEnabled = true
	opts.PoolSize = c.Redis.Pool.Size

	// Redis pool metrics
	client := redis.NewClient(opts)
	collector := redisprometheus.NewCollector("redis", "", client)
	reg.MustRegister(collector)
	reg.MustRegister(newRedisPoolCollector(
		client,
		time.Duration(c.Redis.Timeouts.QueryTimeoutMs)*time.Millisecond,
	))

	// Custom redis metrics
	metrics := &redisMetrics{
		redisQueryDurationSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
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
		},
			[]string{"query", "outcome"},
		),
		// Same as the Postgres pool's: waiting and holding, both measured
		// here, add up to all the time a command spends on the pool
		redisPoolAcquireDurationSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name:    "redis_pool_acquire_duration_seconds",
			Help:    "Time spent waiting for a free Redis pool connection, in seconds",
			Buckets: []float64{.0001, .00025, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		},
			[]string{"outcome"},
		),
		redisPoolConnHeldSecondsTotal: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Name: "redis_pool_conn_held_seconds_total",
			Help: "Total time Redis pool connections were held, from acquire to release",
		}),
	}
	// Every series at zero before traffic: see Metrics.Initialize
	for _, outcome := range redisQueryOutcomes {
		for _, query := range []string{"get_long_url", "put_short_url"} {
			metrics.redisQueryDurationSeconds.WithLabelValues(query, outcome)
		}
		if outcome != "not_found" {
			metrics.redisPoolAcquireDurationSeconds.WithLabelValues(outcome)
		}
	}

	return &RedisUrlRepo{
		client:  client,
		slots:   make(chan struct{}, client.Options().PoolSize),
		logger:  logger,
		config:  c,
		metrics: metrics,
	}, nil
}

func (r *RedisUrlRepo) GetLongUrl(ctx context.Context, shortUrl string) (string, error) {
	ctx, cancel := context.WithTimeout(
		ctx,
		time.Duration(r.config.Redis.Timeouts.QueryTimeoutMs)*time.Millisecond,
	)
	defer cancel()

	acquired, err := r.acquireConn(ctx)
	if err != nil {
		return "", err
	}
	defer r.releaseConn(acquired)

	// Redis query and metrics
	start := time.Now()
	longUrl, err := r.client.Get(ctx, shortUrl).Result()
	elapsed := time.Since(start).Seconds()
	outcome := redisQueryOutcome(err)
	r.metrics.redisQueryDurationSeconds.With(prometheus.Labels{
		"query":   "get_long_url",
		"outcome": outcome,
	}).Observe(elapsed)

	// Error handling
	if errors.Is(err, redis.Nil) {
		return "", &ShortUrlNotFound{shortUrl: shortUrl}
	} else if errors.Is(err, context.DeadlineExceeded) {
		return "", &Overloaded{
			Dependency: "redis",
			Operation:  "get_long_url",
			Timeout:    time.Duration(r.config.Redis.Timeouts.QueryTimeoutMs) * time.Millisecond,
			Elapsed:    time.Duration(elapsed * float64(time.Second)),
			Err:        err,
		}
	} else if err != nil {
		return "", err
	}

	return longUrl, nil
}

func (r *RedisUrlRepo) PutShortUrl(ctx context.Context, shortUrl string, longUrl string) error {
	ctx, cancel := context.WithTimeout(
		ctx,
		time.Duration(r.config.Redis.Timeouts.QueryTimeoutMs)*time.Millisecond,
	)
	defer cancel()

	acquired, err := r.acquireConn(ctx)
	if err != nil {
		return err
	}
	defer r.releaseConn(acquired)

	// Redis query and metrics
	start := time.Now()
	err = r.client.Set(ctx, shortUrl, longUrl, 0).Err()
	elapsed := time.Since(start).Seconds()
	outcome := redisQueryOutcome(err)
	r.metrics.redisQueryDurationSeconds.With(prometheus.Labels{
		"query":   "put_short_url",
		"outcome": outcome,
	}).Observe(elapsed)

	// Error handling
	if errors.Is(err, context.DeadlineExceeded) {
		return &Overloaded{
			Dependency: "redis",
			Operation:  "put_short_url",
			Timeout:    time.Duration(r.config.Redis.Timeouts.QueryTimeoutMs) * time.Millisecond,
			Elapsed:    time.Duration(elapsed * float64(time.Second)),
			Err:        err,
		}
	}

	return err
}

// Every value redisQueryOutcome returns
var redisQueryOutcomes = []string{"ok", "timeout", "not_found", "canceled", "error"}

func redisQueryOutcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, redis.Nil):
		return "not_found"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error"
	}
}

// Waits for a free connection slot until ctx is done, and returns when it got one
func (r *RedisUrlRepo) acquireConn(ctx context.Context) (time.Time, error) {
	start := time.Now()
	select {
	case r.slots <- struct{}{}:
		acquired := time.Now()
		r.metrics.redisPoolAcquireDurationSeconds.WithLabelValues("ok").Observe(acquired.Sub(start).Seconds())
		return acquired, nil
	case <-ctx.Done():
		elapsed := time.Since(start)
		err := ctx.Err()
		r.metrics.redisPoolAcquireDurationSeconds.WithLabelValues(redisQueryOutcome(err)).Observe(elapsed.Seconds())
		if errors.Is(err, context.DeadlineExceeded) {
			return time.Time{}, &Overloaded{
				Dependency: "redis",
				Operation:  "acquire_conn",
				Timeout:    time.Duration(r.config.Redis.Timeouts.QueryTimeoutMs) * time.Millisecond,
				Elapsed:    elapsed,
				Err:        err,
			}
		}
		return time.Time{}, err
	}
}

// Gives the slot back and records how long it was held
func (r *RedisUrlRepo) releaseConn(acquired time.Time) {
	<-r.slots
	r.metrics.redisPoolConnHeldSecondsTotal.Add(time.Since(acquired).Seconds())
}

func (r *RedisUrlRepo) Close() error {
	return r.client.Close()
}
