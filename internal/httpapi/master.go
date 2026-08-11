package httpapi

import (
	"math"
	"net/http"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

type MasterHandler struct {
	reader    catalog.MasterReader
	responder *Responder
}

func NewMasterHandler(reader catalog.MasterReader, responder *Responder) *MasterHandler {
	return &MasterHandler{reader: reader, responder: responder}
}

func (h *MasterHandler) Search(writer http.ResponseWriter, request *http.Request) {
	pageRequest, err := parseCursorPage(request.URL.Query())
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	title, err := optionalSearchTerm(request.URL.Query().Get(ParameterTitle), ParameterTitle)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	year, err := optionalInt(request.URL.Query().Get(ParameterYear), 0, math.MaxInt16, ParameterYear)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.SearchMasters(request.Context(), catalog.MasterFilter{
		Title: title, Year: year,
	}, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, request.URL.Path))
}

func (h *MasterHandler) Get(writer http.ResponseWriter, request *http.Request) {
	id, err := pathID(request)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	item, err := h.reader.Master(request.Context(), id)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, item)
}

func (h *MasterHandler) Releases(writer http.ResponseWriter, request *http.Request) {
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
	page, err := h.reader.MasterReleases(request.Context(), id, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, request.URL.Path))
}
