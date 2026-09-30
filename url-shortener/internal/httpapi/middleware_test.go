package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
)

type observations struct {
	mu     sync.Mutex
	values []float64
}

func (o *observations) Observe(v float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.values = append(o.values, v)
}

func TestLimitInFlightRecordsUsage(t *testing.T) {
	usage := &observations{}
	release := make(chan struct{})
	started := make(chan struct{})
	limit := LimitInFlight(2, usage, slog.New(slog.DiscardHandler))
	handler := limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
	}))

	// Two requests take both slots and stay in the handler.
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		}()
		<-started
	}

	// The third finds the limit full.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	close(release)
	wg.Wait()

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("third request: got %d, want 503", rec.Code)
	}
	if want := []float64{0.5, 1, 1}; !slices.Equal(usage.values, want) {
		t.Fatalf("usage observations: got %v, want %v", usage.values, want)
	}
}
