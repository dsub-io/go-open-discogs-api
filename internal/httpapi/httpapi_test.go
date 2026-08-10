package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/dsub-io/go-open-discogs-api/internal/observability"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	testServerURL    = "https://api.example.com"
	testCacheControl = "public, max-age=10"
)

var errRepository = errors.New("repository failure")

func TestRouterServesEveryContractRoute(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	handler := testRouter(&repositoryStub{}, true, &logs)
	tests := []struct {
		path     string
		contains string
	}{
		{"/artists?name=a&real_name=b&profile=c&page=2&size=31&sort=name,desc", `"total_elements":1`},
		{"/artists/1", `"release_url"`},
		{"/artists/1/releases?sort=released_year,desc", `"resource_url"`},
		{"/labels?contact_info=a&data_quality=b&name=c&profile=d&sort=name,asc", `"total_elements":1`},
		{"/labels/1", `"parent_label"`},
		{"/labels/1/releases?sort=year,desc", `"catno"`},
		{"/masters?title=a&year=2000&sort=released_year,desc", `"total_elements":1`},
		{"/masters/1", `"main_release"`},
		{"/masters/1/releases?sort=title,asc", `"artist_id"`},
		{"/releases?title=a&country=US&year=2000&month=2&master=true&sort=country,desc", `"total_elements":1`},
		{"/releases/1", `"companies"`},
		{RouteOpenAPI, `"openapi": "3.1.0"`},
		{RouteOpenAPIJSON, `"openapi": "3.1.0"`},
		{RouteVersion, `"version"`},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Header().Get(HeaderCacheControl) != testCacheControl || response.Header().Get(HeaderTypeOptions) != HeaderValueNoSniff {
				t.Fatalf("unexpected headers: %v", response.Header())
			}
		})
	}
	if !strings.Contains(logs.String(), logMessageRequest) {
		t.Fatalf("access log missing: %s", logs.String())
	}
}

func TestRouterMapsRepositoryFailuresForEveryUseCase(t *testing.T) {
	t.Parallel()
	handler := testRouter(&repositoryStub{err: errRepository}, false, &bytes.Buffer{})
	paths := []string{
		"/artists", "/artists/1", "/artists/1/releases",
		"/labels", "/labels/1", "/labels/1/releases",
		"/masters", "/masters/1", "/masters/1/releases",
		"/releases", "/releases/1",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestRouterRejectsInvalidTransportInput(t *testing.T) {
	t.Parallel()
	handler := testRouter(&repositoryStub{}, false, &bytes.Buffer{})
	paths := []string{
		"/artists?sort=unknown", "/artists/not-an-id", "/artists/0/releases", "/artists/1/releases?sort=unknown",
		"/labels?sort=unknown", "/labels/0", "/labels/0/releases", "/labels/1/releases?sort=unknown",
		"/masters?year=invalid", "/masters?sort=unknown", "/masters/0", "/masters/0/releases", "/masters/1/releases?sort=unknown",
		"/releases?year=invalid", "/releases?month=13", "/releases?master=maybe", "/releases?sort=unknown", "/releases/0",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusBadRequest || response.Header().Get(HeaderContentType) != ContentTypeProblemJSON {
				t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
			}
		})
	}
}

