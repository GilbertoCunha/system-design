package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/database"
	pgx "github.com/jackc/pgx/v5"
	pgxpool "github.com/jackc/pgx/v5/pgxpool"
)

type PgUrlRepo struct {
	pool    *pgxpool.Pool
	queries *database.Queries
}

func NewPgUrlRepo(ctx context.Context, c *AppConfig) (*PgUrlRepo, error) {
	pool, err := pgxpool.New(
		ctx,
		fmt.Sprintf(
			"postgres://%v:%v@%v:%v/%v",
			c.Postgres.User,
			c.Postgres.Password,
			c.Postgres.Host,
			c.Postgres.Port,
			c.Postgres.DbName,
		),
	)
	if err != nil {
		return nil, err
	}

	return &PgUrlRepo{pool: pool, queries: database.New(pool)}, nil
}

func (r *PgUrlRepo) GetLongUrl(ctx context.Context, shortUrl string) (string, error) {
	longUrl, err := r.queries.GetLongUrl(ctx, shortUrl)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", &ShortUrlNotFound{shortUrl: shortUrl}
		}
		return "", err
	}
	return longUrl, nil
}

func (r *PgUrlRepo) PutShortUrl(ctx context.Context, shortUrl string, longUrl string) (string, error) {
	queryLongUrl, err := r.queries.PutShortUrl(
		ctx,
		database.PutShortUrlParams{ShortUrl: shortUrl, LongUrl: longUrl},
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
