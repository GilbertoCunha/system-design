package httpapi

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/config"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/shortener"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed web/index.html
var indexHTML []byte

// New builds the API's HTTP server. reg is both where the HTTP metrics are
// registered and what /metrics serves.
func New(cfg config.App, svc *shortener.Service, log *slog.Logger, reg *prometheus.Registry) *http.Server {
	h := &handlers{svc: svc, log: log}
	// The limit from the config, so dashboards draw it as a limit instead of
	// hard-coding a value that changes here
	promauto.With(reg).NewGauge(prometheus.GaugeOpts{
		Name: "http_in_flight_limit",
		Help: "Requests the API handles at once before answering 503, from the config",
	}).Set(float64(cfg.MaxInFlight))
	// How full the limit is, seen by every request as it arrives. Bursts that
	// fill it between two scrapes show here; an average of in-flight requests
	// smooths them away, and a gauge only sees the moment of the scrape.
	usage := promauto.With(reg).NewHistogram(prometheus.HistogramOpts{
		Name:    "http_in_flight_limit_usage_ratio",
		Help:    "Share of the in-flight limit in use when a request arrived. 1 means full: the request was answered 503",
		Buckets: []float64{.1, .2, .3, .4, .5, .6, .7, .8, .9, .95, .99, 1},
	})
	limit := LimitInFlight(cfg.MaxInFlight, usage, log)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /{$}", serveIndex)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {})
	mux.Handle("GET /v1/url/{code}", limit(http.HandlerFunc(h.getLongUrl)))
	mux.Handle("POST /v1/url", limit(http.HandlerFunc(h.createShortUrl)))

	// Every status each handler above can answer with; keep in step with the
	// handlers. Their metric series start at zero instead of appearing with
	// their first request.
	m := newMetrics(reg)
	m.Initialize(map[string][]int{
		"/metrics":           {http.StatusOK},
		"GET /{$}":           {http.StatusOK},
		"GET /healthz":       {http.StatusOK},
		"GET /v1/url/{code}": {http.StatusFound, http.StatusBadRequest, http.StatusNotFound, 499, http.StatusInternalServerError, http.StatusServiceUnavailable},
		"POST /v1/url":       {http.StatusCreated, http.StatusBadRequest, 499, http.StatusInternalServerError, http.StatusServiceUnavailable},
	})

	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           metricsMiddleware(m, mux),
		Protocols:         &protocols,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}
}

// NewDebug builds the pprof server. It listens on its own port, which no
// Service exposes, so the gateway never reaches it; Pyroscope's Alloy scrapes it
// on the pod's address. It has no write timeout because a CPU profile streams
// for as long as it records (30s by default).
func NewDebug(addr string, log *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}
}

// Run serves until ctx is done, then shuts down gracefully
func Run(ctx context.Context, srv *http.Server, log *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("Server started", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("Shutdown signal received.")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}
