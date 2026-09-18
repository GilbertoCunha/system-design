package internal

import (
	"errors"
	"log"
	"net/url"
	"regexp"
)

type UrlShortenerService struct {
	dbUrlRepo    UrlRepo
	cacheUrlRepo UrlRepo
}

func NewUrlShortenerService(dbRepo UrlRepo, cacheRepo UrlRepo) UrlShortenerService {
	return UrlShortenerService{
		dbUrlRepo:    dbRepo,
		cacheUrlRepo: cacheRepo,
	}
}

// Shortens a URL, persists it into both database and cache
// and returns the short url
func (u UrlShortenerService) ShortenUrl(longUrl string) (string, error) {
	// Validate url
	_, err := url.ParseRequestURI(longUrl)
	if err != nil {
		return "", &InvalidUrlError{url: longUrl, message: err.Error()}
	}

	// In case there is a collision in ShortUrl creation,
	// meaning two different longUrls having the same hash,
	// then simply add a suffix to the longUrl and try again
	var shortUrl string
	ok := false
	urlToHash := longUrl
	for !ok {
		shortUrl = GetMD5Hash(longUrl)
		_, dberr := u.dbUrlRepo.PutShortUrl(shortUrl, longUrl)

		_, collision := errors.AsType[*ShortUrlCollision](dberr)
		if !collision {
			ok = true
		} else {
			log.Printf("COLLISION for hash %s", shortUrl)
			urlToHash += "1" // add suffix to hash again
		}
	}

	return shortUrl, nil
}

var shortUrlPattern = regexp.MustCompile("^[0-9a-f]{32}$")

// Fetched the long url for a short one
// First checks cache, queries the database if not present
// Returns long url
func (u UrlShortenerService) GetLongUrl(shortUrl string) (string, error) {
	// Validate shortUrl
	match := shortUrlPattern.MatchString(shortUrl)
	if !match {
		return "", &InvalidShortUrl{shortUrl: shortUrl}
	}

	// Retrieve longUrl
	longUrl, err := u.dbUrlRepo.GetLongUrl(shortUrl)
	if err != nil {
		return "", err
	}

	return longUrl, nil
}
