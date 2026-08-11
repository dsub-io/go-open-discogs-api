package app

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/config"
	"github.com/dsub-io/go-open-discogs-api/internal/telemetry"
	canonicalschema "github.com/dsub-io/open-discogs-model/schema"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	envTestDatabaseURL              = "TEST_DATABASE_URL"
	defaultTestDatabaseURL          = "postgres://discogs:discogs@127.0.0.1:55432/discogs?sslmode=disable"
	testSchemaMigrationLockID int64 = 7_803_151_124
)

func TestRunStartsAndStopsBothListeners(t *testing.T) {
	databaseURL := os.Getenv(envTestDatabaseURL)
	if databaseURL == "" {
		databaseURL = defaultTestDatabaseURL
	}
	ensureAppDatabase(t, databaseURL)
	output := newSignalWriter(logMessageListening)
	logger := slog.New(slog.NewJSONHandler(output, nil))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, testConfig(databaseURL), logger)
	}()

	select {
	case <-output.signal:
		cancel()
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("listeners did not start")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("application did not stop")
	}
	if !strings.Contains(output.String(), logMessageShutdown) {
		t.Fatalf("shutdown log missing: %s", output.String())
	}
	if !strings.Contains(output.String(), publicSchemaWarning) {
		t.Fatalf("public schema warning missing: %s", output.String())
	}
	if !strings.Contains(output.String(), `"level":"WARN"`) {
		t.Fatalf("public schema warning has wrong level: %s", output.String())
	}
	missingSchemaConfig := testConfig(databaseURL)
	missingSchemaConfig.Database.Schema = "missing_schema"
	if err := Run(context.Background(), missingSchemaConfig, logger); err == nil || !strings.Contains(err.Error(), "validate PostgreSQL schema") {
		t.Fatalf("missing schema error=%v", err)
	}
}

func TestRunReportsSetupAndDatabaseFailures(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	cfg := testConfig(defaultTestDatabaseURL)
	cfg.Tracing = config.Tracing{Enabled: true, Endpoint: "%invalid", SampleRatio: 1}
	if err := Run(context.Background(), cfg, logger); err == nil || !strings.Contains(err.Error(), "configure telemetry") {
		t.Fatalf("telemetry error=%v", err)
	}
	cfg = testConfig("not-a-database-url")
	if err := Run(context.Background(), cfg, logger); err == nil || !strings.Contains(err.Error(), databaseConnectionPrefix) {
		t.Fatalf("database config error=%v", err)
	}
	cfg = testConfig(defaultTestDatabaseURL)
	newPool := func(context.Context, *pgxpool.Config) (*pgxpool.Pool, error) {
		return nil, errTestApp
	}
	if err := run(context.Background(), cfg, logger, newPool); !errors.Is(err, errTestApp) {
		t.Fatalf("pool creation error=%v", err)
	}
	cfg = testConfig("postgres://reader:secret@127.0.0.1:1/discogs?sslmode=disable&connect_timeout=1")
	cfg.QueryTimeout = 20 * time.Millisecond
	if err := Run(context.Background(), cfg, logger); err == nil || !strings.Contains(err.Error(), "connect to PostgreSQL") {
		t.Fatalf("database connection error=%v", err)
	}
}

