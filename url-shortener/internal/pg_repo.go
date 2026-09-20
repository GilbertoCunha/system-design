package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/database"
	pgx "github.com/jackc/pgx/v5"
	pgxpool "github.com/jackc/pgx/v5/pgxpool"
)

type PgUrlRepo struct {
	pool    *pgxpool.Pool
	queries *database.Queries
	logger  *slog.Logger
}

func NewPgUrlRepo(ctx context.Context, c *AppConfig, logger *slog.Logger) (*PgUrlRepo, error) {
	dsn := fmt.Sprintf(
		"postgres://%v:%v@%v:%v/%v",
		c.Postgres.User,
		c.Postgres.Password,
		c.Postgres.Host,
		c.Postgres.Port,
		c.Postgres.DbName,
	)
	config, err := pgxpool.ParseConfig(dsn)
	config.MaxConns = int32(c.Postgres.Pool.MaxConns)
	config.MinConns = int32(c.Postgres.Pool.MinConns)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	// Start background process for connection pool statistics gathering
	go getPoolStats(ctx, pool, logger, 5)

	return &PgUrlRepo{pool: pool, queries: database.New(pool), logger: logger}, nil
}

func (r *PgUrlRepo) GetLongUrl(ctx context.Context, shortUrl string) (string, error) {
	start := time.Now()
	longUrl, err := r.queries.GetLongUrl(ctx, shortUrl)
	elapsed := time.Since(start)
	r.logger.Debug("query:GetLongUrl",
		"time_ms", elapsed/time.Millisecond,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", &ShortUrlNotFound{shortUrl: shortUrl}
		}
		return "", err
	}
	return longUrl, nil
}

func (r *PgUrlRepo) PutShortUrl(ctx context.Context, shortUrl string, longUrl string) (string, error) {
	start := time.Now()
	queryLongUrl, err := r.queries.PutShortUrl(
		ctx,
		database.PutShortUrlParams{ShortUrl: shortUrl, LongUrl: longUrl},
	)
	elapsed := time.Since(start)
	r.logger.Debug("query:PutShortUrl",
		"time_ms", elapsed/time.Millisecond,
	)

	if err != nil {
		return "", err
	}

	// If queryLongUrl is returned, a collision happened
	// 1. If it's the same as longUrl, then this URL has already been shortened
	// 2. If it's a different longUrl, then an actual collision occurred
	if queryLongUrl != longUrl {
		return "", &ShortUrlCollision{longUrl1: longUrl, longUrl2: queryLongUrl}
	}

	return shortUrl, nil
}

func (r *PgUrlRepo) Close() {
	r.pool.Close()
}

func getPoolStats(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, frequencySeconds int) {
	ticker := time.NewTicker(time.Duration(frequencySeconds) * time.Second)
	defer ticker.Stop()
	var prev *pgxpool.Stat
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cur := pool.Stat()
			if prev != nil {
				acquires := cur.AcquireCount() - prev.AcquireCount()
				waited := cur.AcquireDuration() - prev.AcquireDuration()
				if acquires > 0 {
					logger.Info("pool",
						"avg_wait_ms", waited/(time.Duration(acquires)*time.Millisecond),
						"empty_acquires", cur.EmptyAcquireCount()-prev.EmptyAcquireCount(),
						"in_use", cur.AcquiredConns(),
						"idle", cur.IdleConns(),
						"total", cur.TotalConns(),
						"max", cur.MaxConns(),
					)
				}
			}
			prev = cur
		}
	}
}
