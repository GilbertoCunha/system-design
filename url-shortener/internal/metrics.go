package internal

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	httpActiveRequests         *prometheus.GaugeVec
	httpRequestsTotal          *prometheus.CounterVec
	httpRequestDurationSeconds *prometheus.HistogramVec
	httpUnavailableTotal       *prometheus.CounterVec
}

// Why a request was answered 503. Written onto the response by whatever
// sends the 503 (see setUnavailableCause), and counted by MetricsMiddleware.
const (
	// LimitInFlight turned it away: too many requests were already running.
	causeInFlightLimit = "in_flight_limit"
	// Sent without naming a cause.
	causeUnknown = "unknown"
)

// The causes a 503 can have in practice, for Initialize. Redis timeouts are
// not among them: reads fall back to Postgres and cache writes are only
// logged, so they never become a 503.
var unavailableCauses = []string{
	causeInFlightLimit,
	"postgres_acquire", // waiting for a pool connection timed out
	"postgres_query",   // a query timed out
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	return &Metrics{
		httpActiveRequests: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Name: "http_active_requests",
			Help: "Number of currently active requests",
		},
			[]string{"method", "route"},
		),
		httpRequestsTotal: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of http requests",
		},
			[]string{"status", "method", "route"},
		),
		httpRequestDurationSeconds: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "Duration of http requests in seconds",
			// Most requests finish in a few milliseconds, where the default
			// buckets have a single 5ms bucket that the p50 can only guess
			// inside. Fine steps up to 250ms where the tail lives, edges at the
			// load tests' p99 targets (100ms, 500ms), and nothing past the 5s
			// write timeout.
			Buckets: []float64{
				.001, .0025, .005, .0075, .01, .015, .025, .05, .075,
				.1, .15, .2, .25, .5, 1, 2.5, 5,
			},
		},
			[]string{"status", "method", "route"},
		),
		httpUnavailableTotal: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "http_unavailable_total",
			Help: "503 responses, by what caused them: the in-flight limit, or the dependency that timed out and whether waiting for a connection or running the call",
		},
			[]string{"method", "route", "cause"},
		),
	}
}

// Initialize creates the series for every route and status the API expects,
// at zero, before any traffic arrives. Series are otherwise created by their
// first request, and the first scrape of a new series already holds whatever
// happened until then: rate() takes that sample as its starting point, so a
// burst right after a deploy never shows up in rates or quantiles. (A load
// test's slowest first 15 seconds read as p99 23ms, not 381ms.)
//
// routes maps a mux pattern to the statuses its handler can answer with. The
// method is the pattern's own, or GET for patterns without one.
func (m *Metrics) Initialize(routes map[string][]int) {
	for route, statuses := range routes {
		method := "GET"
		if before, _, ok := strings.Cut(route, " "); ok {
			method = before
		}
		m.httpActiveRequests.WithLabelValues(method, route)
		for _, code := range statuses {
			status := strconv.Itoa(code)
			m.httpRequestsTotal.WithLabelValues(status, method, route)
			m.httpRequestDurationSeconds.WithLabelValues(status, method, route)
			if code == http.StatusServiceUnavailable {
				for _, cause := range unavailableCauses {
					m.httpUnavailableTotal.WithLabelValues(method, route, cause)
				}
			}
		}
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	cause      string // why it was a 503, when it was one
}

// Records why a response is a 503, for http_unavailable_total. w is the
// writer MetricsMiddleware hands down; anything else is ignored, so handlers
// behave the same without the middleware (in tests, say).
func setUnavailableCause(w http.ResponseWriter, cause string) {
	if rec, ok := w.(*statusRecorder); ok {
		rec.cause = cause
	}
}

func (w *statusRecorder) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func MetricsMiddleware(metrics *Metrics, mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Active requests
		method := r.Method
		_, route := mux.Handler(r)
		metrics.httpActiveRequests.With(prometheus.Labels{
			"method": method,
			"route":  route,
		}).Inc()
		defer metrics.httpActiveRequests.With(prometheus.Labels{
			"method": method,
			"route":  route,
		}).Dec()

		// Send request to next handler
		rw := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		mux.ServeHTTP(rw, r)
		status := strconv.Itoa(rw.statusCode)

		// Increase total requests
		metrics.httpRequestsTotal.With(prometheus.Labels{
			"status": status,
			"method": method,
			"route":  route,
		}).Inc()

		if rw.statusCode == http.StatusServiceUnavailable {
			cause := rw.cause
			if cause == "" {
				cause = causeUnknown
			}
			metrics.httpUnavailableTotal.With(prometheus.Labels{
				"method": method,
				"route":  route,
				"cause":  cause,
			}).Inc()
		}

		elapsed := float64(time.Since(start)) / float64(time.Second)
		metrics.httpRequestDurationSeconds.With(prometheus.Labels{
			"status": status,
			"method": method,
			"route":  route,
		}).Observe(elapsed)
	})
}
