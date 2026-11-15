package api

import (
	"bytes"
	"encoding"
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
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
	if err := encodeJSON(w, v); err != nil {
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

// encodeJSON writes v, and when the output holds a null it re-encodes a
// copy with nil slices and maps emptied, so Go nils never reach the
// dashboard (which calls .length and .map on them). The null check keeps
// the reflection walk off the common path.
func encodeJSON(w http.ResponseWriter, v any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		return err
	}
	if bytes.Contains(buf.Bytes(), []byte("null")) {
		rv := reflect.ValueOf(v)
		if fixed, ok := emptyNotNull(rv, false); ok {
			buf.Reset()
			if err := json.NewEncoder(&buf).Encode(fixed.Interface()); err != nil {
				return err
			}
		}
	}
	_, err := w.Write(buf.Bytes())
	return err
}

var (
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

func hasOwnMarshaler(t reflect.Type) bool {
	return t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType) ||
		t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)
}

// emptyNotNull returns a copy of v where nil slices and maps are empty,
// and false when nothing changed. omitempty fields stay omitted, and
// types with their own marshaler are left alone.
func emptyNotNull(v reflect.Value, keepNil bool) (reflect.Value, bool) {
	if !v.IsValid() {
		return v, false
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return v, false
		}
		return emptyNotNull(v.Elem(), keepNil)
	case reflect.Pointer:
		if v.IsNil() || hasOwnMarshaler(v.Type().Elem()) {
			return v, false
		}
		inner, ok := emptyNotNull(v.Elem(), false)
		if !ok {
			return v, false
		}
		p := reflect.New(v.Type().Elem())
		p.Elem().Set(inner)
		return p, true
	case reflect.Slice:
		if v.IsNil() {
			if keepNil || hasOwnMarshaler(v.Type()) {
				return v, false
			}
			return reflect.MakeSlice(v.Type(), 0, 0), true
		}
		if !canHoldNil(v.Type().Elem()) || hasOwnMarshaler(v.Type()) {
			return v, false
		}
		var out reflect.Value
		for i := 0; i < v.Len(); i++ {
			if e, ok := emptyNotNull(v.Index(i), false); ok {
				if !out.IsValid() {
					out = reflect.MakeSlice(v.Type(), v.Len(), v.Len())
					reflect.Copy(out, v)
				}
				out.Index(i).Set(e)
			}
		}
		return out, out.IsValid()
	case reflect.Map:
		if v.IsNil() {
			if keepNil || hasOwnMarshaler(v.Type()) {
				return v, false
			}
			return reflect.MakeMap(v.Type()), true
		}
		if !canHoldNil(v.Type().Elem()) || hasOwnMarshaler(v.Type()) {
			return v, false
		}
		var out reflect.Value
		iter := v.MapRange()
		for iter.Next() {
			if e, ok := emptyNotNull(iter.Value(), false); ok {
				if !out.IsValid() {
					out = reflect.MakeMapWithSize(v.Type(), v.Len())
					for it2 := v.MapRange(); it2.Next(); {
						out.SetMapIndex(it2.Key(), it2.Value())
					}
				}
				out.SetMapIndex(iter.Key(), e)
			}
		}
		return out, out.IsValid()
	case reflect.Struct:
		if hasOwnMarshaler(v.Type()) {
			return v, false
		}
		return fixStruct(v)
	}
	return v, false
}

// canHoldNil reports whether a value of type t can contain a nil slice
// or map, so plain slices of numbers or strings are skipped quickly.
func canHoldNil(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Struct:
		return true
	}
	return false
}

func fixStruct(sv reflect.Value) (reflect.Value, bool) {
	t := sv.Type()
	var cp reflect.Value
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		keep := strings.Contains(tag, "omitempty") || strings.Contains(tag, "omitzero")
		fv := sv.Field(i)
		if fv.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.Uint8 && fv.IsNil() {
			continue
		}
		e, ok := emptyNotNull(fv, keep)
		if !ok {
			continue
		}
		if !cp.IsValid() {
			cp = reflect.New(t).Elem()
			cp.Set(sv)
		}
		cp.Field(i).Set(e)
	}
	return cp, cp.IsValid()
}
