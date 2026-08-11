package httpapi

import (
	"errors"
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
	country, err := optionalCountry(request.URL.Query().Get(ParameterCountry))
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
	if month != nil && year == nil {
		h.responder.BadRequest(writer, request, errors.New(errorMonthRequiresYear))
		return
	}
	master, err := optionalBool(request.URL.Query().Get(ParameterMaster), ParameterMaster)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.SearchReleases(request.Context(), catalog.ReleaseFilter{
		Title: title, Country: country,
		Year: year, Month: month, Master: master,
	}, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, request.URL.Path))
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
