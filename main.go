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

	"github.com/dsub-io/go-open-discogs-api/internal/app"
	"github.com/dsub-io/go-open-discogs-api/internal/buildinfo"
	"github.com/dsub-io/go-open-discogs-api/internal/config"
)

const (
	logMessageStarting = "starting OpenDiscogs API"
	logFieldVersion    = "version"
	logFieldMaxProcs   = "gomaxprocs"
	logFieldDBMaxConns = "db_max_connections"
)

type exitProcess func(int)

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
