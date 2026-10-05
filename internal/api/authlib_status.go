package api

import (
	"net/http"
	"os"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

type authEngineStatusResponse struct {
	authengine.Status
	Areas []string `json:"areas"`
}

func (rt *Router) registerAuthEngineStatusRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth-engine/status", rt.requireAbility(AbilityRoot, rt.handleAuthEngineStatus))
}

// handleAuthEngineStatus handles GET /api/v1/auth-engine/status: the active
// mode, the areas served by the library, and the shadow comparison counters.
func (rt *Router) handleAuthEngineStatus(w http.ResponseWriter, _ *http.Request) {
	st := authengine.Status{Mode: authengine.Mode(), LibraryVersion: authengine.LibraryVersion(), Mismatches: []authengine.Mismatch{}}
	if rt.authLib.shadow != nil {
		snap := rt.authLib.shadow.Snapshot()
		snap.Mode, snap.LibraryVersion = st.Mode, st.LibraryVersion
		st = snap
	}
	areas := []string{}
	if authengine.Enabled() {
		parsed, err := authengine.ParseAreas(os.Getenv(authengine.EnvAreas))
		if err == nil {
			for _, a := range parsed {
				areas = append(areas, string(a))
			}
		}
	}
	writeJSON(w, http.StatusOK, authEngineStatusResponse{Status: st, Areas: areas})
}
