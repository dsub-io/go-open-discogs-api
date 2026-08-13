//go:build ignore

// Command api-performance-load runs bounded HTTP load scenarios against a
// running OpenDiscogs API and verifies that every public data operation in the
// OpenAPI document is represented.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL             = "http://127.0.0.1:18080"
	defaultOpenAPIPath         = "internal/httpapi/openapi.json"
	defaultOutputPath          = "api-performance-results.json"
	defaultRequestsPerScenario = 200
	defaultWarmupRequests      = 20
	defaultConcurrency         = 4
	defaultRequestTimeout      = 10 * time.Second
	defaultArtistRows          = 50_000
	defaultLabelRows           = 20_000
	defaultMasterRows          = 50_000
	defaultReleaseRows         = 200_000
	defaultP99RegressionLimit  = 20.0
	defaultP99RegressionMinMS  = 1.0
	expectedStatusCode         = http.StatusOK
	httpMethodGet              = "GET"
	systemTag                  = "system"
	outputFilePermissions      = 0o600
)

type openAPIDocument struct {
	Paths map[string]openAPIPath `json:"paths"`
}

type openAPIPath struct {
	Get *openAPIOperation `json:"get"`
}

type openAPIOperation struct {
	OperationID string   `json:"operationId"`
	Tags        []string `json:"tags"`
}

type scenario struct {
	Name        string
	OperationID string
	Path        string
	Target      string
}

type configuration struct {
	BaseURL             string
	OpenAPIPath         string
	OutputPath          string
	RequestsPerScenario int
	WarmupRequests      int
	Concurrency         int
	RequestTimeout      time.Duration
	ArtistRows          int
	LabelRows           int
	MasterRows          int
	ReleaseRows         int
	BaselinePath        string
	P99RegressionLimit  float64
	P99RegressionMinMS  float64
	ValidateOnly        bool
}

type sample struct {
	Duration time.Duration
	Bytes    int64
	Status   int
	Err      error
}

type scenarioResult struct {
	Name                string  `json:"name"`
	OperationID         string  `json:"operation_id"`
	Target              string  `json:"target"`
	Requests            int     `json:"requests"`
	Succeeded           int     `json:"succeeded"`
	Failed              int     `json:"failed"`
	ElapsedMilliseconds float64 `json:"elapsed_ms"`
	P50Milliseconds     float64 `json:"p50_ms"`
	P95Milliseconds     float64 `json:"p95_ms"`
	P99Milliseconds     float64 `json:"p99_ms"`
	RequestsPerSecond   float64 `json:"requests_per_second"`
	ResponseBytes       int64   `json:"response_bytes"`
	FirstError          string  `json:"first_error,omitempty"`
}

type report struct {
	GeneratedAt         time.Time        `json:"generated_at"`
	BaseURL             string           `json:"base_url"`
	RequestsPerScenario int              `json:"requests_per_scenario"`
	WarmupRequests      int              `json:"warmup_requests"`
	Concurrency         int              `json:"concurrency"`
	ArtistRows          int              `json:"artist_rows"`
	LabelRows           int              `json:"label_rows"`
	MasterRows          int              `json:"master_rows"`
	ReleaseRows         int              `json:"release_rows"`
	Results             []scenarioResult `json:"results"`
}

