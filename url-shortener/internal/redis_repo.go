package internal

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type RedisUrlRepo struct {
	client *redis.Client
}

func NewRedisUrlRepo(c *AppConfig) *RedisUrlRepo {
	return &RedisUrlRepo{
		client: redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%d", c.Redis.Host, c.Redis.Port),
			Password: c.Redis.Password,
			Username: c.Redis.User,
		}),
	}
}

func (r *RedisUrlRepo) GetLongUrl(ctx context.Context, shortUrl string) (string, error) {
	return "", nil
}

func (r *RedisUrlRepo) PutShortUrl(ctx context.Context, shortUrl string, longUrl string) (string, error) {
	return "", nil
}

func (r *RedisUrlRepo) Close() error {
	return r.client.Close()
}
