package management

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	RouteHealth             = "/healthz"
	RouteReadiness          = "/readyz"
	RouteActuatorHealth     = "/actuator/health"
	RouteMetrics            = "/metrics"
	RouteActuatorPrometheus = "/actuator/prometheus"

	MethodGet               = "GET"
	HeaderContentType       = "Content-Type"
	ContentTypeJSON         = "application/json"
	StatusUp                = "UP"
	StatusDown              = "DOWN"
	DefaultReadinessTimeout = 2 * time.Second
)

type HealthChecker interface {
	Ping(context.Context) error
}

type Handler struct {
	checker          HealthChecker
	metricsEnabled   bool
	readinessTimeout time.Duration
	registry         *prometheus.Registry
}

func NewHandler(checker HealthChecker, registry *prometheus.Registry, metricsEnabled bool) *Handler {
	return &Handler{
		checker:          checker,
		metricsEnabled:   metricsEnabled,
		readinessTimeout: DefaultReadinessTimeout,
		registry:         registry,
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(MethodGet+" "+RouteHealth, h.liveness)
	mux.HandleFunc(MethodGet+" "+RouteReadiness, h.readiness)
	mux.HandleFunc(MethodGet+" "+RouteActuatorHealth, h.readiness)
	if h.metricsEnabled {
		metricsHandler := promhttp.HandlerFor(h.registry, promhttp.HandlerOpts{})
		mux.Handle(MethodGet+" "+RouteMetrics, metricsHandler)
		mux.Handle(MethodGet+" "+RouteActuatorPrometheus, metricsHandler)
	}
	return mux
}

func (h *Handler) liveness(writer http.ResponseWriter, _ *http.Request) {
	h.writeHealth(writer, http.StatusOK, StatusUp)
}

func (h *Handler) readiness(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), h.readinessTimeout)
	defer cancel()
	if err := h.checker.Ping(ctx); err != nil {
		h.writeHealth(writer, http.StatusServiceUnavailable, StatusDown)
		return
	}
	h.writeHealth(writer, http.StatusOK, StatusUp)
}

func (h *Handler) writeHealth(writer http.ResponseWriter, statusCode int, status string) {
	writer.Header().Set(HeaderContentType, ContentTypeJSON)
	writer.WriteHeader(statusCode)
	_ = json.NewEncoder(writer).Encode(healthDocument{Status: status})
}

type healthDocument struct {
	Status string `json:"status"`
}
