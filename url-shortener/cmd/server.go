package main

import (
	"log"

	"github.com/GilbertoCunha/system-design/url-shortener/internal"
)

func main() {
	api, err := internal.NewApi()
	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(api.Run())
}
