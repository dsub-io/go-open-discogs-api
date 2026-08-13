package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/dsub-io/go-open-discogs-api/internal/buildinfo"
	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
	"github.com/dsub-io/go-open-discogs-api/internal/observability"
)

const (
	logMessagePanic   = "panic in HTTP handler"
	logMessageRequest = "HTTP request"
)

type Router struct {
	accessLog  bool
	httpTracer observability.HTTPTracer
	logger     *slog.Logger
	metrics    *observability.Metrics
	responder  *Responder
	repository catalog.Repository
}

func NewRouter(
	repository catalog.Repository,
	cacheControl string,
	accessLog bool,
	logger *slog.Logger,
	metrics *observability.Metrics,
	httpTracer observability.HTTPTracer,
) *Router {
	return &Router{
		accessLog:  accessLog,
		httpTracer: httpTracer,
		logger:     logger,
		metrics:    metrics,
		responder:  NewResponder(cacheControl, logger),
		repository: repository,
	}
}

func (r *Router) Handler() http.Handler {
	mux := http.NewServeMux()
	artistHandler := NewArtistHandler(r.repository, r.responder)
	labelHandler := NewLabelHandler(r.repository, r.responder)
	masterHandler := NewMasterHandler(r.repository, r.responder)
	releaseHandler := NewReleaseHandler(r.repository, r.responder)
	snapshotHandler := NewSnapshotHandler(r.repository, r.responder)
	documentationHandler := NewDocumentationHandler(r.responder)

	r.handle(mux, RouteArtists, artistHandler.Search)
	r.handle(mux, RouteArtist, artistHandler.Get)
	r.handle(mux, RouteArtistRelease, artistHandler.Releases)
	r.handle(mux, RouteLabels, labelHandler.Search)
	r.handle(mux, RouteLabel, labelHandler.Get)
	r.handle(mux, RouteLabelRelease, labelHandler.Releases)
	r.handle(mux, RouteMasters, masterHandler.Search)
	r.handle(mux, RouteMaster, masterHandler.Get)
	r.handle(mux, RouteMasterRelease, masterHandler.Releases)
	r.handle(mux, RouteReleases, releaseHandler.Search)
	r.handle(mux, RouteRelease, releaseHandler.Get)
	r.handle(mux, RouteReleaseTracks, releaseHandler.Tracks)
	r.handle(mux, RouteReleaseIdentifiers, releaseHandler.Identifiers)
	r.handle(mux, RouteSnapshot, snapshotHandler.Get)
	r.handle(mux, RouteOpenAPI, documentationHandler.OpenAPI)
	r.handle(mux, RouteOpenAPIJSON, documentationHandler.OpenAPI)
	r.handle(mux, RouteVersion, r.version)
	return securityHeaders(mux)
}

func (r *Router) handle(mux *http.ServeMux, route string, handler http.HandlerFunc) {
	pattern := MethodGet + " " + route
	mux.Handle(pattern, r.httpTracer.Wrap(route, r.wrap(route, handler)))
}

func (r *Router) wrap(route string, handler http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		r.metrics.RequestStarted(route)
		statusRecorder := &statusWriter{ResponseWriter: writer, status: http.StatusOK}
		defer func() {
			if recovered := recover(); recovered != nil {
				r.logger.Error(logMessagePanic, "route", route, "panic", recovered, "stack", string(debug.Stack()))
				if !statusRecorder.wroteHeader {
					r.responder.Problem(statusRecorder, request, http.StatusInternalServerError, ProblemTitleInternal, ProblemDetailInternal)
				}
			}
			r.metrics.RequestFinished(route, request.Method, statusRecorder.status, started)
			if r.accessLog {
				r.logger.Info(logMessageRequest, "method", request.Method, "route", route, "status", statusRecorder.status, "duration", time.Since(started))
			}
		}()
		handler(statusRecorder, request)
	})
}

func (r *Router) version(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(r.responder, writer, http.StatusOK, versionDocument{Version: buildinfo.Version})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(HeaderTypeOptions, HeaderValueNoSniff)
		writer.Header().Set(HeaderReferrerPolicy, HeaderValueNoReferrer)
		writer.Header().Set(HeaderFrameOptions, HeaderValueDeny)
		next.ServeHTTP(writer, request)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
