package management

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

var errUnready = errors.New("database unavailable")

func TestManagementHealthAndMetricsRoutes(t *testing.T) {
	t.Parallel()
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_metric", Help: "Test metric."}))
	handler := NewHandler(checkerStub{}, registry, true).Routes()
	tests := []struct {
		path     string
		contains string
	}{
		{RouteHealth, `"status":"UP"`},
		{RouteReadiness, `"status":"UP"`},
		{RouteActuatorHealth, `"status":"UP"`},
		{RouteMetrics, "test_metric"},
		{RouteActuatorPrometheus, "test_metric"},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.contains) {
			t.Fatalf("path=%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestManagementReadinessFailureAndDisabledMetrics(t *testing.T) {
	t.Parallel()
	handler := NewHandler(checkerStub{err: errUnready}, prometheus.NewRegistry(), false).Routes()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, RouteReadiness, nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), StatusDown) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, RouteMetrics, nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("metrics status=%d", response.Code)
	}
}

type checkerStub struct {
	err error
}

func (c checkerStub) Ready(context.Context) error { return c.err }
