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
	redisQueryDurationSeconds *prometheus.HistogramVec
}

type RedisUrlRepo struct {
	client  *redis.Client
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

	// Redis pool metrics
	client := redis.NewClient(opts)
	collector := redisprometheus.NewCollector("redis", "", client)
	reg.MustRegister(collector)

	// Custom redis metrics
	metrics := &redisMetrics{
		redisQueryDurationSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name: "redis_query_duration_seconds",
			Help: "Duration of redis queries",
		},
			[]string{"query", "outcome"},
		),
	}

	return &RedisUrlRepo{
		client:  client,
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
		return "", &Overloaded{msg: err.Error()}
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

	// Redis query and metrics
	start := time.Now()
	err := r.client.Set(ctx, shortUrl, longUrl, 0).Err()
	elapsed := time.Since(start).Seconds()
	outcome := redisQueryOutcome(err)
	r.metrics.redisQueryDurationSeconds.With(prometheus.Labels{
		"query":   "put_short_url",
		"outcome": outcome,
	}).Observe(elapsed)

	// Error handling
	if errors.Is(err, context.DeadlineExceeded) {
		return &Overloaded{msg: err.Error()}
	}

	return err
}

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

func (r *RedisUrlRepo) Close() error {
	return r.client.Close()
}
