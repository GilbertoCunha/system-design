package internal

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"regexp"
)

type UrlShortenerService struct {
	dbUrlRepo    UrlRepo
	cacheUrlRepo UrlRepo
	logger       *slog.Logger
}

func NewUrlShortenerService(dbRepo UrlRepo, cacheRepo UrlRepo, logger *slog.Logger) UrlShortenerService {
	return UrlShortenerService{
		dbUrlRepo:    dbRepo,
		cacheUrlRepo: cacheRepo,
		logger:       logger,
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
	for !ok {
		shortUrl = GetMD5Hash(urlToHash)
		dberr := u.dbUrlRepo.PutShortUrl(ctx, shortUrl, longUrl)

		_, collision := errors.AsType[*ShortUrlCollision](dberr)
		if dberr != nil && !collision {
			return "", dberr
		} else if dberr != nil {
			u.logger.Warn("hash collision", "hash", shortUrl, "url", urlToHash)
			urlToHash += "1" // add suffix to hash again
		} else {
			// Write to cache before closing
			err := u.cacheUrlRepo.PutShortUrl(ctx, shortUrl, longUrl)
			if err != nil {
				return "", err
			}
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

	// Retrieve longUrl from cache
	longUrl, err := u.cacheUrlRepo.GetLongUrl(ctx, shortUrl)
	if err == nil {
		return longUrl, nil
	} else if _, ok := errors.AsType[*ShortUrlNotFound](err); !ok {
		return "", err
	}

	// Retrieve longUrl from DB
	longUrl, err = u.dbUrlRepo.GetLongUrl(ctx, shortUrl)
	if err != nil {
		return "", err
	}

	return longUrl, nil
}
