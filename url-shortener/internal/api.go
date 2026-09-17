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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		RequestHandler(w, r, urlShortener)
	})

	// Server definition
	server := &http.Server{
		Addr:    ":80",
		Handler: mux,
	}

	return &API{server: server}
}
