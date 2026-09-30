package shortener

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

// An in-memory Store that behaves like the real ones: a short URL keeps the
// first long URL stored under it
type memStore struct {
	urls    map[string]string
	putErr  error
	getErr  error
	puts    int
	collide map[string]bool // short URLs that already belong to another long URL
}

func newMemStore() *memStore {
	return &memStore{urls: map[string]string{}, collide: map[string]bool{}}
}

func (m *memStore) GetLongUrl(_ context.Context, code string) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	long, ok := m.urls[code]
	if !ok {
		return "", ErrNotFound
	}
	return long, nil
}

func (m *memStore) PutShortUrl(_ context.Context, code, longUrl string) error {
	m.puts++
	if m.putErr != nil {
		return m.putErr
	}
	if m.collide[code] {
		return ErrCollision
	}
	m.urls[code] = longUrl
	return nil
}

func newService() (*Service, *memStore, *memStore) {
	db, cache := newMemStore(), newMemStore()
	return New(db, cache, slog.New(slog.DiscardHandler)), db, cache
}

func TestShortenStoresInBothAndResolves(t *testing.T) {
	svc, db, cache := newService()
	code, err := svc.ShortenUrl(t.Context(), "https://example.com/a")
	if err != nil {
		t.Fatal(err)
	}
	if db.urls[code] != "https://example.com/a" || cache.urls[code] != "https://example.com/a" {
		t.Fatalf("not stored in both: db %q, cache %q", db.urls[code], cache.urls[code])
	}
	got, err := svc.GetLongUrl(t.Context(), code)
	if err != nil || got != "https://example.com/a" {
		t.Fatalf("GetLongUrl = %q, %v", got, err)
	}
}

func TestShortenRetriesOnCollision(t *testing.T) {
	svc, db, _ := newService()
	long := "https://example.com/a"
	db.collide[hash(long)] = true

	code, err := svc.ShortenUrl(t.Context(), long)
	if err != nil {
		t.Fatal(err)
	}
	if code != hash(long+"1") || db.puts != 2 {
		t.Errorf("got %s after %d puts, want %s after 2", code, db.puts, hash(long+"1"))
	}
}

func TestShortenRejectsInvalidUrls(t *testing.T) {
	svc, db, _ := newService()
	for _, u := range []string{"not a url", "http://example.com", "https://"} {
		if _, err := svc.ShortenUrl(t.Context(), u); !errors.Is(err, ErrInvalidUrl) {
			t.Errorf("%q: got %v, want ErrInvalidUrl", u, err)
		}
	}
	if db.puts != 0 {
		t.Errorf("invalid URLs reached the store")
	}
}

func TestShortenIgnoresCacheWriteFailures(t *testing.T) {
	svc, _, cache := newService()
	cache.putErr = errors.New("cache down")
	if _, err := svc.ShortenUrl(t.Context(), "https://example.com/a"); err != nil {
		t.Fatalf("cache failure surfaced: %v", err)
	}
}

func TestGetFallsBackToDatabaseAndRefillsCache(t *testing.T) {
	svc, db, cache := newService()
	code := hash("https://example.com/a")
	db.urls[code] = "https://example.com/a"
	cache.getErr = &Overloaded{Dependency: "redis", Operation: "get_long_url", Err: context.DeadlineExceeded}

	got, err := svc.GetLongUrl(t.Context(), code)
	if err != nil || got != "https://example.com/a" {
		t.Fatalf("GetLongUrl = %q, %v", got, err)
	}
	if cache.urls[code] != "https://example.com/a" {
		t.Error("cache not refilled after a database read")
	}
}

func TestGetErrors(t *testing.T) {
	svc, _, _ := newService()
	if _, err := svc.GetLongUrl(t.Context(), "nope"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("malformed code: got %v, want ErrInvalidCode", err)
	}
	if _, err := svc.GetLongUrl(t.Context(), hash("unknown")); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown code: got %v, want ErrNotFound", err)
	}
}
