package shortener

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
)

// Store keeps short → long URL mappings. PutShortUrl returns ErrCollision when
// the short URL already maps to a different long URL, and GetLongUrl returns
// ErrNotFound for an unknown one.
type Store interface {
	GetLongUrl(ctx context.Context, code string) (string, error)
	PutShortUrl(ctx context.Context, code, longUrl string) error
}

type Service struct {
	db, cache Store
	log       *slog.Logger
}

func New(db, cache Store, log *slog.Logger) *Service {
	return &Service{db: db, cache: cache, log: log}
}

// ShortenUrl stores longUrl in the database and the cache and returns its
// short URL
func (s *Service) ShortenUrl(ctx context.Context, longUrl string) (string, error) {
	if err := validate(longUrl); err != nil {
		return "", err
	}

	// On a collision (two long URLs with the same hash), add a suffix to the
	// long URL and hash again
	for suffix := ""; ; suffix += "1" {
		code := hash(longUrl + suffix)
		err := s.db.PutShortUrl(ctx, code, longUrl)
		if errors.Is(err, ErrCollision) {
			s.log.Warn("hash collision", "hash", code, "url", longUrl+suffix)
			continue
		}
		if err != nil {
			return "", err
		}
		s.cacheBestEffort(ctx, code, longUrl)
		return code, nil
	}
}

var codePattern = regexp.MustCompile("^[0-9a-f]{32}$")

// GetLongUrl looks the short URL up in the cache, then in the database
func (s *Service) GetLongUrl(ctx context.Context, code string) (string, error) {
	if !codePattern.MatchString(code) {
		return "", fmt.Errorf("%w: %q is not an MD5 hash", ErrInvalidCode, code)
	}

	longUrl, err := s.cache.GetLongUrl(ctx, code)
	if err == nil {
		return longUrl, nil
	}
	// Anything but a plain miss (a timeout, a lost connection) still falls
	// back to the database, but shouldn't pass unnoticed.
	if !errors.Is(err, ErrNotFound) {
		s.log.Warn("cache read failed", ErrorAttrs(err)...)
	}

	longUrl, err = s.db.GetLongUrl(ctx, code)
	if err != nil {
		return "", err
	}
	s.cacheBestEffort(ctx, code, longUrl)
	return longUrl, nil
}

// A failed cache write isn't returned: the URL is in the database, and a
// later read falls back to it.
func (s *Service) cacheBestEffort(ctx context.Context, code, longUrl string) {
	if err := s.cache.PutShortUrl(ctx, code, longUrl); err != nil {
		s.log.Warn("cache write failed", ErrorAttrs(err)...)
	}
}

func validate(longUrl string) error {
	u, err := url.ParseRequestURI(longUrl)
	switch {
	case err != nil:
		return fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	case u.Scheme != "https":
		return fmt.Errorf("%w: must use the https scheme", ErrInvalidUrl)
	case u.Host == "":
		return fmt.Errorf("%w: host must not be empty", ErrInvalidUrl)
	}
	return nil
}

func hash(text string) string {
	sum := md5.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}
