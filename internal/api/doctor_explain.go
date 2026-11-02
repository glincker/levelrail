package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/fixit"
)

// handleDoctorExplain handles GET /api/v1/doctor/explain?id=<check-id>. The text is a
// fixed table, so it needs no network and carries no per-install data.
func (rt *Router) handleDoctorExplain(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusOK, doctorExplainList{Explanations: allDoctorExplanations()})
		return
	}
	e, ok := fixit.Explain(id)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown check id")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

type doctorExplainList struct {
	Explanations []fixit.Explanation `json:"explanations"`
}

func allDoctorExplanations() []fixit.Explanation {
	ids := fixit.ExplainIDs()
	out := make([]fixit.Explanation, 0, len(ids))
	for _, id := range ids {
		e, _ := fixit.Explain(id)
		out = append(out, e)
	}
	return out
}
