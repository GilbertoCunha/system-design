package main

import (
	"flag"
	"log"

	"github.com/GilbertoCunha/system-design/url-shortener/internal"
)

func main() {
	environmentStr := flag.String("environment", "local", "the environment in which to run on.")
	environment, ok := internal.EnvironmentFromString(*environmentStr)
	if !ok {
		log.Fatal("Unsupported environment selected: ", environmentStr)
	}

	config, err := internal.NewAppConfig(environment)
	if err != nil {
		log.Fatal("An error occurred while parsing the configuration file: ", err)
	}

	api := internal.NewAPI(config)
	log.Fatal(api.Run())
}
