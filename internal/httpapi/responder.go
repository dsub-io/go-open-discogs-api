package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

const (
	responseBufferCount       = 16
	maximumPooledResponseSize = 1 << 18
)

type JSONPayload interface {
	PageResponse[catalog.Artist] |
		PageResponse[catalog.ArtistRelease] |
		PageResponse[catalog.Label] |
		PageResponse[catalog.LabelRelease] |
		PageResponse[catalog.Master] |
		PageResponse[catalog.MasterRelease] |
		PageResponse[catalog.Release] |
		catalog.ArtistDetail |
		catalog.LabelDetail |
		catalog.MasterDetail |
		catalog.ReleaseDetail |
		problemDocument |
		versionDocument
}

type Responder struct {
	cacheControl string
	logger       *slog.Logger
	buffers      chan *bytes.Buffer
}

func NewResponder(cacheControl string, logger *slog.Logger) *Responder {
	return &Responder{
		cacheControl: cacheControl,
		logger:       logger,
		buffers:      make(chan *bytes.Buffer, responseBufferCount),
	}
}

func writeJSON[T JSONPayload](responder *Responder, writer http.ResponseWriter, status int, value T) {
	buffer := responder.getBuffer()
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	if writer.Header().Get(HeaderContentType) == "" {
		writer.Header().Set(HeaderContentType, ContentTypeJSON)
	}
	if status >= http.StatusOK && status < http.StatusMultipleChoices && responder.cacheControl != "" {
		writer.Header().Set(HeaderCacheControl, responder.cacheControl)
	}
	writer.Header().Set(HeaderContentLength, strconv.Itoa(buffer.Len()))
	writer.WriteHeader(status)
	if _, err := writer.Write(buffer.Bytes()); err != nil {
		responder.logger.Debug("write HTTP response", "error", err)
	}
	responder.putBuffer(buffer)
}

func (r *Responder) RepositoryError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		r.Problem(writer, request, http.StatusNotFound, ProblemTitleNotFound, ProblemDetailNotFound)
	case errors.Is(err, context.DeadlineExceeded):
		r.Problem(writer, request, http.StatusGatewayTimeout, ProblemTitleTimeout, ProblemDetailTimeout)
	case errors.Is(err, context.Canceled):
		return
	default:
		r.logger.Error("repository request failed", "path", request.URL.Path, "error", err)
		r.Problem(writer, request, http.StatusInternalServerError, ProblemTitleInternal, ProblemDetailInternal)
	}
}

func (r *Responder) BadRequest(writer http.ResponseWriter, request *http.Request, err error) {
	r.Problem(writer, request, http.StatusBadRequest, ProblemTitleBadRequest, err.Error())
}

func (r *Responder) Problem(writer http.ResponseWriter, request *http.Request, status int, title, detail string) {
	writer.Header().Set(HeaderContentType, ContentTypeProblemJSON)
	writeJSON(r, writer, status, problemDocument{
		Type: ProblemTypeAboutBlank, Title: title, Status: status, Detail: detail, Instance: request.URL.Path,
	})
}

func (r *Responder) Document(writer http.ResponseWriter, document []byte) {
	writer.Header().Set(HeaderContentType, ContentTypeJSON)
	writer.Header().Set(HeaderContentLength, strconv.Itoa(len(document)))
	if r.cacheControl != "" {
		writer.Header().Set(HeaderCacheControl, r.cacheControl)
	}
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write(document); err != nil {
		r.logger.Debug("write HTTP document", "error", err)
	}
}

func (r *Responder) getBuffer() *bytes.Buffer {
	select {
	case buffer := <-r.buffers:
		buffer.Reset()
		return buffer
	default:
		return &bytes.Buffer{}
	}
}

func (r *Responder) putBuffer(buffer *bytes.Buffer) {
	if buffer.Cap() > maximumPooledResponseSize {
		return
	}
	buffer.Reset()
	select {
	case r.buffers <- buffer:
	default:
	}
}

type problemDocument struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

type versionDocument struct {
	Version string `json:"version"`
}
