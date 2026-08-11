package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/app"
	"github.com/dsub-io/go-open-discogs-api/internal/buildinfo"
	"github.com/dsub-io/go-open-discogs-api/internal/config"
	"github.com/dsub-io/go-open-discogs-api/internal/healthcheck"
)

const (
	logMessageStarting = "starting OpenDiscogs API"
	logFieldVersion    = "version"
	logFieldMaxProcs   = "gomaxprocs"
	logFieldDBMaxConns = "db_max_connections"
	readinessTimeout   = 2 * time.Second
)

type exitProcess func(int)
type readinessProbe func(context.Context) error

func main() {
	mainWithExit(os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr, os.Exit)
}

func mainWithExit(arguments []string, lookup config.LookupEnv, stdout, stderr io.Writer, exit exitProcess) {
	if err := run(arguments, lookup, stdout, stderr); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		exit(1)
	}
}

func run(arguments []string, lookup config.LookupEnv, stdout, stderr io.Writer) error {
	return runWithReadinessProbe(arguments, lookup, stdout, stderr, healthcheck.Readiness)
}

func runWithReadinessProbe(
	arguments []string,
	lookup config.LookupEnv,
	stdout, stderr io.Writer,
	probe readinessProbe,
) error {
	result, err := config.Load(arguments, lookup, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("load configuration: %w", err)
	}
	if result.ShowVersion {
		_, err := fmt.Fprintln(stdout, buildinfo.Version)
		return err
	}
	if result.CheckReadiness {
		ctx, cancel := context.WithTimeout(context.Background(), readinessTimeout)
		defer cancel()
		return probe(ctx)
	}
	cfg := result.Config
	if cfg.MaxProcs > 0 {
		runtime.GOMAXPROCS(cfg.MaxProcs)
	}
	if cfg.MemoryLimitBytes > 0 {
		debug.SetMemoryLimit(cfg.MemoryLimitBytes)
	}

	logger := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	logger.Info(logMessageStarting,
		logFieldVersion, buildinfo.Version,
		logFieldMaxProcs, runtime.GOMAXPROCS(0),
		logFieldDBMaxConns, cfg.Database.MaxConnections,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return app.Run(ctx, cfg, logger)
}
