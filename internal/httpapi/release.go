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
	lookup, err := parseReleaseLookup(request.URL.Query())
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	switch lookup.kind {
	case releaseLookupCatalogNumber:
		page, err := h.reader.ReleasesByCatalogNumber(request.Context(), lookup.catalogNumber, pageRequest)
		if err != nil {
			h.responder.RepositoryError(writer, request, err)
			return
		}
		writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, request.URL.Path))
		return
	case releaseLookupIdentifier:
		page, err := h.reader.ReleasesByIdentifier(request.Context(), lookup.identifier, pageRequest)
		if err != nil {
			h.responder.RepositoryError(writer, request, err)
			return
		}
		writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, request.URL.Path))
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

func (h *ReleaseHandler) Tracks(writer http.ResponseWriter, request *http.Request) {
	id, pageRequest, err := releaseChildRequest(request)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.ReleaseTracks(request.Context(), id, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, hashPageResponse(page, request.URL.Path))
}

func (h *ReleaseHandler) Identifiers(writer http.ResponseWriter, request *http.Request) {
	id, pageRequest, err := releaseChildRequest(request)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.ReleaseIdentifiers(request.Context(), id, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, hashPageResponse(page, request.URL.Path))
}

func releaseChildRequest(request *http.Request) (int64, catalog.HashPageRequest, error) {
	id, err := pathID(request)
	if err != nil {
		return 0, catalog.HashPageRequest{}, err
	}
	pageRequest, err := parseHashCursorPage(request.URL.Query())
	if err != nil {
		return 0, catalog.HashPageRequest{}, err
	}
	return id, pageRequest, nil
}