func TestDatabaseAndServerConfiguration(t *testing.T) {
	t.Parallel()
	cfg := testConfig(defaultTestDatabaseURL)
	runtime, err := telemetry.Setup(context.Background(), config.Tracing{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	poolConfig, err := databaseConfig(cfg, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if poolConfig.MaxConns != cfg.Database.MaxConnections || poolConfig.MinConns != cfg.Database.MinConnections || poolConfig.ConnConfig.StatementCacheCapacity != cfg.Database.StatementCacheSize {
		t.Fatalf("pool config=%+v", poolConfig)
	}
	if poolConfig.ConnConfig.RuntimeParams[connectionParameterName] != databaseApplicationName ||
		poolConfig.ConnConfig.RuntimeParams[databaseSearchPathName] != `"public"` ||
		poolConfig.ConnConfig.Tracer != nil {
		t.Fatalf("connection config=%+v", poolConfig.ConnConfig)
	}

	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	configured := newConfiguredServer(serverRolePublic, "127.0.0.1:0", handler, cfg)
	if configured.role != serverRolePublic || configured.server.Handler == nil || configured.server.ReadHeaderTimeout != cfg.ReadHeaderTimeout || configured.server.IdleTimeout != cfg.IdleTimeout {
		t.Fatalf("server=%+v", configured)
	}
}

func TestDatabaseConfigurationIncludesTracingWhenEnabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	runtime, err := telemetry.Setup(context.Background(), config.Tracing{
		Enabled: true, Endpoint: "http://127.0.0.1:4318/v1/traces", SampleRatio: 0,
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Shutdown(context.Background())
	poolConfig, err := databaseConfig(testConfig(defaultTestDatabaseURL), runtime)
	if err != nil {
		t.Fatal(err)
	}
	if poolConfig.ConnConfig.Tracer == nil {
		t.Fatal("query tracer was not configured")
	}
}

func TestServeRejectsAddressConflictAndClosesEarlierListeners(t *testing.T) {
	t.Parallel()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	servers := []configuredServer{
		{role: serverRolePublic, server: &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()}},
		{role: serverRoleManagement, server: &http.Server{Addr: occupied.Addr().String(), Handler: http.NotFoundHandler()}},
	}
	err = serve(context.Background(), servers, time.Second, logger)
	if err == nil || !strings.Contains(err.Error(), serverRoleManagement) {
		t.Fatalf("serve error=%v", err)
	}
}

func TestServeStopsOnContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	servers := []configuredServer{{
		role:   serverRolePublic,
		server: &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()},
	}}
	if err := serve(ctx, servers, time.Second, logger); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownFailuresAreReported(t *testing.T) {
	t.Parallel()
	loggerOutput := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(loggerOutput, nil))
	shutdownTelemetry(shutdownStub{err: errTestApp}, time.Second, logger)
	if !strings.Contains(loggerOutput.String(), "flush telemetry") {
		t.Fatalf("telemetry error was not logged: %s", loggerOutput.String())
	}
	err := stopServer(context.Background(), serverRolePublic, shutdownStub{err: errTestApp})
	if !errors.Is(err, errTestApp) || !strings.Contains(err.Error(), serverRolePublic) {
		t.Fatalf("server shutdown error=%v", err)
	}
	if err := stopServer(context.Background(), serverRolePublic, shutdownStub{}); err != nil {
		t.Fatal(err)
	}
}

func testConfig(databaseURL string) config.Config {
	return config.Config{
		PublicAddress: "127.0.0.1:0", ManagementAddress: "127.0.0.1:0", ServerURL: "http://127.0.0.1:8080",
		CacheControl: "no-store", ReadHeaderTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second,
		IdleTimeout: time.Second, ShutdownTimeout: time.Second, QueryTimeout: time.Second, MetricsEnabled: true,
		Database: config.Database{
			URL: databaseURL, Schema: config.DefaultDatabaseSchema,
			MaxConnections: 2, MinConnections: 0, MaxConnectionIdle: time.Minute,
			MaxConnectionLife: time.Minute, HealthCheckPeriod: time.Minute, StatementCacheSize: 16,
		},
	}
}

func ensureAppDatabase(t *testing.T, databaseURL string) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", testSchemaMigrationLockID); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass('public.artist') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return
	}
	migrations, err := canonicalschema.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	files, err := fs.ReadDir(migrations, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		migration, readErr := fs.ReadFile(migrations, file.Name())
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err := tx.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply %s: %v", file.Name(), err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

type signalWriter struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	pattern string
	signal  chan struct{}
	once    sync.Once
}

type shutdownStub struct {
	err error
}

func (s shutdownStub) Shutdown(context.Context) error { return s.err }

var errTestApp = errors.New("test app failure")

func newSignalWriter(pattern string) *signalWriter {
	return &signalWriter{pattern: pattern, signal: make(chan struct{})}
}

func (w *signalWriter) Write(content []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	written, err := w.buffer.Write(content)
	if strings.Contains(string(content), w.pattern) {
		w.once.Do(func() { close(w.signal) })
	}
	return written, err
}

func (w *signalWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}
