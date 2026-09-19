package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/GilbertoCunha/system-design/url-shortener/internal"
)

func main() {
	logger := internal.NewLogger(os.Stdout, slog.LevelInfo)
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	environmentStr := flag.String("environment", "local", "the environment in which to run on.")
	flag.Parse()

	environment, ok := internal.EnvironmentFromString(*environmentStr)
	if !ok {
		return fmt.Errorf("unsupported environment selected: %s", *environmentStr)
	}

	config, err := internal.NewAppConfig(environment)
	if err != nil {
		return fmt.Errorf("an error occurred while parsing the configuration file: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	api, err := internal.NewAPI(ctx, config, logger)
	if err != nil {
		return fmt.Errorf("an error occurred when creating the API: %w", err)
	}
	defer func() {
		if cerr := api.Close(); cerr != nil {
			logger.Error("error closing API resources", "err", cerr)
		}
	}()

	return api.Run(ctx)
}
