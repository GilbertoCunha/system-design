package internal

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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
	// Creates Repositories
	pgRepo, err := NewPgUrlRepo(ctx, config, logger)
	if err != nil {
		return nil, err
	}
	redisRepo, err := NewRedisUrlRepo(config, logger)
	if err != nil {
		return nil, err
	}

	// Creates Services
	urlShortener := NewUrlShortenerService(
		pgRepo,
		redisRepo,
		logger,
	)

	// Create middleware
	reg := prometheus.NewRegistry()
	metrics := NewMetrics(reg)

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