func TestRouterRecoversBeforeHeadersAreWritten(t *testing.T) {
	t.Parallel()
	handler := testRouter(&repositoryStub{panicSearch: true}, false, &bytes.Buffer{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, RouteArtists, nil))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), ProblemTitleInternal) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestOpenAPIDocumentIsValid(t *testing.T) {
	t.Parallel()
	document, err := openapi3.NewLoader().LoadFromData(openAPIDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPaginationAndRequestParsing(t *testing.T) {
	t.Parallel()
	request, err := parsePage(url.Values{ParameterSize: {"100"}}, sortFields(catalog.FieldID), defaultIDSort())
	if err != nil || request.Page != 1 || request.Size != maximumPageSize || request.Offset() != 0 {
		t.Fatalf("request=%+v err=%v", request, err)
	}
	request, err = parsePage(url.Values{ParameterPage: {"3"}, ParameterSize: {"2"}, ParameterSort: {"id", "id,desc"}}, sortFields(catalog.FieldID), defaultIDSort())
	if err != nil || request.Offset() != 4 || len(request.Sort) != 2 || request.Sort[1].Direction != catalog.Descending {
		t.Fatalf("request=%+v err=%v", request, err)
	}
	invalid := []url.Values{
		{ParameterPage: {"0"}}, {ParameterSize: {"invalid"}}, {ParameterSort: {"id,sideways"}}, {ParameterSort: {"id,asc,extra"}},
	}
	for _, values := range invalid {
		if _, err := parsePage(values, sortFields(catalog.FieldID), defaultIDSort()); err == nil {
			t.Fatalf("accepted invalid values: %v", values)
		}
	}

	page := pageResponse(catalog.Page[catalog.Artist]{Items: nil, Total: 0}, catalog.PageRequest{Page: 1, Size: 20}, RouteArtists)
	if page.PageNumber != 0 || !page.First || !page.Last || page.Items == nil || page.Sorted {
		t.Fatalf("unexpected empty page: %+v", page)
	}
	page = pageResponse(catalog.Page[catalog.Artist]{Items: []catalog.Artist{{ID: 1}}, Total: 21}, catalog.PageRequest{Page: 2, Size: 20, Sort: defaultIDSort()}, RouteArtists)
	if page.TotalPages != 2 || !page.Last || page.First || !page.Sorted {
		t.Fatalf("unexpected populated page: %+v", page)
	}

	if value, err := optionalInt("", 1, 12, ParameterMonth); err != nil || value != nil {
		t.Fatalf("optional int value=%v err=%v", value, err)
	}
	if value, err := optionalBool("false", ParameterMaster); err != nil || value == nil || *value {
		t.Fatalf("optional bool value=%v err=%v", value, err)
	}
	if value, err := optionalBool("", ParameterMaster); err != nil || value != nil {
		t.Fatalf("optional bool value=%v err=%v", value, err)
	}
}

func TestResponderErrorMappingAndBufferBounds(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	responder := NewResponder(testCacheControl, logger)
	request := httptest.NewRequest(http.MethodGet, "/resource", nil)
	tests := []struct {
		err    error
		status int
	}{
		{catalog.ErrNotFound, http.StatusNotFound},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
		{errRepository, http.StatusInternalServerError},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		responder.RepositoryError(response, request, test.err)
		if response.Code != test.status {
			t.Fatalf("error=%v status=%d", test.err, response.Code)
		}
	}
	canceled := httptest.NewRecorder()
	responder.RepositoryError(canceled, request, context.Canceled)
	if canceled.Body.Len() != 0 {
		t.Fatalf("canceled response body=%s", canceled.Body.String())
	}

	buffer := responder.getBuffer()
	buffer.WriteString("reusable")
	responder.putBuffer(buffer)
	if reused := responder.getBuffer(); reused.Len() != 0 {
		t.Fatalf("pooled buffer was not reset: %d", reused.Len())
	} else {
		responder.putBuffer(reused)
	}
	large := bytes.NewBuffer(make([]byte, 0, maximumPooledResponseSize+1))
	responder.putBuffer(large)
	for index := 0; index < responseBufferCount+1; index++ {
		responder.putBuffer(&bytes.Buffer{})
	}

	failing := newFailingWriter()
	writeJSON(responder, failing, http.StatusOK, versionDocument{Version: "test"})
	responder.Document(failing, openAPIDocument)
	if !strings.Contains(logs.String(), "write HTTP response") || !strings.Contains(logs.String(), "write HTTP document") {
		t.Fatalf("write errors not logged: %s", logs.String())
	}
}

func TestStatusWriterAndSecurityHeaders(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()
	writer := &statusWriter{ResponseWriter: response, status: http.StatusOK}
	writer.WriteHeader(http.StatusCreated)
	writer.WriteHeader(http.StatusNoContent)
	if _, err := writer.Write([]byte("body")); err != nil {
		t.Fatal(err)
	}
	if writer.status != http.StatusCreated || writer.Unwrap() != response {
		t.Fatalf("unexpected writer: %+v", writer)
	}
	implicit := &statusWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	if _, err := implicit.Write([]byte("body")); err != nil || !implicit.wroteHeader {
		t.Fatalf("implicit status write failed: %+v err=%v", implicit, err)
	}

	response = httptest.NewRecorder()
	handler := securityHeaders(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Header().Get(HeaderFrameOptions) != HeaderValueDeny || response.Header().Get(HeaderReferrerPolicy) != HeaderValueNoReferrer {
		t.Fatalf("security headers missing: %v", response.Header())
	}
}

func testRouter(repository catalog.Repository, accessLog bool, logOutput *bytes.Buffer) http.Handler {
	logger := slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))
	metrics := observability.NewMetrics(prometheus.NewRegistry(), false)
	return NewRouter(repository, testCacheControl, accessLog, logger, metrics, observability.NoopHTTPTracer{}).Handler()
}

type repositoryStub struct {
	err         error
	panicSearch bool
}

func (r *repositoryStub) SearchArtists(context.Context, catalog.ArtistFilter, catalog.PageRequest) (catalog.Page[catalog.Artist], error) {
	if r.panicSearch {
		panic("test panic")
	}
	return catalog.Page[catalog.Artist]{Items: []catalog.Artist{{ID: 1}}, Total: 1}, r.err
}

func (r *repositoryStub) Artist(context.Context, int64) (catalog.ArtistDetail, error) {
	return catalog.ArtistDetail{Artist: catalog.Artist{ID: 1}, Members: []catalog.ArtistReference{}, Groups: []catalog.ArtistReference{}, Aliases: []catalog.ArtistReference{}, NameVariations: []string{}, URLs: []string{}}, r.err
}

func (r *repositoryStub) ArtistReleases(context.Context, int64, catalog.PageRequest) (catalog.Page[catalog.ArtistRelease], error) {
	return catalog.Page[catalog.ArtistRelease]{Items: []catalog.ArtistRelease{{ID: 1, ResourceURL: testServerURL + "/releases/1"}}, Total: 1}, r.err
}

func (r *repositoryStub) SearchLabels(context.Context, catalog.LabelFilter, catalog.PageRequest) (catalog.Page[catalog.Label], error) {
	return catalog.Page[catalog.Label]{Items: []catalog.Label{{ID: 1}}, Total: 1}, r.err
}

func (r *repositoryStub) Label(context.Context, int64) (catalog.LabelDetail, error) {
	return catalog.LabelDetail{Label: catalog.Label{ID: 1}, Sublabels: []catalog.LabelReference{}, URLs: []string{}}, r.err
}

func (r *repositoryStub) LabelReleases(context.Context, int64, catalog.PageRequest) (catalog.Page[catalog.LabelRelease], error) {
	return catalog.Page[catalog.LabelRelease]{Items: []catalog.LabelRelease{{ID: 1}}, Total: 1}, r.err
}

func (r *repositoryStub) SearchMasters(context.Context, catalog.MasterFilter, catalog.PageRequest) (catalog.Page[catalog.Master], error) {
	return catalog.Page[catalog.Master]{Items: []catalog.Master{{ID: 1}}, Total: 1}, r.err
}

func (r *repositoryStub) Master(context.Context, int64) (catalog.MasterDetail, error) {
	return catalog.MasterDetail{ID: 1, Genres: []string{}, Styles: []string{}, Artists: []catalog.ArtistReference{}, Videos: []catalog.MasterVideo{}}, r.err
}

func (r *repositoryStub) MasterReleases(context.Context, int64, catalog.PageRequest) (catalog.Page[catalog.MasterRelease], error) {
	return catalog.Page[catalog.MasterRelease]{Items: []catalog.MasterRelease{{ID: 1, Artists: []string{}, ArtistIDs: []int64{}}}, Total: 1}, r.err
}

func (r *repositoryStub) SearchReleases(context.Context, catalog.ReleaseFilter, catalog.PageRequest) (catalog.Page[catalog.Release], error) {
	return catalog.Page[catalog.Release]{Items: []catalog.Release{{ID: 1}}, Total: 1}, r.err
}

func (r *repositoryStub) Release(context.Context, int64) (catalog.ReleaseDetail, error) {
	return catalog.ReleaseDetail{Release: catalog.Release{ID: 1}, Artists: []catalog.ReleaseArtist{}, Labels: []catalog.ReleaseLabel{}, Companies: []catalog.ReleaseLabel{}, Formats: []catalog.ReleaseFormat{}, Styles: []string{}, Genres: []string{}, Videos: []catalog.ReleaseVideo{}}, r.err
}

type failingWriter struct {
	header http.Header
}

func newFailingWriter() *failingWriter {
	return &failingWriter{header: make(http.Header)}
}

func (w *failingWriter) Header() http.Header { return w.header }

func (w *failingWriter) Write([]byte) (int, error) { return 0, errRepository }

func (w *failingWriter) WriteHeader(int) {}
