package internal

type UrlRepo interface {
	GetLongUrl(shortUrl string) (string, error)
	PutShortUrl(shortUrl string, longUrl string) (string, error)
}