func performanceScenarios(cfg configuration) []scenario {
	return []scenario{
		{Name: "artists-page-deep", OperationID: "searchArtists", Path: "/artists", Target: fmt.Sprintf("/artists?after_id=%d&size=30", cfg.ArtistRows-1_000)},
		{Name: "artists-search-name", OperationID: "searchArtists", Path: "/artists", Target: "/artists?name=Artist%2049&size=30"},
		{Name: "artists-search-real-name", OperationID: "searchArtists", Path: "/artists", Target: "/artists?real_name=Real%20Artist%2049&size=30"},
		{Name: "artist-detail-rich", OperationID: "getArtist", Path: "/artists/{id}", Target: "/artists/1"},
		{Name: "artist-releases-deep", OperationID: "getArtistReleases", Path: "/artists/{id}/releases", Target: fmt.Sprintf("/artists/1/releases?after_id=%d&size=30", cfg.ReleaseRows/2)},
		{Name: "labels-search-name", OperationID: "searchLabels", Path: "/labels", Target: "/labels?name=Label%2019&size=30"},
		{Name: "label-detail-rich", OperationID: "getLabel", Path: "/labels/{id}", Target: "/labels/1"},
		{Name: "label-releases-deep", OperationID: "getLabelReleases", Path: "/labels/{id}/releases", Target: fmt.Sprintf("/labels/1/releases?after_id=%d&size=30", cfg.ReleaseRows/2)},
		{Name: "masters-search-title", OperationID: "searchMasters", Path: "/masters", Target: "/masters?title=Master%2049&size=30"},
		{Name: "masters-search-year", OperationID: "searchMasters", Path: "/masters", Target: "/masters?year=2000&size=30"},
		{Name: "master-detail-rich", OperationID: "getMaster", Path: "/masters/{id}", Target: "/masters/1"},
		{Name: "master-releases-deep", OperationID: "getMasterReleases", Path: "/masters/{id}/releases", Target: fmt.Sprintf("/masters/1/releases?after_id=%d&size=30", cfg.ReleaseRows/2)},
		{Name: "releases-page-deep", OperationID: "searchReleases", Path: "/releases", Target: fmt.Sprintf("/releases?after_id=%d&size=30", cfg.ReleaseRows-1_000)},
		{Name: "releases-search-title", OperationID: "searchReleases", Path: "/releases", Target: "/releases?title=rare%20needle&size=30"},
		{Name: "releases-search-combined", OperationID: "searchReleases", Path: "/releases", Target: "/releases?country=GB&year=2000&month=3&master=true&size=30"},
		{Name: "release-detail-rich", OperationID: "getRelease", Path: "/releases/{id}", Target: "/releases/1"},
	}
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfiguration(arguments, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	benchmarkScenarios := performanceScenarios(cfg)
	if err := validateInventory(cfg.OpenAPIPath, benchmarkScenarios); err != nil {
		return err
	}
	if cfg.ValidateOnly {
		fmt.Fprintln(stdout, "OpenAPI performance scenario inventory is complete.")
		return nil
	}
	client := newHTTPClient(cfg)
	results := make([]scenarioResult, 0, len(benchmarkScenarios))
	fmt.Fprintln(stdout, "scenario\trequests\tfailures\tp50_ms\tp95_ms\tp99_ms\trps\tresponse_mib")
	for _, testScenario := range benchmarkScenarios {
		if err := executeWarmup(client, cfg, testScenario); err != nil {
			return fmt.Errorf("warm up %s: %w", testScenario.Name, err)
		}
		result := executeScenario(client, cfg, testScenario)
		results = append(results, result)
		fmt.Fprintf(
			stdout,
			"%s\t%d\t%d\t%.3f\t%.3f\t%.3f\t%.1f\t%.3f\n",
			result.Name,
			result.Requests,
			result.Failed,
			result.P50Milliseconds,
			result.P95Milliseconds,
			result.P99Milliseconds,
			result.RequestsPerSecond,
			float64(result.ResponseBytes)/(1024*1024),
		)
	}
	benchmarkReport := report{
		GeneratedAt:         time.Now().UTC(),
		BaseURL:             cfg.BaseURL,
		RequestsPerScenario: cfg.RequestsPerScenario,
		WarmupRequests:      cfg.WarmupRequests,
		Concurrency:         cfg.Concurrency,
		ArtistRows:          cfg.ArtistRows,
		LabelRows:           cfg.LabelRows,
		MasterRows:          cfg.MasterRows,
		ReleaseRows:         cfg.ReleaseRows,
		Results:             results,
	}
	if err := writeReport(cfg.OutputPath, benchmarkReport); err != nil {
		return err
	}
	if cfg.BaselinePath != "" {
		if err := compareBaseline(stdout, cfg, benchmarkReport); err != nil {
			return err
		}
	}
	for _, result := range results {
		if result.Failed > 0 {
			return fmt.Errorf("scenario %s failed %d requests: %s", result.Name, result.Failed, result.FirstError)
		}
	}
	return nil
}

func parseConfiguration(arguments []string, output io.Writer) (configuration, error) {
	cfg := configuration{}
	flags := flag.NewFlagSet("api-performance-load", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&cfg.BaseURL, "base-url", defaultBaseURL, "OpenDiscogs public API base URL")
	flags.StringVar(&cfg.OpenAPIPath, "openapi", defaultOpenAPIPath, "OpenAPI document path")
	flags.StringVar(&cfg.OutputPath, "output", defaultOutputPath, "JSON result path")
	flags.IntVar(&cfg.RequestsPerScenario, "requests", defaultRequestsPerScenario, "measured requests per scenario")
	flags.IntVar(&cfg.WarmupRequests, "warmup", defaultWarmupRequests, "warm-up requests per scenario")
	flags.IntVar(&cfg.Concurrency, "concurrency", defaultConcurrency, "concurrent requests per scenario")
	flags.DurationVar(&cfg.RequestTimeout, "timeout", defaultRequestTimeout, "individual request timeout")
	flags.IntVar(&cfg.ArtistRows, "artist-rows", defaultArtistRows, "artist rows in the deterministic fixture")
	flags.IntVar(&cfg.LabelRows, "label-rows", defaultLabelRows, "label rows in the deterministic fixture")
	flags.IntVar(&cfg.MasterRows, "master-rows", defaultMasterRows, "master rows in the deterministic fixture")
	flags.IntVar(&cfg.ReleaseRows, "release-rows", defaultReleaseRows, "release rows in the deterministic fixture")
	flags.StringVar(&cfg.BaselinePath, "baseline", "", "optional JSON baseline used for a p99 regression gate")
	flags.Float64Var(&cfg.P99RegressionLimit, "max-p99-regression-percent", defaultP99RegressionLimit, "maximum allowed p99 increase from the baseline")
	flags.Float64Var(&cfg.P99RegressionMinMS, "min-p99-regression-ms", defaultP99RegressionMinMS, "minimum absolute p99 increase considered a regression")
	flags.BoolVar(&cfg.ValidateOnly, "validate-only", false, "validate OpenAPI scenario coverage without issuing requests")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return configuration{}, flag.ErrHelp
		}
		return configuration{}, err
	}
	if flags.NArg() != 0 {
		return configuration{}, fmt.Errorf("unexpected positional argument %q", flags.Arg(0))
	}
	if _, err := url.ParseRequestURI(cfg.BaseURL); err != nil {
		return configuration{}, fmt.Errorf("invalid base URL: %w", err)
	}
	if cfg.RequestsPerScenario <= 0 || cfg.WarmupRequests < 0 || cfg.Concurrency <= 0 || cfg.RequestTimeout <= 0 {
		return configuration{}, errors.New("requests, concurrency, and timeout must be positive; warmup cannot be negative")
	}
	if cfg.ArtistRows < 2_000 || cfg.LabelRows < 100 || cfg.MasterRows < 100 || cfg.ReleaseRows < 2_000 {
		return configuration{}, errors.New("fixture row counts are too small for the performance scenarios")
	}
	if cfg.P99RegressionLimit < 0 || cfg.P99RegressionMinMS < 0 {
		return configuration{}, errors.New("p99 regression thresholds cannot be negative")
	}
	return cfg, nil
}

