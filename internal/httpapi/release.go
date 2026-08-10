package httpapi

import (
	"math"
	"net/http"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

type ReleaseHandler struct {
	reader    catalog.ReleaseReader
	responder *Responder
}

func NewReleaseHandler(reader catalog.ReleaseReader, responder *Responder) *ReleaseHandler {
	return &ReleaseHandler{reader: reader, responder: responder}
}

func (h *ReleaseHandler) Search(writer http.ResponseWriter, request *http.Request) {
	pageRequest, err := parsePage(request.URL.Query(), sortFields(
		catalog.FieldID, catalog.FieldTitle, catalog.FieldCountry, catalog.FieldReleasedYear, catalog.FieldReleasedMonth,
	), defaultIDSort())
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	year, err := optionalInt(request.URL.Query().Get(ParameterYear), 0, math.MaxInt16, ParameterYear)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	month, err := optionalInt(request.URL.Query().Get(ParameterMonth), 1, 12, ParameterMonth)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	master, err := optionalBool(request.URL.Query().Get(ParameterMaster), ParameterMaster)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.SearchReleases(request.Context(), catalog.ReleaseFilter{
		Title: request.URL.Query().Get(ParameterTitle), Country: request.URL.Query().Get(ParameterCountry),
		Year: year, Month: month, Master: master,
	}, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, pageRequest, request.URL.Path))
}

func (h *ReleaseHandler) Get(writer http.ResponseWriter, request *http.Request) {
	id, err := pathID(request)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	item, err := h.reader.Release(request.Context(), id)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, item)
}
