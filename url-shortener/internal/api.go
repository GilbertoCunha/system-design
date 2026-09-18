package internal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type API struct {
	server    *http.Server
	config    *AppConfig
	pgRepo    *PgUrlRepo
	redisRepo *RedisUrlRepo
}

func (a *API) Run(ctx context.Context) error {
	errChn := make(chan error, 1)

	go func() {
		log.Printf("Server started on port %v.", a.config.App.Port)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChn <- err
		}
	}()

	select {
	case err := <-errChn:
		return err
	case <-ctx.Done():
		log.Println("Shutdown signal received.")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return a.server.Shutdown(shutdownCtx)
}

func (a *API) Close() error {
	a.pgRepo.Close()
	return a.redisRepo.Close()
}

func NewAPI(ctx context.Context, config *AppConfig) (*API, error) {
	// Creates Repositories
	pgRepo, err := NewPgUrlRepo(ctx, config)
	if err != nil {
		return nil, err
	}
	redisRepo := NewRedisUrlRepo(config)

	// Creates Services
	urlShortener := NewUrlShortenerService(
		pgRepo,
		redisRepo,
	)

	// Handler definition
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/url/{code}", func(w http.ResponseWriter, r *http.Request) {
		GetLongUrlHandler(w, r, urlShortener)
	})
	mux.HandleFunc("POST /api/v1/url", func(w http.ResponseWriter, r *http.Request) {
		CreateShortUrlHandler(w, r, urlShortener)
	})

	// Server definition
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.App.Port),
		Handler:           mux,
		ReadHeaderTimeout: time.Duration(config.App.ReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout:       time.Duration(config.App.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(config.App.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(config.App.IdleTimeoutSeconds) * time.Second,
	}

	return &API{
		server:    server,
		config:    config,
		pgRepo:    pgRepo,
		redisRepo: redisRepo,
	}, nil
}
