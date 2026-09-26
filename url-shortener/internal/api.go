package internal

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Embed static HTML file
//
//go:embed web/index.html
var indexHTML []byte

type API struct {
	server    *http.Server
	config    *AppConfig
	pgRepo    *PgUrlRepo
	redisRepo *RedisUrlRepo
	logger    *slog.Logger
}

func (a *API) Run(ctx context.Context) error {
	errChn := make(chan error, 1)

	go func() {
		a.logger.Info("Server started", "port", a.config.App.Port)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChn <- err
		}
	}()

	select {
	case err := <-errChn:
		return err
	case <-ctx.Done():
		a.logger.Info("Shutdown signal received.")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return a.server.Shutdown(shutdownCtx)
}

func (a *API) Close() error {
	a.pgRepo.Close()
	return a.redisRepo.Close()
}

func NewAPI(ctx context.Context, config *AppConfig, logger *slog.Logger) (*API, error) {
	// Create metrics
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(
		// Also go_sched_latencies_seconds: how long goroutines wait for a
		// thread before running. CPU % can look fine while this grows, as
		// when the API ran on one thread with ~2000 goroutines queued.
		collectors.WithGoCollectorRuntimeMetrics(collectors.GoRuntimeMetricsRule{
			Matcher: regexp.MustCompile(`^/sched/latencies:seconds$`),
		}),
	))
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	metrics := NewMetrics(reg)

	// Creates Repositories
	pgRepo, err := NewPgUrlRepo(ctx, config, logger, reg)
	if err != nil {
		return nil, err
	}
	redisRepo, err := NewRedisUrlRepo(config, logger, reg)
	if err != nil {
		return nil, err
	}

	// Creates Services
	urlShortener := NewUrlShortenerService(
		pgRepo,
		redisRepo,
		logger,
	)

	// Handler definition
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /v1/url/{code}", func(w http.ResponseWriter, r *http.Request) {
		GetLongUrlHandler(w, r, urlShortener, logger)
	})
	mux.HandleFunc("POST /v1/url", func(w http.ResponseWriter, r *http.Request) {
		CreateShortUrlHandler(w, r, urlShortener, logger)
	})

	// Every status each handler above can answer with; keep in step with the
	// handlers. Their metric series start at zero instead of appearing with
	// their first request.
	metrics.Initialize(map[string][]int{
		"/metrics":           {http.StatusOK},
		"GET /{$}":           {http.StatusOK},
		"GET /healthz":       {http.StatusOK},
		"GET /v1/url/{code}": {http.StatusFound, http.StatusBadRequest, http.StatusNotFound, 499, http.StatusInternalServerError, http.StatusServiceUnavailable},
		"POST /v1/url":       {http.StatusCreated, http.StatusBadRequest, 499, http.StatusInternalServerError, http.StatusServiceUnavailable},
	})

	// Server definition
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.App.Port),
		Handler:           MetricsMiddleware(metrics, mux),
		ReadHeaderTimeout: time.Duration(config.App.ReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout:       time.Duration(config.App.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(config.App.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(config.App.IdleTimeoutSeconds) * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	return &API{
		server:    server,
		config:    config,
		pgRepo:    pgRepo,
		redisRepo: redisRepo,
		logger:    logger,
	}, nil
}
