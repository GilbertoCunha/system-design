package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

// LimitInFlight answers 503 once max requests are already being handled.
// usage records how full the limit is as each request arrives, itself
// included: 1 for a request turned away.
func LimitInFlight(max int, usage prometheus.Observer, logger *slog.Logger) func(http.Handler) http.Handler {
	slots := make(chan struct{}, max)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			select {
			case slots <- struct{}{}:
				usage.Observe(float64(len(slots)) / float64(max))
				defer func() { <-slots }()
				next.ServeHTTP(w, req)
			default:
				usage.Observe(1)
				setUnavailableCause(w, causeInFlightLimit)
				w.Header().Set("Retry-After", "5")
				http.Error(w, "overloaded", http.StatusServiceUnavailable)
				logger.Error("overloaded", "error", "too many requests in flight")
			}
		})
	}
}
