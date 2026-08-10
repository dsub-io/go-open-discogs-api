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
	pageRequest, err := parsePage(request.URL.Query(), sortFields(
		catalog.FieldID, catalog.FieldName, catalog.FieldRealName, catalog.FieldProfile,
	), defaultIDSort())
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.SearchArtists(request.Context(), catalog.ArtistFilter{
		Name: request.URL.Query().Get(ParameterName), RealName: request.URL.Query().Get(ParameterRealName),
		Profile: request.URL.Query().Get(ParameterProfile),
	}, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, pageRequest, request.URL.Path))
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
	pageRequest, err := parsePage(request.URL.Query(), sortFields(
		catalog.FieldID, catalog.FieldTitle, catalog.FieldCountry, catalog.FieldMasterID,
		catalog.FieldReleasedYear, catalog.FieldReleasedMonth, catalog.FieldReleasedDay,
	), defaultIDSort())
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.ArtistReleases(request.Context(), id, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, pageRequest, request.URL.Path))
}
