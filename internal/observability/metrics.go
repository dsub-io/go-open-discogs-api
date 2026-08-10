package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

const (
	metricNamespace = "open_discogs"
	metricSubsystem = "http"
)

type Metrics struct {
	enabled  bool
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight *prometheus.GaugeVec
}

func NewMetrics(registry *prometheus.Registry, enabled bool) *Metrics {
	metrics := &Metrics{enabled: enabled}
	if !enabled {
		return metrics
	}
	metrics.requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricNamespace, Subsystem: metricSubsystem, Name: "requests_total", Help: "Completed HTTP requests.",
	}, []string{"route", "method", "status"})
	metrics.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricNamespace, Subsystem: metricSubsystem, Name: "request_duration_seconds", Help: "HTTP request duration.",
		Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"route", "method", "status"})
	metrics.inFlight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricNamespace, Subsystem: metricSubsystem, Name: "in_flight_requests", Help: "Current HTTP requests.",
	}, []string{"route"})
	registry.MustRegister(metrics.requests, metrics.duration, metrics.inFlight)
	return metrics
}

func (m *Metrics) RequestStarted(route string) {
	if m.enabled {
		m.inFlight.WithLabelValues(route).Inc()
	}
}

func (m *Metrics) RequestFinished(route, method string, status int, started time.Time) {
	if !m.enabled {
		return
	}
	m.inFlight.WithLabelValues(route).Dec()
	statusText := strconv.Itoa(status)
	m.requests.WithLabelValues(route, method, statusText).Inc()
	m.duration.WithLabelValues(route, method, statusText).Observe(time.Since(started).Seconds())
}

type HTTPTracer interface {
	Wrap(route string, handler http.Handler) http.Handler
}

type NoopHTTPTracer struct{}

func (NoopHTTPTracer) Wrap(_ string, handler http.Handler) http.Handler { return handler }

type OpenTelemetryHTTPTracer struct {
	provider trace.TracerProvider
}

func NewOpenTelemetryHTTPTracer(provider trace.TracerProvider) OpenTelemetryHTTPTracer {
	return OpenTelemetryHTTPTracer{provider: provider}
}

func (t OpenTelemetryHTTPTracer) Wrap(route string, handler http.Handler) http.Handler {
	return otelhttp.NewHandler(handler, route,
		otelhttp.WithTracerProvider(t.provider),
		otelhttp.WithSpanNameFormatter(func(_ string, _ *http.Request) string { return route }),
	)
}

type PoolCollector struct {
	pool       *pgxpool.Pool
	acquired   *prometheus.Desc
	idle       *prometheus.Desc
	max        *prometheus.Desc
	constructs *prometheus.Desc
}

func NewPoolCollector(pool *pgxpool.Pool) *PoolCollector {
	return &PoolCollector{
		pool:       pool,
		acquired:   prometheus.NewDesc("open_discogs_database_acquired_connections", "Acquired PostgreSQL connections.", nil, nil),
		idle:       prometheus.NewDesc("open_discogs_database_idle_connections", "Idle PostgreSQL connections.", nil, nil),
		max:        prometheus.NewDesc("open_discogs_database_max_connections", "Configured PostgreSQL connection limit.", nil, nil),
		constructs: prometheus.NewDesc("open_discogs_database_new_connections_total", "PostgreSQL connections constructed by the pool.", nil, nil),
	}
}

func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquired
	ch <- c.idle
	ch <- c.max
	ch <- c.constructs
}

func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(stat.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(stat.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.max, prometheus.GaugeValue, float64(stat.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.constructs, prometheus.CounterValue, float64(stat.NewConnsCount()))
}
