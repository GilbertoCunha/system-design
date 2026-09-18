package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/GilbertoCunha/system-design/url-shortener/internal"
)

func main() {
	if err := run(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func run() error {
	environmentStr := flag.String("environment", "local", "the environment in which to run on.")
	environment, ok := internal.EnvironmentFromString(*environmentStr)
	if !ok {
		return fmt.Errorf("unsupported environment selected: %s", *environmentStr)
	}

	config, err := internal.NewAppConfig(environment)
	if err != nil {
		return fmt.Errorf("an error occurred while parsing the configuration file: %w", err)
	}

	ctx := context.Background()
	api, err := internal.NewAPI(&ctx, config)
	if err != nil {
		return fmt.Errorf("an error occurred when creating the API: %w", err)
	}
	defer func() {
		if cerr := api.Close(); cerr != nil {
			log.Println("error closing API resources:", cerr)
		}
	}()

	return api.Run()
}
