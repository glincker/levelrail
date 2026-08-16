package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// apiError is the JSON body every non-2xx response returns. A stable,
// predictable error shape matters here specifically because a later MCP
// layer is expected to parse these responses programmatically, not just
// a human reading them in a browser.
type apiError struct {
	Error string `json:"error"`
}

// writeJSON encodes v as the response body with the given status. A nil
// v writes only the status and headers, for handlers that have nothing
// to return (204 responses).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line and headers are already flushed by this point,
		// so there is nothing left to do but log; the client just gets a
		// truncated body.
		slog.Default().Error("api: encode response failed", slog.String("error", err.Error()))
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

// internalError logs err against context plus any extra fields, then
// writes this package's standard 500 "internal error" response: full
// detail server-side, a generic message client-side, one place instead
// of re-typing both halves at every call site.
func (rt *Router) internalError(w http.ResponseWriter, context string, err error, extra ...slog.Attr) {
	attrs := make([]any, 0, len(extra)+1)
	attrs = append(attrs, slog.String("error", err.Error()))
	for _, a := range extra {
		attrs = append(attrs, a)
	}
	rt.logger.Error(context, attrs...)
	writeError(w, http.StatusInternalServerError, "internal error")
}