func validateInventory(path string, tests []scenario) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read OpenAPI document: %w", err)
	}
	var document openAPIDocument
	if err := json.Unmarshal(contents, &document); err != nil {
		return fmt.Errorf("decode OpenAPI document: %w", err)
	}
	operations := make(map[string]string)
	for route, pathItem := range document.Paths {
		if pathItem.Get == nil || slices.Contains(pathItem.Get.Tags, systemTag) {
			continue
		}
		if pathItem.Get.OperationID == "" {
			return fmt.Errorf("OpenAPI %s %s has no operationId", httpMethodGet, route)
		}
		operations[pathItem.Get.OperationID] = route
	}
	covered := make(map[string]bool, len(tests))
	for _, testScenario := range tests {
		route, exists := operations[testScenario.OperationID]
		if !exists {
			return fmt.Errorf("scenario %s references unknown operation %s", testScenario.Name, testScenario.OperationID)
		}
		if route != testScenario.Path {
			return fmt.Errorf("scenario %s route %s does not match OpenAPI route %s", testScenario.Name, testScenario.Path, route)
		}
		covered[testScenario.OperationID] = true
	}
	missing := make([]string, 0)
	for operationID := range operations {
		if !covered[operationID] {
			missing = append(missing, operationID)
		}
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		return fmt.Errorf("performance scenarios do not cover OpenAPI operations: %s", strings.Join(missing, ", "))
	}
	return nil
}

