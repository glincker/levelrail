package ingress

import "net/http"

// Unmatched host modes mirror the ingress.unmatched_mode runtime setting.
const (
	UnmatchedModeDiagnostic = "diagnostic"
	UnmatchedModeNotFound   = "not_found"
	UnmatchedModeRedirect   = "redirect"
)

// unmatchedCatchAll builds the last route for the chosen mode. ok is false for diagnostic mode,
// a redirect without a target, or any unknown value, so the diagnostic page stays the safe default.
func unmatchedCatchAll(h *Hardening) (Route, bool) {
	switch h.UnmatchedMode {
	case UnmatchedModeNotFound:
		return Route{Handle: []any{StaticResponseHandler{
			Handler: "static_response", StatusCode: http.StatusNotFound,
			Headers: map[string][]string{"Cache-Control": {"no-store"}},
		}}}, true
	case UnmatchedModeRedirect:
		if h.UnmatchedRedirectURL == "" {
			return Route{}, false
		}
		hd := NewRedirectResponseHandler(h.UnmatchedRedirectURL, http.StatusFound)
		hd.Headers["Cache-Control"] = []string{"no-store"}
		return Route{Handle: []any{hd}}, true
	}
	return Route{}, false
}
