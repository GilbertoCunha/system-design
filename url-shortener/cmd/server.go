package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
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
	setMemoryLimit(logger)

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

// Go's GC paces itself on heap growth alone and knows nothing of the
// container's memory limit, so under load it lets memory run past it and the
// kernel kills the process. MEMORY_LIMIT_BYTES carries that limit (see the
// Deployment), and the GC is told to keep Go's memory under 90% of it: the
// rest is for what the runtime doesn't account for. An explicit GOMEMLIMIT
// wins, since the runtime has already applied it.
func setMemoryLimit(logger *slog.Logger) {
	if os.Getenv("GOMEMLIMIT") != "" {
		return
	}
	limit, err := strconv.ParseInt(os.Getenv("MEMORY_LIMIT_BYTES"), 10, 64)
	if err != nil || limit <= 0 {
		return
	}
	goLimit := limit / 10 * 9
	debug.SetMemoryLimit(goLimit)
	logger.Info("Go memory limit set", "bytes", goLimit, "container_limit_bytes", limit)
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
