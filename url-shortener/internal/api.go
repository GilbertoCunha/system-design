package internal

import (
	"log"
	"net/http"
)

type API struct {
	server *http.Server
}

func (a *API) Run() error {
	log.Print("Server started...")
	return a.server.ListenAndServe()
}

func NewAPI() *API {
	// Creates Services
	urlShortener := &UrlShortenerService{}

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
		Addr:    ":8080",
		Handler: mux,
	}

	return &API{server: server}
}
