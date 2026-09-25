package internal

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/extra/redisprometheus/v9"
	"github.com/redis/go-redis/v9"
)

type RedisUrlRepo struct {
	client *redis.Client
	logger *slog.Logger
	config *AppConfig
}

func NewRedisUrlRepo(c *AppConfig, logger *slog.Logger, reg prometheus.Registerer) (*RedisUrlRepo, error) {
	opts, err := redis.ParseURL(c.Redis.Uri)
	if err != nil {
		return nil, err
	}

	// Redis pool metrics
	client := redis.NewClient(opts)
	collector := redisprometheus.NewCollector("", "", client)
	reg.MustRegister(collector)

	return &RedisUrlRepo{
		client: client,
		logger: logger,
		config: c,
	}, nil
}

func (r *RedisUrlRepo) GetLongUrl(ctx context.Context, shortUrl string) (string, error) {
	ctx, cancel := context.WithTimeout(
		ctx,
		time.Duration(r.config.Redis.Timeouts.QueryTimeoutMs)*time.Millisecond,
	)
	defer cancel()

	start := time.Now()
	longUrl, err := r.client.Get(ctx, shortUrl).Result()
	elapsed := time.Since(start)
	r.logger.Debug("redis:query_time",
		"query", "get_long_url",
		"time_ms", elapsed/time.Millisecond,
	)

	if errors.Is(err, redis.Nil) {
		return "", &ShortUrlNotFound{}
	} else if err != nil {
		r.logger.Error("redis:error",
			"query", "get_long_url",
			"error", err.Error(),
		)
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

	// TODO: Figure out expiration
	start := time.Now()
	err := r.client.Set(ctx, shortUrl, longUrl, 0).Err()
	elapsed := time.Since(start)
	r.logger.Debug("redis:query_time",
		"query", "put_short_url",
		"time_ms", elapsed/time.Millisecond,
	)

	if err != nil {
		r.logger.Error("redis:error",
			"query", "put_short_url",
			"error", err.Error(),
		)
		return err
	}

	return nil
}

func (r *RedisUrlRepo) Close() error {
	return r.client.Close()
}
