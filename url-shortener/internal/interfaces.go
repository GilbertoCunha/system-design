package internal

import (
	"context"
)

type UrlRepo interface {
	GetLongUrl(ctx context.Context, shortUrl string) (string, error)
	PutShortUrl(ctx context.Context, shortUrl string, longUrl string) error
}
