package httpapi

import (
	"net/http"

	"github.com/dsub-io/go-open-discogs-api/internal/catalog"
)

type SnapshotHandler struct {
	reader    catalog.SnapshotReader
	responder *Responder
}

func NewSnapshotHandler(reader catalog.SnapshotReader, responder *Responder) *SnapshotHandler {
	return &SnapshotHandler{reader: reader, responder: responder}
}

func (h *SnapshotHandler) Get(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := h.reader.Snapshot(request.Context())
	if err != nil {
		h.responder.RepositoryError(writer, request, err)
		return
	}
	writeJSON(h.responder, writer, http.StatusOK, snapshot)
}
