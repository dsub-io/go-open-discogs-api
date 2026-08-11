package httpapi

import (
	"net/http"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

type ArtistHandler struct {
	reader    catalog.ArtistReader
	responder *Responder
}

func NewArtistHandler(reader catalog.ArtistReader, responder *Responder) *ArtistHandler {
	return &ArtistHandler{reader: reader, responder: responder}
}

func (h *ArtistHandler) Search(writer http.ResponseWriter, request *http.Request) {
	pageRequest, err := parseCursorPage(request.URL.Query())
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	name, err := optionalSearchTerm(request.URL.Query().Get(ParameterName), ParameterName)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	realName, err := optionalSearchTerm(request.URL.Query().Get(ParameterRealName), ParameterRealName)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.SearchArtists(request.Context(), catalog.ArtistFilter{
		Name: name, RealName: realName,
	}, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, request.URL.Path))
}

func (h *ArtistHandler) Get(writer http.ResponseWriter, request *http.Request) {
	id, err := pathID(request)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	item, err := h.reader.Artist(request.Context(), id)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, item)
}

func (h *ArtistHandler) Releases(writer http.ResponseWriter, request *http.Request) {
	id, err := pathID(request)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	pageRequest, err := parseCursorPage(request.URL.Query())
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.ArtistReleases(request.Context(), id, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, request.URL.Path))
}
