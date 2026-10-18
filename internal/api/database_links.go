package api

import (
	"encoding/json"
	"net/http"
	"sort"
)

type databaseLink struct {
	App    string `json:"app"`
	EnvVar string `json:"env_var"`
	Field  string `json:"field"`
}

type connectDatabaseRequest struct {
	App    string `json:"app"`
	EnvVar string `json:"env_var"`
	Field  string `json:"field"`
}

// handleListDatabaseLinks handles GET /api/v1/databases/{name}/links: every
// app env var that resolves from this database (connections and the legacy
// single attachment), limited to apps the caller may read.
func (rt *Router) handleListDatabaseLinks(w http.ResponseWriter, r *http.Request) {
	dbName := r.PathValue("name")
	services, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: list database links failed", err)
		return
	}
	links := make([]databaseLink, 0)
	for i := range services {
		var found []databaseLink
		for envVar, ref := range services[i].DatabaseEnv {
			if ref.Database == dbName {
				found = append(found, databaseLink{App: services[i].Name, EnvVar: envVar, Field: ref.Field})
			}
		}
		if att := services[i].DatabaseAttachment; att != nil && att.DatabaseName == dbName {
			found = append(found, databaseLink{App: services[i].Name, EnvVar: att.EnvVar, Field: att.Field})
		}
		if len(found) == 0 {
			continue
		}
		denied, err := rt.firstDeniedRef(r, AbilityRead, appRefs([]string{services[i].Name}))
		if err != nil {
			rt.internalError(w, "api: list database links authz failed", err)
			return
		}
		if denied == nil {
			links = append(links, found...)
		}
	}
	sort.Slice(links, func(a, b int) bool {
		if links[a].App != links[b].App {
			return links[a].App < links[b].App
		}
		return links[a].EnvVar < links[b].EnvVar
	})
	writeJSON(w, http.StatusOK, map[string][]databaseLink{"links": links})
}

// handleConnectDatabaseApp handles POST /api/v1/databases/{name}/links: the
// database-side one-click form of POST /api/v1/apps/{app}/connections.
func (rt *Router) handleConnectDatabaseApp(w http.ResponseWriter, r *http.Request) {
	var req connectDatabaseRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || req.App == "" {
		writeError(w, http.StatusBadRequest, "app is required")
		return
	}
	denied, err := rt.firstDeniedRef(r, AbilityWrite, appRefs([]string{req.App}))
	if err != nil {
		rt.internalError(w, "api: connect database authz failed", err)
		return
	}
	if denied != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	rt.createAppConnection(w, r, req.App, createAppConnectionRequest{Database: r.PathValue("name"), Field: req.Field, EnvVar: req.EnvVar})
}
