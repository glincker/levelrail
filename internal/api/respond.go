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
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

// errorCodeForStatus gives every plain writeError a stable machine code
// derived from the HTTP status, so clients can branch without parsing text.
func errorCodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "validation_failed"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict, http.StatusPreconditionFailed, http.StatusPreconditionRequired:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusServiceUnavailable:
		return "unavailable"
	}
	if status >= 500 {
		return errCodeInternal
	}
	return "request_failed"
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
	if status == http.StatusInternalServerError && msg == "internal error" {
		m := "Something went wrong on the server."
		writeJSON(w, status, typedError{Error: m, Code: errCodeInternal, Message: m, RequestID: w.Header().Get(requestIDHeader)})
		return
	}
	writeJSON(w, status, apiError{Error: msg, Code: errorCodeForStatus(status), RequestID: w.Header().Get(requestIDHeader)})
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
	reqID := w.Header().Get(requestIDHeader)
	attrs = append(attrs, slog.String("request_id", reqID))
	if t := classifyError(err); t != nil {
		rt.logger.Warn(context, append(attrs, slog.String("code", t.Code))...)
		t.RequestID = reqID
		writeJSON(w, t.status, t)
		return
	}
	rt.logger.Error(context, attrs...)
	msg := "Something went wrong on the server."
	writeJSON(w, http.StatusInternalServerError, typedError{Error: msg, Code: errCodeInternal, Message: msg, RequestID: reqID})
}
