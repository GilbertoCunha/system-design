package internal

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type PgUrlRepo struct {
	conn *pgx.Conn
}

func NewPgUrlRepo(ctx context.Context, c *AppConfig) (*PgUrlRepo, error) {
	conn, err := pgx.Connect(
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
	return &PgUrlRepo{conn: conn}, nil
}

func (r *PgUrlRepo) GetLongUrl(shortUrl string) (string, error) {
	return "", nil
}

func (r *PgUrlRepo) PutShortUrl(shortUrl string, longUrl string) (string, error) {
	return "", nil
}
