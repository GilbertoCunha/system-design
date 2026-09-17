package main

import (
	"log"

	"github.com/GilbertoCunha/system-design/url-shortener/internal"
)

func main() {
	api := internal.NewAPI()
	log.Fatal(api.Run())
}
