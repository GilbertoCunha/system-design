package httpapi

import (
	"log/slog"
	"net/http"
)

func LimitInFlight(max int, logger *slog.Logger) func(http.Handler) http.Handler {
	slots := make(chan struct{}, max)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				next.ServeHTTP(w, req)
			default:
				setUnavailableCause(w, causeInFlightLimit)
				w.Header().Set("Retry-After", "5")
				http.Error(w, "overloaded", http.StatusServiceUnavailable)
				logger.Error("overloaded", "error", "too many requests in flight")
			}
		})
	}
}
