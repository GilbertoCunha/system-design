package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestInitializeCreatesSeriesAtZero(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)
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
	metrics := NewMetrics(reg)
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

	handler := MetricsMiddleware(metrics, mux)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/url", strings.NewReader("{}")))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))

	if after := testutil.CollectAndCount(metrics.httpRequestDurationSeconds); after != before {
		t.Errorf("requests created new series: %d before, %d after", before, after)
	}
	if got := testutil.ToFloat64(metrics.httpRequestsTotal.WithLabelValues("201", "POST", "POST /v1/url")); got != 1 {
		t.Errorf("POST /v1/url 201 count = %v, want 1", got)
	}
}
