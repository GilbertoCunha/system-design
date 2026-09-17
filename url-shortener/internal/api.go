package internal

import (
	"log"
	"net/http"
)

type Api struct {
	server *http.Server
}

func (a *Api) Run() error {
	log.Print("Server started...")
	return a.server.ListenAndServe()
}

func NewApi() (Api, error) {
	mux := http.NewServeMux()

	// Handler definition
	mux.HandleFunc("/", RequestHandler)

	// Server definition
	server := &http.Server{
		Addr:    ":80",
		Handler: mux,
	}

	return Api{server: server}, nil
}
