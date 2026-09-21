package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisUrlRepo struct {
	client *redis.Client
	logger *slog.Logger
}

func NewRedisUrlRepo(c *AppConfig, logger *slog.Logger) *RedisUrlRepo {
	return &RedisUrlRepo{
		client: redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", c.Redis.Host, c.Redis.Port),
			Password: c.Redis.Password,
			Username: c.Redis.User,
		}),
		logger: logger,
	}
}

// TODO: Error handling of context timeouts
func (r *RedisUrlRepo) GetLongUrl(ctx context.Context, shortUrl string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
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

// TODO: Error handling of context timeouts
func (r *RedisUrlRepo) PutShortUrl(ctx context.Context, shortUrl string, longUrl string) error {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
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
