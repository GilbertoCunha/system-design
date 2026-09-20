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
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	// Fetch program flags
	environmentStr := flag.String("environment", "local", "the environment in which to run on.")
	flag.Parse()
	environment, ok := internal.EnvironmentFromString(*environmentStr)
	if !ok {
		return fmt.Errorf("unsupported environment selected: %s", *environmentStr)
	}

	// Load app config
	config, err := internal.NewAppConfig(environment)
	if err != nil {
		return fmt.Errorf("an error occurred while parsing the configuration file: %w", err)
	}

	// Configure logging
	logLevel := logLevelFromStr(config.App.LogLevel)
	logger := internal.NewLogger(os.Stdout, logLevel)
	slog.SetDefault(logger)

	// Configure and run API
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

func logLevelFromStr(logLevel string) slog.Level {
	mapper := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	return mapper[logLevel]
}
