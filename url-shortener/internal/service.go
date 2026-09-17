package internal

type UrlShortenerService struct{}

// Shortens a URL, persists it into both database and cache
// and returns the short url
func (u *UrlShortenerService) ShortenUrl(longUrl string) (string, error) {
	return "fakeShort", nil
}

// Fetched the long url for a short one
// First checks cache, queries the database if not present
// Returns long url
func (u *UrlShortenerService) GetLongUrl(shortUrl string) (string, error) {
	return "fakeLong", nil
}
