package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/config"
	"github.com/dsub-io/go-open-discogs-api/internal/httpapi"
	"github.com/dsub-io/go-open-discogs-api/internal/management"
	"github.com/dsub-io/go-open-discogs-api/internal/observability"
	"github.com/dsub-io/go-open-discogs-api/internal/postgres"
	"github.com/dsub-io/go-open-discogs-api/internal/telemetry"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

const (
	databaseApplicationName  = "go-open-discogs-api"
	logMessageListening      = "HTTP listener started"
	logMessageShutdown       = "HTTP servers stopped"
	serverRolePublic         = "public"
	serverRoleManagement     = "management"
	connectionParameterName  = "application_name"
	databaseSearchPathName   = "search_path"
	serverErrorBufferSize    = 2
	databaseConnectionPrefix = "configure PostgreSQL"
	publicSchemaWarning      = "database schema is public; set --database-schema or API_DATABASE_SCHEMA to isolate OpenDiscogs tables"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	return run(ctx, cfg, logger, pgxpool.NewWithConfig)
}

type poolFactory func(context.Context, *pgxpool.Config) (*pgxpool.Pool, error)

func run(ctx context.Context, cfg config.Config, logger *slog.Logger, newPool poolFactory) error {
	if cfg.Database.Schema == config.DefaultDatabaseSchema {
		logger.Warn(publicSchemaWarning, "schema", cfg.Database.Schema)
	}
	telemetryRuntime, err := telemetry.Setup(ctx, cfg.Tracing, logger)
	if err != nil {
		return fmt.Errorf("configure telemetry: %w", err)
	}
	defer shutdownTelemetry(telemetryRuntime, cfg.ShutdownTimeout, logger)

	poolConfig, err := databaseConfig(cfg, telemetryRuntime)
	if err != nil {
		return err
	}
	pool, err := newPool(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	defer pool.Close()

	pingContext, cancelPing := context.WithTimeout(ctx, cfg.QueryTimeout)
	err = pool.Ping(pingContext)
	cancelPing()
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	schemaContext, cancelSchema := context.WithTimeout(ctx, cfg.QueryTimeout)
	err = postgres.ValidateSchema(schemaContext, pool, cfg.Database.Schema)
	cancelSchema()
	if err != nil {
		return fmt.Errorf("validate PostgreSQL schema: %w", err)
	}

	registry := prometheus.NewRegistry()
	if cfg.MetricsEnabled {
		registry.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			observability.NewPoolCollector(pool),
		)
	}
	metrics := observability.NewMetrics(registry, cfg.MetricsEnabled)
	repository := postgres.New(pool, cfg.ServerURL, cfg.QueryTimeout)
	publicHandler := httpapi.NewRouter(
		repository,
		cfg.CacheControl,
		cfg.AccessLog,
		logger,
		metrics,
		telemetryRuntime.HTTPTracer(),
	).Handler()
	managementHandler := management.NewHandler(repository, registry, cfg.MetricsEnabled).Routes()

	servers := []configuredServer{
		newConfiguredServer(serverRolePublic, cfg.PublicAddress, publicHandler, cfg),
		newConfiguredServer(serverRoleManagement, cfg.ManagementAddress, managementHandler, cfg),
	}
	return serve(ctx, servers, cfg.ShutdownTimeout, logger)
}

func databaseConfig(cfg config.Config, telemetryRuntime telemetry.Runtime) (*pgxpool.Config, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", databaseConnectionPrefix, err)
	}
	poolConfig.MaxConns = cfg.Database.MaxConnections
	poolConfig.MinConns = cfg.Database.MinConnections
	poolConfig.MaxConnIdleTime = cfg.Database.MaxConnectionIdle
	poolConfig.MaxConnLifetime = cfg.Database.MaxConnectionLife
	poolConfig.HealthCheckPeriod = cfg.Database.HealthCheckPeriod
	poolConfig.ConnConfig.StatementCacheCapacity = cfg.Database.StatementCacheSize
	poolConfig.ConnConfig.RuntimeParams[connectionParameterName] = databaseApplicationName
	poolConfig.ConnConfig.RuntimeParams[databaseSearchPathName] = `"` + cfg.Database.Schema + `"`
	if queryTracer := telemetryRuntime.QueryTracer(); queryTracer != nil {
		poolConfig.ConnConfig.Tracer = queryTracer
	}
	return poolConfig, nil
}

type configuredServer struct {
	role   string
	server *http.Server
}

func newConfiguredServer(role, address string, handler http.Handler, cfg config.Config) configuredServer {
	return configuredServer{role: role, server: &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}}
}

func serve(ctx context.Context, servers []configuredServer, shutdownTimeout time.Duration, logger *slog.Logger) error {
	listeners := make([]net.Listener, 0, len(servers))
	for _, configured := range servers {
		listener, err := net.Listen("tcp", configured.server.Addr)
		if err != nil {
			closeListeners(listeners)
			return fmt.Errorf("listen on %s address %s: %w", configured.role, configured.server.Addr, err)
		}
		listeners = append(listeners, listener)
		logger.Info(logMessageListening, "role", configured.role, "address", listener.Addr().String())
	}

	errorsChannel := make(chan error, serverErrorBufferSize)
	for index, configured := range servers {
		listener := listeners[index]
		go func() {
			err := configured.server.Serve(listener)
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			errorsChannel <- err
		}()
	}

	var runError error
	select {
	case <-ctx.Done():
	case runError = <-errorsChannel:
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	for _, configured := range servers {
		runError = errors.Join(runError, stopServer(shutdownContext, configured.role, configured.server))
	}
	logger.Info(logMessageShutdown)
	return runError
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

type telemetryShutdown interface {
	Shutdown(context.Context) error
}

func shutdownTelemetry(runtime telemetryShutdown, timeout time.Duration, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		logger.Warn("flush telemetry", "error", err)
	}
}

type serverShutdown interface {
	Shutdown(context.Context) error
}

func stopServer(ctx context.Context, role string, server serverShutdown) error {
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("stop %s server: %w", role, err)
	}
	return nil
}
