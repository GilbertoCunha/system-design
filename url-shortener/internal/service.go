package internal

import (
	"context"
	"errors"
	"log"
	"net/url"
	"regexp"
	"time"
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
func (u UrlShortenerService) ShortenUrl(ctx context.Context, longUrl string) (string, error) {
	// Validate url
	parsedUrl, err := url.ParseRequestURI(longUrl)
	if err != nil {
		return "", &InvalidUrl{url: longUrl, message: err.Error()}
	} else if parsedUrl.Scheme != "https" {
		return "", &InvalidUrl{url: longUrl, message: "Url does not use https scheme"}
	} else if parsedUrl.Host == "" {
		return "", &InvalidUrl{url: longUrl, message: "Url host must not be empty"}
	}

	// In case there is a collision in ShortUrl creation,
	// meaning two different longUrls having the same hash,
	// then simply add a suffix to the longUrl and try again
	var shortUrl string
	ok := false
	urlToHash := longUrl
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	for !ok {
		shortUrl = GetMD5Hash(urlToHash)
		_, dberr := u.dbUrlRepo.PutShortUrl(ctx, shortUrl, longUrl)

		_, collision := errors.AsType[*ShortUrlCollision](dberr)
		if dberr != nil && !collision {
			return "", dberr
		} else if dberr != nil {
			log.Printf("COLLISION for hash %s of url %s", shortUrl, urlToHash)
			urlToHash += "1" // add suffix to hash again
		} else {
			ok = true
		}
	}

	return shortUrl, nil
}

var shortUrlPattern = regexp.MustCompile("^[0-9a-f]{32}$")

// Fetched the long url for a short one
// First checks cache, queries the database if not present
// Returns long url
func (u UrlShortenerService) GetLongUrl(ctx context.Context, shortUrl string) (string, error) {
	// Validate shortUrl
	match := shortUrlPattern.MatchString(shortUrl)
	if !match {
		return "", &InvalidShortUrl{shortUrl: shortUrl}
	}

	// Retrieve longUrl
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	longUrl, err := u.dbUrlRepo.GetLongUrl(ctx, shortUrl)
	if err != nil {
		return "", err
	}

	return longUrl, nil
}
