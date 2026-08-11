package healthcheck

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const testEndpoint = "http://management.test/readyz"

var errTransport = errors.New("transport failure")

func TestProbeMapsReadinessResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		endpoint   string
		transport  roundTripFunc
		wantError  error
		wantDetail string
	}{
		{
			name:      "ready",
			endpoint:  testEndpoint,
			transport: responseTransport(http.StatusOK),
		},
		{
			name:       "not ready",
			endpoint:   testEndpoint,
			transport:  responseTransport(http.StatusServiceUnavailable),
			wantDetail: "HTTP 503",
		},
		{
			name:      "transport failure",
			endpoint:  testEndpoint,
			transport: errorTransport(errTransport),
			wantError: errTransport,
		},
		{
			name:       "invalid endpoint",
			endpoint:   "://invalid",
			transport:  responseTransport(http.StatusOK),
			wantDetail: errorCreateRequest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &http.Client{Transport: test.transport}
			err := probe(context.Background(), client, test.endpoint)
			if test.wantError == nil && test.wantDetail == "" && err != nil {
				t.Fatal(err)
			}
			if test.wantError != nil && !errors.Is(err, test.wantError) {
				t.Fatalf("error=%v, want %v", err, test.wantError)
			}
			if test.wantDetail != "" && (err == nil || !strings.Contains(err.Error(), test.wantDetail)) {
				t.Fatalf("error=%v, want detail %q", err, test.wantDetail)
			}
		})
	}
}

func TestReadinessUsesCanonicalEndpoint(t *testing.T) {
	t.Parallel()
	if ManagementReadinessURL != "http://127.0.0.1:8081/readyz" {
		t.Fatalf("management readiness URL=%q", ManagementReadinessURL)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Readiness(ctx); err == nil {
		t.Fatal("canceled readiness probe succeeded")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func responseTransport(status int) roundTripFunc {
	return func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.String() != testEndpoint {
			return nil, errors.New("unexpected request")
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader("status")),
			Header:     make(http.Header),
		}, nil
	}
}

func errorTransport(err error) roundTripFunc {
	return func(*http.Request) (*http.Response, error) {
		return nil, err
	}
}
