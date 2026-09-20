package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

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
	redisRepo := NewRedisUrlRepo(config)

	// Creates Services
	urlShortener := NewUrlShortenerService(
		pgRepo,
		redisRepo,
		logger,
	)

	// Handler definition
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /api/v1/url/{code}", func(w http.ResponseWriter, r *http.Request) {
		GetLongUrlHandler(w, r, urlShortener, logger)
	})
	mux.HandleFunc("POST /api/v1/url", func(w http.ResponseWriter, r *http.Request) {
		CreateShortUrlHandler(w, r, urlShortener, logger)
	})

	// Server definition
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.App.Port),
		Handler:           mux,
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
