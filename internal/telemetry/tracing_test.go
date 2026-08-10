package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dsub-io/go-open-discogs-api/internal/config"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var errExporter = errors.New("exporter failed")

func TestDisabledRuntimeCreatesNoExporter(t *testing.T) {
	var logs bytes.Buffer
	runtime, err := Setup(context.Background(), config.Tracing{}, testLogger(&logs))
	if err != nil || runtime.QueryTracer() != nil || runtime.HTTPTracer() == nil {
		t.Fatalf("runtime=%+v err=%v", runtime, err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEnabledRuntimeExportsHTTPTrace(t *testing.T) {
	requests := make(chan string, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- request.URL.Path
		writer.Header().Set("Content-Type", "application/x-protobuf")
		writer.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()

	runtime, err := Setup(context.Background(), config.Tracing{
		Enabled: true, Endpoint: collector.URL + "/v1/traces", SampleRatio: 1,
	}, testLogger(&bytes.Buffer{}))
	if err != nil || runtime.QueryTracer() == nil || runtime.HTTPTracer() == nil {
		t.Fatalf("runtime=%+v err=%v", runtime, err)
	}
	handler := runtime.HTTPTracer().Wrap("/test", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/test", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case path := <-requests:
		if path != "/v1/traces" {
			t.Fatalf("collector path=%s", path)
		}
	default:
		t.Fatal("trace was not exported")
	}
}

func TestSetupRejectsInvalidEndpointAndExporterFailure(t *testing.T) {
	logger := testLogger(&bytes.Buffer{})
	_, err := Setup(context.Background(), config.Tracing{Enabled: true, Endpoint: "%invalid", SampleRatio: 1}, logger)
	if err == nil {
		t.Fatal("invalid endpoint was accepted")
	}
	factory := func(context.Context, []otlptracehttp.Option) (sdktrace.SpanExporter, error) {
		return nil, errExporter
	}
	_, err = setup(context.Background(), config.Tracing{Enabled: true, Endpoint: "https://collector.example.com", SampleRatio: 1}, logger, factory)
	if !errors.Is(err, errExporter) {
		t.Fatalf("error=%v", err)
	}
}

func TestOTelErrorHandlerLogsDeliveryFailure(t *testing.T) {
	var logs bytes.Buffer
	handler := otelErrorHandler{logger: testLogger(&logs)}
	handler.Handle(errExporter)
	if !strings.Contains(logs.String(), telemetryErrorMessage) || !strings.Contains(logs.String(), errExporter.Error()) {
		t.Fatalf("log=%s", logs.String())
	}
}

func testLogger(output *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