func newHTTPClient(cfg configuration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = cfg.Concurrency * 2
	transport.MaxIdleConnsPerHost = cfg.Concurrency
	transport.MaxConnsPerHost = cfg.Concurrency
	transport.DisableCompression = true
	return &http.Client{Transport: transport, Timeout: cfg.RequestTimeout}
}

func executeWarmup(client *http.Client, cfg configuration, testScenario scenario) error {
	for requestIndex := 0; requestIndex < cfg.WarmupRequests; requestIndex++ {
		result := performRequest(client, cfg.BaseURL+testScenario.Target)
		if result.Err != nil {
			return result.Err
		}
		if result.Status != expectedStatusCode {
			return fmt.Errorf("unexpected HTTP status %d", result.Status)
		}
	}
	return nil
}

func executeScenario(client *http.Client, cfg configuration, testScenario scenario) scenarioResult {
	jobs := make(chan struct{})
	samples := make(chan sample, cfg.RequestsPerScenario)
	var workers sync.WaitGroup
	workerCount := min(cfg.Concurrency, cfg.RequestsPerScenario)
	workers.Add(workerCount)
	started := time.Now()
	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		go func() {
			defer workers.Done()
			for range jobs {
				samples <- performRequest(client, cfg.BaseURL+testScenario.Target)
			}
		}()
	}
	go func() {
		for requestIndex := 0; requestIndex < cfg.RequestsPerScenario; requestIndex++ {
			jobs <- struct{}{}
		}
		close(jobs)
		workers.Wait()
		close(samples)
	}()
	durations := make([]time.Duration, 0, cfg.RequestsPerScenario)
	result := scenarioResult{
		Name:        testScenario.Name,
		OperationID: testScenario.OperationID,
		Target:      testScenario.Target,
		Requests:    cfg.RequestsPerScenario,
	}
	for requestSample := range samples {
		if requestSample.Err != nil || requestSample.Status != expectedStatusCode {
			result.Failed++
			if result.FirstError == "" {
				if requestSample.Err != nil {
					result.FirstError = requestSample.Err.Error()
				} else {
					result.FirstError = fmt.Sprintf("unexpected HTTP status %d", requestSample.Status)
				}
			}
			continue
		}
		result.Succeeded++
		result.ResponseBytes += requestSample.Bytes
		durations = append(durations, requestSample.Duration)
	}
	elapsed := time.Since(started)
	result.ElapsedMilliseconds = milliseconds(elapsed)
	result.RequestsPerSecond = float64(result.Requests) / elapsed.Seconds()
	if len(durations) > 0 {
		slices.Sort(durations)
		result.P50Milliseconds = milliseconds(percentile(durations, 0.50))
		result.P95Milliseconds = milliseconds(percentile(durations, 0.95))
		result.P99Milliseconds = milliseconds(percentile(durations, 0.99))
	}
	return result
}

