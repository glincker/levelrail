package api

import "net/http"

// handleBrand serves GET /api/v1/brand: CLAUDE.md section 3's brand
// indirection endpoint. The frontend hydrates a React context from this
// on boot instead of ever hardcoding a product name string. Public, no
// auth required.
func (rt *Router) handleBrand(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, rt.brand)
}
