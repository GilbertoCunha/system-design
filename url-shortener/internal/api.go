package internal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
)

type API struct {
	server    *http.Server
	config    *AppConfig
	pgRepo    *PgUrlRepo
	redisRepo *RedisUrlRepo
}

func (a *API) Run() error {
	log.Printf("Server started on port %v.", a.config.App.Port)
	return a.server.ListenAndServe()
}

func (a *API) Close() error {
	return errors.Join(a.pgRepo.Close(), a.redisRepo.Close())
}

func NewAPI(ctx *context.Context, config *AppConfig) (*API, error) {
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
	mux.HandleFunc("GET /{code}", func(w http.ResponseWriter, r *http.Request) {
		shortUrl := r.PathValue("code")
		GetLongUrlHandler(w, r, urlShortener, shortUrl)
	})
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		CreateShortUrlHandler(w, r, urlShortener)
	})

	// Server definition
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.App.Port),
		Handler: mux,
	}

	return &API{
		server:    server,
		config:    config,
		pgRepo:    pgRepo,
		redisRepo: redisRepo,
	}, nil
}
