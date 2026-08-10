package main

import (
	"bytes"
	"errors"
	"io"
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
)

const (
	envTestDatabaseURL     = "TEST_DATABASE_URL"
	defaultTestDatabaseURL = "postgres://discogs:discogs@127.0.0.1:55432/discogs?sslmode=disable"
	testListenerMessage    = "HTTP listener started"
)

var errWriter = errors.New("writer failure")

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
