package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"

	"github.com/GilbertoCunha/system-design/url-shortener/internal/config"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/httpapi"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/shortener"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/storage/postgres"
	"github.com/GilbertoCunha/system-design/url-shortener/internal/storage/redis"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	env := flag.String("environment", "local", "the environment in which to run on.")
	flag.Parse()

	cfg, err := config.Load(strings.ToLower(*env))
	if err != nil {
		return fmt.Errorf("loading config for environment %q: %w", *env, err)
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.App.LogLevel)); err != nil {
		return fmt.Errorf("log level: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	setMemoryLimit(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(
		// Also go_sched_latencies_seconds: how long goroutines wait for a
		// thread before running. CPU % can look fine while this grows, as
		// when the API ran on one thread with ~2000 goroutines queued.
		// And the CPU the GC takes, next to the CPU the whole process
		// takes, for the dashboard's GC share.
		collectors.WithGoCollectorRuntimeMetrics(collectors.GoRuntimeMetricsRule{
			Matcher: regexp.MustCompile(`^/sched/latencies:seconds$|^/cpu/classes/(gc/)?total:cpu-seconds$`),
		}),
	))
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	db, err := postgres.New(ctx, cfg.Postgres, reg)
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer db.Close()

	cache, err := redis.New(cfg.Redis, reg)
	if err != nil {
		return fmt.Errorf("connecting to redis: %w", err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			logger.Error("error closing redis client", "error", err.Error())
		}
	}()

	if cfg.App.PprofAddr != "" {
		go func() {
			if err := httpapi.Run(ctx, httpapi.NewDebug(cfg.App.PprofAddr, logger), logger); err != nil {
				logger.Error("pprof server stopped", "error", err.Error())
			}
		}()
	}

	svc := shortener.New(db, cache, logger)
	return httpapi.Run(ctx, httpapi.New(cfg.App, svc, logger, reg), logger)
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
