package httpapi

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openAPIDocument []byte

//go:embed docs/index.html
var documentationHTML []byte

//go:embed docs/reference.js
var documentationScript []byte

//go:embed docs/scalar-1.75.0.js
var scalarScript []byte

type DocumentationHandler struct {
	responder *Responder
}

func NewDocumentationHandler(responder *Responder) *DocumentationHandler {
	return &DocumentationHandler{responder: responder}
}

func (h *DocumentationHandler) OpenAPI(writer http.ResponseWriter, _ *http.Request) {
	h.responder.Document(writer, openAPIDocument)
}

func (h *DocumentationHandler) Reference(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	h.responder.Asset(writer, ContentTypeHTML, documentationHTML)
}

func (h *DocumentationHandler) ReferenceScript(writer http.ResponseWriter, _ *http.Request) {
	h.responder.Asset(writer, ContentTypeJavaScript, documentationScript)
}

func (h *DocumentationHandler) ScalarScript(writer http.ResponseWriter, _ *http.Request) {
	h.responder.Asset(writer, ContentTypeJavaScript, scalarScript)
}
