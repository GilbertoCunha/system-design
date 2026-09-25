package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

type Overloaded struct {
	msg string
}

func (e *Overloaded) Error() string {
	return "Server overloaded: " + e.msg
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

func HttpErrorHandler(w http.ResponseWriter, err error, logger *slog.Logger) bool {
	if _, ok := errors.AsType[*Overloaded](err); ok {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(503)
		return true
	} else if errors.Is(err, context.Canceled) {
		w.WriteHeader(499)
		return true
	} else if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return true
	}
	return false
}
