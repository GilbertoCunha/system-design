package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/shortener"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestInitializeCreatesSeriesAtZero(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := newMetrics(reg)
	metrics.Initialize(map[string][]int{
		"/metrics":     {200},
		"POST /v1/url": {201, 503},
	})

	for _, labels := range [][]string{
		{"200", "GET", "/metrics"},
		{"201", "POST", "POST /v1/url"},
		{"503", "POST", "POST /v1/url"},
	} {
		got := testutil.ToFloat64(metrics.httpRequestsTotal.WithLabelValues(labels...))
		if got != 0 {
			t.Errorf("http_requests_total%v = %v, want 0", labels, got)
		}
	}
	if n := testutil.CollectAndCount(metrics.httpRequestDurationSeconds); n != 3 {
		t.Errorf("duration histogram has %d series, want 3", n)
	}
}

// A real request has to land in a series Initialize created. If the route or
// method labels drift from what the middleware records, it creates a new
// series instead, and the first scrape of that one hides its burst again.
func TestRequestsUseInitializedSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := newMetrics(reg)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/url", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {})
	metrics.Initialize(map[string][]int{
		"POST /v1/url": {201},
		"GET /healthz": {200},
	})
	before := testutil.CollectAndCount(metrics.httpRequestDurationSeconds)

	handler := metricsMiddleware(metrics, mux)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/url", strings.NewReader("{}")))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))

	if after := testutil.CollectAndCount(metrics.httpRequestDurationSeconds); after != before {
		t.Errorf("requests created new series: %d before, %d after", before, after)
	}
	if got := testutil.ToFloat64(metrics.httpRequestsTotal.WithLabelValues("201", "POST", "POST /v1/url")); got != 1 {
		t.Errorf("POST /v1/url 201 count = %v, want 1", got)
	}
}

// Every 503 is counted under what caused it, whether the limiter sent it or
// a handler did for a dependency that timed out, and those series exist at
// zero from the start like the rest.
func TestUnavailableCauses(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := newMetrics(reg)
	logger := slog.New(slog.DiscardHandler)
	mux := http.NewServeMux()
	// No slots at all: the limiter turns every request away.
	mux.Handle("GET /limited", LimitInFlight(0, &observations{}, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	h := &handlers{log: logger}
	mux.HandleFunc("GET /acquire", func(w http.ResponseWriter, r *http.Request) {
		h.writeError(w, &shortener.Overloaded{Dependency: "postgres", Operation: "acquire_conn", Err: context.DeadlineExceeded})
	})
	mux.HandleFunc("GET /query", func(w http.ResponseWriter, r *http.Request) {
		h.writeError(w, &shortener.Overloaded{Dependency: "postgres", Operation: "put_short_url", Err: context.DeadlineExceeded})
	})
	mux.HandleFunc("GET /bare", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	metrics.Initialize(map[string][]int{"GET /limited": {200, 503}})

	for _, cause := range unavailableCauses {
		if got := testutil.ToFloat64(metrics.httpUnavailableTotal.WithLabelValues("GET", "GET /limited", cause)); got != 0 {
			t.Errorf("GET /limited %s initialized at %v, want 0", cause, got)
		}
	}

	handler := metricsMiddleware(metrics, mux)
	for _, path := range []string{"/limited", "/acquire", "/query", "/bare"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s answered %d, want 503", path, rec.Code)
		}
	}

	for _, tc := range []struct{ route, cause string }{
		{"GET /limited", causeInFlightLimit},
		{"GET /acquire", "postgres_acquire"},
		{"GET /query", "postgres_query"},
		{"GET /bare", causeUnknown},
	} {
		if got := testutil.ToFloat64(metrics.httpUnavailableTotal.WithLabelValues("GET", tc.route, tc.cause)); got != 1 {
			t.Errorf("%s %s = %v, want 1", tc.route, tc.cause, got)
		}
	}
}
