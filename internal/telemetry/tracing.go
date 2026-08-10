package telemetry

import (
	"context"
	"log/slog"
	"net/url"

	"github.com/dsub-io/go-open-discogs-api/internal/buildinfo"
	"github.com/dsub-io/go-open-discogs-api/internal/config"
	"github.com/dsub-io/go-open-discogs-api/internal/observability"
	"github.com/exaring/otelpgx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.40.0"
)

const telemetryErrorMessage = "OpenTelemetry exporter error"

type Runtime struct {
	httpTracer  observability.HTTPTracer
	queryTracer *otelpgx.Tracer
	provider    *sdktrace.TracerProvider
}

func Setup(ctx context.Context, settings config.Tracing, logger *slog.Logger) (Runtime, error) {
	return setup(ctx, settings, logger, newOTLPExporter)
}

type exporterFactory func(context.Context, []otlptracehttp.Option) (sdktrace.SpanExporter, error)

func setup(ctx context.Context, settings config.Tracing, logger *slog.Logger, factory exporterFactory) (Runtime, error) {
	if !settings.Enabled {
		return Runtime{httpTracer: observability.NoopHTTPTracer{}}, nil
	}
	endpoint, err := url.Parse(settings.Endpoint)
	if err != nil {
		return Runtime{}, err
	}
	options := []otlptracehttp.Option{otlptracehttp.WithEndpointURL(settings.Endpoint)}
	if endpoint.Scheme == "http" {
		options = append(options, otlptracehttp.WithInsecure())
	}
	exporter, err := factory(ctx, options)
	if err != nil {
		return Runtime{}, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(settings.SampleRatio))),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ApplicationName),
			semconv.ServiceVersion(buildinfo.Version),
		)),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	otel.SetErrorHandler(otelErrorHandler{logger: logger})
	return Runtime{
		httpTracer: observability.NewOpenTelemetryHTTPTracer(provider),
		queryTracer: otelpgx.NewTracer(
			otelpgx.WithTracerProvider(provider),
			otelpgx.WithDisableConnectionDetailsInAttributes(),
			otelpgx.WithDisableSQLStatementInAttributes(),
			otelpgx.WithTrimSQLInSpanName(),
		),
		provider: provider,
	}, nil
}

func newOTLPExporter(ctx context.Context, options []otlptracehttp.Option) (sdktrace.SpanExporter, error) {
	return otlptracehttp.New(ctx, options...)
}

func (r Runtime) HTTPTracer() observability.HTTPTracer { return r.httpTracer }

func (r Runtime) QueryTracer() *otelpgx.Tracer { return r.queryTracer }

func (r Runtime) Shutdown(ctx context.Context) error {
	if r.provider == nil {
		return nil
	}
	return r.provider.Shutdown(ctx)
}

type otelErrorHandler struct {
	logger *slog.Logger
}

func (h otelErrorHandler) Handle(err error) {
	h.logger.Warn(telemetryErrorMessage, "error", err)
}
