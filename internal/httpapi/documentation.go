package httpapi

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openAPIDocument []byte

type DocumentationHandler struct {
	responder *Responder
}

func NewDocumentationHandler(responder *Responder) *DocumentationHandler {
	return &DocumentationHandler{responder: responder}
}

func (h *DocumentationHandler) OpenAPI(writer http.ResponseWriter, _ *http.Request) {
	h.responder.Document(writer, openAPIDocument)
}
