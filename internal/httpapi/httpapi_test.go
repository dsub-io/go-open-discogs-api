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
	testServerURL                    = "https://api.example.com"
	testCacheControl                 = "public, max-age=10"
	testSnapshotDescriptionFragment  = "currently imported public Discogs monthly dump snapshot"
	testUnorderedDescriptionFragment = "Nested relation arrays are unordered"
	testNotFoundDescriptionFragment  = "does not assert absence from Discogs"
	testReleaseFormatQuantity        = "1010487400000000000000000000000000000000000000000000"
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
		{"/artists?name=alpha&real_name=beta&after_id=1&size=30", `"has_more":false`},
		{"/artists/1", `"release_url"`},
		{"/artists/1/releases?after_id=1", `"resource_url"`},
		{"/labels?name=label", `"has_more":false`},
		{"/labels/1", `"parent_label"`},
		{"/labels/1/releases?after_id=1", `"catnos"`},
		{"/masters?title=master&year=2000", `"has_more":false`},
		{"/masters/1", `"main_release"`},
		{"/masters/1/releases?after_id=1", `"artist_id"`},
		{"/releases?title=release&country=US&year=2000&month=2&master=true", `"has_more":false`},
		{"/releases/1", `"companies"`},
		{"/releases/1", `"qty":"` + testReleaseFormatQuantity + `"`},
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
		"/artists?name=ab", "/artists?real_name=ab", "/artists?after_id=-1", "/artists?size=31", "/artists/not-an-id", "/artists/0/releases", "/artists/1/releases?after_id=invalid",
		"/labels?name=ab", "/labels?size=31", "/labels/0", "/labels/0/releases", "/labels/1/releases?after_id=invalid",
		"/masters?title=ab", "/masters?year=invalid", "/masters?after_id=invalid", "/masters/0", "/masters/0/releases", "/masters/1/releases?size=31",
		"/releases?title=ab", "/releases?country=" + strings.Repeat("x", maximumCountryLength+1), "/releases?after_id=invalid", "/releases?year=invalid", "/releases?month=13", "/releases?month=2", "/releases?master=maybe", "/releases/0",
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
	if !strings.Contains(document.Info.Description, testSnapshotDescriptionFragment) {
		t.Fatalf("OpenAPI description does not define snapshot semantics: %s", document.Info.Description)
	}
	if !strings.Contains(document.Info.Description, testUnorderedDescriptionFragment) {
		t.Fatalf("OpenAPI description does not define relation ordering semantics: %s", document.Info.Description)
	}
	notFound := document.Components.Responses["NotFound"]
	if notFound == nil || notFound.Value == nil || notFound.Value.Description == nil ||
		!strings.Contains(*notFound.Value.Description, testNotFoundDescriptionFragment) {
		t.Fatalf("OpenAPI 404 response does not define snapshot semantics: %+v", notFound)
	}
}

