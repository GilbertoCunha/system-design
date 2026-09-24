package internal

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	httpActiveRequests         *prometheus.GaugeVec
	httpRequestsTotal          *prometheus.CounterVec
	httpRequestDurationSeconds *prometheus.HistogramVec
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
		},
			[]string{"status", "method", "route"},
		),
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
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

		elapsed := float64(time.Since(start)) / float64(time.Second)
		metrics.httpRequestDurationSeconds.With(prometheus.Labels{
			"status": status,
			"method": method,
			"route":  route,
		}).Observe(elapsed)
	})
}
