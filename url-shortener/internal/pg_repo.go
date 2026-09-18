package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/database"
	"github.com/jackc/pgx/v5"
)

type PgUrlRepo struct {
	ctx     *context.Context
	conn    *pgx.Conn
	queries *database.Queries
}

func NewPgUrlRepo(ctx *context.Context, c *AppConfig) (*PgUrlRepo, error) {
	conn, err := pgx.Connect(
		*ctx,
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

	return &PgUrlRepo{ctx: ctx, conn: conn, queries: database.New(conn)}, nil
}

func (r *PgUrlRepo) GetLongUrl(shortUrl string) (string, error) {
	longUrl, err := r.queries.GetLongUrl(*r.ctx, shortUrl)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", &ShortUrlNotFound{shortUrl: shortUrl}
		}
		return "", err
	}
	return longUrl, nil
}

func (r *PgUrlRepo) PutShortUrl(shortUrl string, longUrl string) (string, error) {
	queryLongUrl, err := r.queries.PutShortUrl(
		*r.ctx,
		database.PutShortUrlParams{ShortUrl: shortUrl, LongUrl: longUrl},
	)

	// In case of collision, no rows are returned by the query
	// all other errors are unexpected
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	// If queryLongUrl is returned, a collision happened
	// 1. If it's the same as longUrl, then this URL has already been shortened
	// 2. If it's a different longUrl, then an actual collision occurred
	if queryLongUrl == longUrl {
		return "", &ShortUrlCollision{longUrl1: longUrl, longUrl2: queryLongUrl}
	}

	return shortUrl, nil
}

func (r *PgUrlRepo) Close() error {
	return r.conn.Close(*r.ctx)
}
