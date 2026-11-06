package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
)

// handleListStatusReasons handles GET /api/v1/status-reasons: every reason a
// deploy can fail or roll back, with a plain meaning and the next step.
func (rt *Router) handleListStatusReasons(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, deploysafety.ReasonCatalog())
}
