package api

import (
	"context"
	"net/http"
	"strings"
)

// writeAppNotFound answers 404 and, when name is a stack with services, lists
// the real app names (apps are named <stack>-<service>).
func (rt *Router) writeAppNotFound(ctx context.Context, w http.ResponseWriter, name string) {
	msg := "app not found"
	if svcs, err := rt.apps.ListDesiredServices(ctx); err == nil && name != "" {
		var matches []string
		for _, s := range svcs {
			if strings.HasPrefix(s.Name, name+"-") {
				matches = append(matches, s.Name)
			}
		}
		if len(matches) > 0 {
			msg += ": apps are named <stack>-<service>, and " + name + " looks like a stack; did you mean " + strings.Join(matches, ", ") + "?"
		} else {
			msg += ": apps are named <stack>-<service> for compose stacks (see 'apps list')"
		}
	}
	writeError(w, http.StatusNotFound, msg)
}
