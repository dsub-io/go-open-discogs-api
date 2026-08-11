package healthcheck

import (
	"context"
	"fmt"
	"net/http"
)

const (
	ManagementReadinessURL = "http://127.0.0.1:8081/readyz"
	errorCreateRequest     = "create readiness request"
	errorExecuteRequest    = "execute readiness request"
	errorUnexpectedStatus  = "readiness endpoint returned HTTP %d"
)

// Readiness verifies that the local management endpoint and PostgreSQL are ready.
func Readiness(ctx context.Context) error {
	return probe(ctx, http.DefaultClient, ManagementReadinessURL)
}

func probe(ctx context.Context, client *http.Client, endpoint string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", errorCreateRequest, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("%s: %w", errorExecuteRequest, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf(errorUnexpectedStatus, response.StatusCode)
	}
	return nil
}
