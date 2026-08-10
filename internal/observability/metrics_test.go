package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace/noop"
)

const testRoute = "/test/{id}"

func TestMetricsEnabledAndDisabled(t *testing.T) {
	t.Parallel()
	disabled := NewMetrics(prometheus.NewRegistry(), false)
	disabled.RequestStarted(testRoute)
	disabled.RequestFinished(testRoute, http.MethodGet, http.StatusOK, time.Now())

	registry := prometheus.NewRegistry()
	enabled := NewMetrics(registry, true)
	enabled.RequestStarted(testRoute)
	enabled.RequestFinished(testRoute, http.MethodGet, http.StatusCreated, time.Now().Add(-time.Millisecond))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 3 {
		t.Fatalf("metric families=%d", len(families))
	}
}

func TestHTTPTracersPreserveHandler(t *testing.T) {
	t.Parallel()
	base := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	tracers := []HTTPTracer{
		NoopHTTPTracer{},
		NewOpenTelemetryHTTPTracer(noop.NewTracerProvider()),
	}
	for _, tracer := range tracers {
		response := httptest.NewRecorder()
		tracer.Wrap(testRoute, base).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/test/1", nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("status=%d", response.Code)
		}
	}
}

func TestPoolCollectorReportsPoolState(t *testing.T) {
	t.Parallel()
	pool, err := pgxpool.New(context.Background(), "postgres://reader:secret@127.0.0.1:1/discogs?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	collector := NewPoolCollector(pool)
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 4 {
		t.Fatalf("metric families=%d", len(families))
	}
}
