package httpapi

import (
	"net/http"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

type LabelHandler struct {
	reader    catalog.LabelReader
	responder *Responder
}

func NewLabelHandler(reader catalog.LabelReader, responder *Responder) *LabelHandler {
	return &LabelHandler{reader: reader, responder: responder}
}

func (h *LabelHandler) Search(writer http.ResponseWriter, request *http.Request) {
	pageRequest, err := parsePage(request.URL.Query(), sortFields(
		catalog.FieldID, catalog.FieldContactInfo, catalog.FieldDataQuality, catalog.FieldName, catalog.FieldProfile,
	), defaultIDSort())
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.SearchLabels(request.Context(), catalog.LabelFilter{
		ContactInfo: request.URL.Query().Get(ParameterContactInfo), DataQuality: request.URL.Query().Get(ParameterDataQuality),
		Name: request.URL.Query().Get(ParameterName), Profile: request.URL.Query().Get(ParameterProfile),
	}, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, pageRequest, request.URL.Path))
}

func (h *LabelHandler) Get(writer http.ResponseWriter, request *http.Request) {
	id, err := pathID(request)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	item, err := h.reader.Label(request.Context(), id)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, item)
}

func (h *LabelHandler) Releases(writer http.ResponseWriter, request *http.Request) {
	id, err := pathID(request)
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	pageRequest, err := parsePage(request.URL.Query(), sortFields(catalog.FieldID, catalog.FieldTitle, catalog.FieldYear), defaultIDSort())
	if err != nil {
		h.responder.BadRequest(writer, request, err)
		return
	}
	page, err := h.reader.LabelReleases(request.Context(), id, pageRequest)
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, pageResponse(page, pageRequest, request.URL.Path))
}
