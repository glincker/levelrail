package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestWriteErrorCodeAndRequestID(t *testing.T) {
	cases := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, "validation_failed"},
		{http.StatusUnprocessableEntity, "validation_failed"},
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "not_found"},
		{http.StatusMethodNotAllowed, "method_not_allowed"},
		{http.StatusConflict, "conflict"},
		{http.StatusTooManyRequests, "rate_limited"},
		{http.StatusServiceUnavailable, "unavailable"},
		{http.StatusBadGateway, errCodeInternal},
		{http.StatusTeapot, "request_failed"},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			rec := httptest.NewRecorder()
			rec.Header().Set(requestIDHeader, "req_abc")
			writeError(rec, tc.status, "boom")
			var got map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["error"] != "boom" || got["code"] != tc.code || got["request_id"] != "req_abc" {
				t.Fatalf("body = %v, want error=boom code=%s request_id=req_abc", got, tc.code)
			}
		})
	}
}

func TestWriteErrorOmitsRequestIDWhenUnset(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusNotFound, "gone")
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["request_id"]; ok || got["code"] != "not_found" {
		t.Fatalf("body = %v", got)
	}
}

func TestWriteErrorGeneric500KeepsTypedCode(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set(requestIDHeader, "req_x")
	writeError(rec, http.StatusInternalServerError, "internal error")
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["code"] != errCodeInternal || got["request_id"] != "req_x" {
		t.Fatalf("body = %v", got)
	}
}

var requestIDField = regexp.MustCompile(`,"request_id":"[^"]*"`)

// withoutRequestID drops the per-request id so bodies can be compared across requests.
func withoutRequestID(body string) string { return requestIDField.ReplaceAllString(body, "") }
