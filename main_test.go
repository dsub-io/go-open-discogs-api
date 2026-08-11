package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/buildinfo"
	canonicalschema "github.com/dsub-io/open-discogs-model/schema"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	envTestDatabaseURL              = "TEST_DATABASE_URL"
	defaultTestDatabaseURL          = "postgres://discogs:discogs@127.0.0.1:55432/discogs?sslmode=disable"
	testListenerMessage             = "HTTP listener started"
	testSchemaMigrationLockID int64 = 7_803_151_124
)

var (
	errProbe  = errors.New("probe failure")
	errWriter = errors.New("writer failure")
)

func TestRunControlPaths(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := run([]string{"--version"}, emptyEnvironment, &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != buildinfo.Version {
		t.Fatalf("version output=%q", output.String())
	}
	if err := run([]string{"--help"}, emptyEnvironment, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	probeCalled := false
	if err := runWithReadinessProbe(
		[]string{"--healthcheck"},
		emptyEnvironment,
		io.Discard,
		io.Discard,
		func(ctx context.Context) error {
			probeCalled = true
			if _, hasDeadline := ctx.Deadline(); !hasDeadline {
				t.Fatal("readiness context has no deadline")
			}
			return nil
		},
	); err != nil || !probeCalled {
		t.Fatalf("healthcheck called=%t err=%v", probeCalled, err)
	}
	if err := runWithReadinessProbe(
		[]string{"--healthcheck"},
		emptyEnvironment,
		io.Discard,
		io.Discard,
		func(context.Context) error { return errProbe },
	); !errors.Is(err, errProbe) {
		t.Fatalf("healthcheck error=%v", err)
	}
	if err := run([]string{"--unknown"}, emptyEnvironment, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "load configuration") {
		t.Fatalf("configuration error=%v", err)
	}
	if err := run([]string{"--version"}, emptyEnvironment, failingOutput{}, io.Discard); !errors.Is(err, errWriter) {
		t.Fatalf("version writer error=%v", err)
	}
}

func TestRunAppliesLimitsAndStopsOnSignal(t *testing.T) {
	databaseURL := os.Getenv(envTestDatabaseURL)
	if databaseURL == "" {
		databaseURL = defaultTestDatabaseURL
	}
	ensureMainDatabase(t, databaseURL)
	previousProcs := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(previousProcs)
	previousMemoryLimit := debug.SetMemoryLimit(math.MaxInt64)
	defer debug.SetMemoryLimit(previousMemoryLimit)

	output := newMainSignalWriter(testListenerMessage)
	done := make(chan error, 1)
	arguments := []string{
		"--address=127.0.0.1:0", "--management-address=127.0.0.1:0", "--server-url=http://127.0.0.1:8080",
		"--database-url=" + databaseURL, "--max-procs=1", "--memory-limit-mib=64", "--metrics-enabled=false",
	}
	go func() {
		done <- run(arguments, emptyEnvironment, output, io.Discard)
	}()
	select {
	case <-output.signal:
	case <-time.After(5 * time.Second):
		t.Fatal("application did not start")
	}
	if runtime.GOMAXPROCS(0) != 1 {
		t.Fatalf("GOMAXPROCS=%d", runtime.GOMAXPROCS(0))
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("application did not stop")
	}
}

func ensureMainDatabase(t *testing.T, databaseURL string) {
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

func TestMainWithExitReportsFailure(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	exitCode := 0
	mainWithExit([]string{"--unknown"}, emptyEnvironment, io.Discard, &stderr, func(code int) {
		exitCode = code
	})
	if exitCode != 1 || !strings.Contains(stderr.String(), "load configuration") {
		t.Fatalf("exit=%d stderr=%s", exitCode, stderr.String())
	}
}

func TestMainSuccess(t *testing.T) {
	previousArguments := os.Args
	os.Args = []string{"go-open-discogs-api", "--version"}
	defer func() { os.Args = previousArguments }()
	main()
}

func emptyEnvironment(string) (string, bool) { return "", false }

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, errWriter }

type mainSignalWriter struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	pattern string
	signal  chan struct{}
	once    sync.Once
}

func newMainSignalWriter(pattern string) *mainSignalWriter {
	return &mainSignalWriter{pattern: pattern, signal: make(chan struct{})}
}

func (w *mainSignalWriter) Write(content []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	written, err := w.buffer.Write(content)
	if strings.Contains(string(content), w.pattern) {
		w.once.Do(func() { close(w.signal) })
	}
	return written, err
}
