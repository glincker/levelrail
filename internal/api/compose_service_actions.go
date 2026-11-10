package api

import (
	"log/slog"
	"net/http"
	"strings"
)

// composeServiceAction runs a single-app handler against one member of a compose stack.
// The member is looked up inside the stack, so a key from another app can never be reached.
func (rt *Router) composeServiceAction(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		appID, key := r.PathValue("name"), r.PathValue("service")
		services, err := rt.appGroups.ListServicesByApp(r.Context(), appID)
		if err != nil {
			rt.internalError(w, "api: compose: list services failed", err, slog.String("app", appID))
			return
		}
		for _, s := range services {
			if strings.TrimPrefix(s.Name, appID+"-") == key {
				r.SetPathValue("name", s.Name)
				next(w, r)
				return
			}
		}
		writeError(w, http.StatusNotFound, "compose service not found")
	}
}