func performRequest(client *http.Client, target string) sample {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		return sample{Err: err}
	}
	started := time.Now()
	response, err := client.Do(request)
	duration := time.Since(started)
	if err != nil {
		return sample{Duration: duration, Err: err}
	}
	defer response.Body.Close()
	written, readErr := io.Copy(io.Discard, response.Body)
	return sample{Duration: duration, Bytes: written, Status: response.StatusCode, Err: readErr}
}

func percentile(sorted []time.Duration, fraction float64) time.Duration {
	index := int(math.Ceil(float64(len(sorted))*fraction)) - 1
	return sorted[index]
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func writeReport(path string, benchmarkReport report) error {
	contents, err := json.MarshalIndent(benchmarkReport, "", "  ")
	if err != nil {
		return fmt.Errorf("encode performance report: %w", err)
	}
	contents = append(contents, '\n')
	if err := os.WriteFile(path, contents, outputFilePermissions); err != nil {
		return fmt.Errorf("write performance report: %w", err)
	}
	return nil
}

func compareBaseline(output io.Writer, cfg configuration, current report) error {
	contents, err := os.ReadFile(cfg.BaselinePath)
	if err != nil {
		return fmt.Errorf("read performance baseline: %w", err)
	}
	var baseline report
	if err := json.Unmarshal(contents, &baseline); err != nil {
		return fmt.Errorf("decode performance baseline: %w", err)
	}
	if baseline.RequestsPerScenario != current.RequestsPerScenario ||
		baseline.WarmupRequests != current.WarmupRequests ||
		baseline.Concurrency != current.Concurrency ||
		baseline.ArtistRows != current.ArtistRows ||
		baseline.LabelRows != current.LabelRows ||
		baseline.MasterRows != current.MasterRows ||
		baseline.ReleaseRows != current.ReleaseRows {
		return errors.New("performance baseline conditions do not match the current run")
	}
	baselineByName := make(map[string]scenarioResult, len(baseline.Results))
	for _, result := range baseline.Results {
		baselineByName[result.Name] = result
	}
	fmt.Fprintln(output, "scenario\tbaseline_p99_ms\tcurrent_p99_ms\tchange_percent\tgate")
	regressions := make([]string, 0)
	for _, currentResult := range current.Results {
		baselineResult, exists := baselineByName[currentResult.Name]
		if !exists {
			return fmt.Errorf("performance baseline is missing scenario %s", currentResult.Name)
		}
		if baselineResult.P99Milliseconds <= 0 {
			return fmt.Errorf("performance baseline scenario %s has an invalid p99", currentResult.Name)
		}
		changePercent := (currentResult.P99Milliseconds/baselineResult.P99Milliseconds - 1) * 100
		gate := "pass"
		absoluteChange := currentResult.P99Milliseconds - baselineResult.P99Milliseconds
		if changePercent > cfg.P99RegressionLimit && absoluteChange > cfg.P99RegressionMinMS {
			gate = "fail"
			regressions = append(regressions, currentResult.Name)
		}
		fmt.Fprintf(
			output,
			"%s\t%.3f\t%.3f\t%.1f\t%s\n",
			currentResult.Name,
			baselineResult.P99Milliseconds,
			currentResult.P99Milliseconds,
			changePercent,
			gate,
		)
	}
	if len(regressions) > 0 {
		return fmt.Errorf(
			"p99 regression exceeds %.1f%% and %.3f ms for scenarios: %s",
			cfg.P99RegressionLimit,
			cfg.P99RegressionMinMS,
			strings.Join(regressions, ", "),
		)
	}
	return nil
}