func TestPaginationAndRequestParsing(t *testing.T) {
	t.Parallel()
	request, err := parseCursorPage(url.Values{ParameterAfterID: {"40"}, ParameterSize: {"30"}})
	if err != nil || request.AfterID != 40 || request.Size != maximumPageSize || request.FetchSize() != 31 {
		t.Fatalf("request=%+v err=%v", request, err)
	}
	request, err = parseCursorPage(url.Values{})
	if err != nil || request.AfterID != 0 || request.Size != defaultPageSize {
		t.Fatalf("request=%+v err=%v", request, err)
	}
	invalid := []url.Values{
		{ParameterAfterID: {"-1"}}, {ParameterAfterID: {"2147483648"}}, {ParameterSize: {"invalid"}}, {ParameterSize: {"31"}},
	}
	for _, values := range invalid {
		if _, err := parseCursorPage(values); err == nil {
			t.Fatalf("accepted invalid values: %v", values)
		}
	}

	page := pageResponse(catalog.Page[catalog.Artist]{Items: nil}, RouteArtists)
	if page.HasMore || page.NextAfterID != nil || page.Items == nil || page.PageSize != 0 {
		t.Fatalf("unexpected empty page: %+v", page)
	}
	page = pageResponse(catalog.NewPage([]catalog.Artist{{ID: 1}, {ID: 2}}, 1), RouteArtists)
	if !page.HasMore || page.NextAfterID == nil || *page.NextAfterID != 1 || page.PageSize != 1 {
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
	if value, err := optionalSearchTerm(" abc ", ParameterName); err != nil || value != "abc" {
		t.Fatalf("search value=%q err=%v", value, err)
	}
	if _, err := optionalSearchTerm("ab", ParameterName); err == nil {
		t.Fatal("short search term was accepted")
	}
	if _, err := optionalSearchTerm(string([]byte{0xff, 0xfe, 0xfd}), ParameterName); err == nil {
		t.Fatal("invalid UTF-8 search term was accepted")
	}
	if value, err := optionalCountry(" KR "); err != nil || value != "KR" {
		t.Fatalf("country=%q err=%v", value, err)
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
		if errors.Is(test.err, catalog.ErrNotFound) && !strings.Contains(response.Body.String(), ProblemDetailNotFound) {
			t.Fatalf("not-found response does not define snapshot semantics: %s", response.Body.String())
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
	return catalog.Page[catalog.Artist]{Items: []catalog.Artist{{ID: 1}}}, r.err
}

func (r *repositoryStub) Artist(context.Context, int64) (catalog.ArtistDetail, error) {
	return catalog.ArtistDetail{Artist: catalog.Artist{ID: 1}, Members: []catalog.ArtistReference{}, Groups: []catalog.ArtistReference{}, Aliases: []catalog.ArtistReference{}, NameVariations: []string{}, URLs: []string{}}, r.err
}

func (r *repositoryStub) ArtistReleases(context.Context, int64, catalog.PageRequest) (catalog.Page[catalog.ArtistRelease], error) {
	return catalog.Page[catalog.ArtistRelease]{Items: []catalog.ArtistRelease{{ID: 1, ResourceURL: testServerURL + "/releases/1"}}}, r.err
}

func (r *repositoryStub) SearchLabels(context.Context, catalog.LabelFilter, catalog.PageRequest) (catalog.Page[catalog.Label], error) {
	return catalog.Page[catalog.Label]{Items: []catalog.Label{{ID: 1}}}, r.err
}

func (r *repositoryStub) Label(context.Context, int64) (catalog.LabelDetail, error) {
	return catalog.LabelDetail{Label: catalog.Label{ID: 1}, Sublabels: []catalog.LabelReference{}, URLs: []string{}}, r.err
}

func (r *repositoryStub) LabelReleases(context.Context, int64, catalog.PageRequest) (catalog.Page[catalog.LabelRelease], error) {
	return catalog.Page[catalog.LabelRelease]{Items: []catalog.LabelRelease{{ID: 1}}}, r.err
}

func (r *repositoryStub) SearchMasters(context.Context, catalog.MasterFilter, catalog.PageRequest) (catalog.Page[catalog.Master], error) {
	return catalog.Page[catalog.Master]{Items: []catalog.Master{{ID: 1}}}, r.err
}

func (r *repositoryStub) Master(context.Context, int64) (catalog.MasterDetail, error) {
	return catalog.MasterDetail{ID: 1, Genres: []string{}, Styles: []string{}, Artists: []catalog.ArtistReference{}, Videos: []catalog.MasterVideo{}}, r.err
}

func (r *repositoryStub) MasterReleases(context.Context, int64, catalog.PageRequest) (catalog.Page[catalog.MasterRelease], error) {
	return catalog.Page[catalog.MasterRelease]{Items: []catalog.MasterRelease{{ID: 1, Artists: []string{}, ArtistIDs: []int64{}}}}, r.err
}

func (r *repositoryStub) SearchReleases(context.Context, catalog.ReleaseFilter, catalog.PageRequest) (catalog.Page[catalog.Release], error) {
	return catalog.Page[catalog.Release]{Items: []catalog.Release{{ID: 1}}}, r.err
}

func (r *repositoryStub) Release(context.Context, int64) (catalog.ReleaseDetail, error) {
	quantity := testReleaseFormatQuantity
	return catalog.ReleaseDetail{Release: catalog.Release{ID: 1}, Artists: []catalog.ReleaseArtist{}, Labels: []catalog.ReleaseLabel{}, Companies: []catalog.ReleaseLabel{}, Formats: []catalog.ReleaseFormat{{Quantity: &quantity, Descriptions: []string{}}}, Styles: []string{}, Genres: []string{}, Videos: []catalog.ReleaseVideo{}}, r.err
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
