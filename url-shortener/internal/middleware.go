package internal

import (
	"net/http"
)

func LimitInFlight(max int) func(http.Handler) http.Handler {
	slots := make(chan struct{}, max)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				next.ServeHTTP(w, req)
			default:
				w.Header().Set("Retry-After", "5")
				http.Error(w, "overloaded", http.StatusServiceUnavailable)
			}
		})
	}
}
