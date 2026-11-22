package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/fielddesc"
)

// handleGetFieldDescriptors handles GET /api/v1/field-descriptors/{resource}: every registered
// resource is served here, so a new descriptor file needs no route of its own.
func (rt *Router) handleGetFieldDescriptors(w http.ResponseWriter, r *http.Request) {
	res, ok := fielddesc.Get(r.PathValue("resource"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown field descriptor resource")
		return
	}
	writeJSON(w, http.StatusOK, res)
}
