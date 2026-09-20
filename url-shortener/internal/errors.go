package internal

import (
	"fmt"
)

type Overloaded struct{}

func (e *Overloaded) Error() string {
	return "Server overloaded, try again later"
}

type InvalidUrl struct {
	url     string
	message string
}

func (e *InvalidUrl) Error() string {
	return fmt.Sprintf("ERROR InvalidUrl: %s - %s", e.url, e.message)
}

type InvalidShortUrl struct {
	shortUrl string
}

func (e *InvalidShortUrl) Error() string {
	return fmt.Sprintf("ERROR InvalidShortUrl: short url %s is not an MD5 hash.", e.shortUrl)
}

type ShortUrlCollision struct {
	longUrl1 string
	longUrl2 string
}

func (e *ShortUrlCollision) Error() string {
	return fmt.Sprintf("ERROR ShortUrlCollision: %s and %s have the same MD5 hash", e.longUrl1, e.longUrl2)
}

type ShortUrlNotFound struct {
	shortUrl string
}

func (e *ShortUrlNotFound) Error() string {
	return fmt.Sprintf("ERROR ShortUrlNotFound: %s", e.shortUrl)
}
