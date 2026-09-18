package internal

type UrlRepo interface {
	GetLongUrl(shortUrl string) (string, error)
	PutShortUrl(longUrl string) (string, error)
}
