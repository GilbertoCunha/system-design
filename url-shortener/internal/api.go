package internal

import (
	"context"
	"fmt"
	"log"
	"net/http"
)

type API struct {
	server *http.Server
	config *AppConfig
}

func (a *API) Run() error {
	log.Printf("Server started on port %v.", a.config.App.Port)
	return a.server.ListenAndServe()
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

	return &API{server: server, config: config}, nil
}
