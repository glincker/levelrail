package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPanicRecoveryMiddleware_RecoversAndReturns500(t *testing.T) {
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("something went wrong")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)

	// Must not itself panic and crash the test: that's exactly the
	// behavior this middleware exists to prevent from reaching the
	// caller (net/http.Server's own request-serving goroutine).
	panicRecoveryMiddleware(discardLogger())(panicking).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	var got apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response body is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if got.Error == "" {
		t.Error("response body has no error message")
	}
}

func TestPanicRecoveryMiddleware_NoPanic_PassesThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	panicRecoveryMiddleware(discardLogger())(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequestIDMiddleware_GeneratesIDWhenAbsent(t *testing.T) {
	var gotFromContext string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotFromContext = requestIDFromContext(r.Context())
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	requestIDMiddleware(inner).ServeHTTP(rec, req)

	header := rec.Header().Get(requestIDHeader)
	if header == "" {
		t.Fatal("response has no X-Request-Id header")
	}
	if gotFromContext != header {
		t.Errorf("requestIDFromContext() = %q, want it to match the response header %q", gotFromContext, header)
	}
}

func TestRequestIDMiddleware_ReusesIncomingID(t *testing.T) {
	var gotFromContext string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotFromContext = requestIDFromContext(r.Context())
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	req.Header.Set(requestIDHeader, "req_from_upstream_proxy")
	requestIDMiddleware(inner).ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeader); got != "req_from_upstream_proxy" {
		t.Errorf("response X-Request-Id = %q, want the incoming value preserved", got)
	}
	if gotFromContext != "req_from_upstream_proxy" {
		t.Errorf("requestIDFromContext() = %q, want the incoming value", gotFromContext)
	}
}

func TestSecurityHeadersMiddleware_SetsExpectedHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	securityHeadersMiddleware(inner).ServeHTTP(rec, req)

	tests := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, want := range tests {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestHandleHealthz_OKWithNoAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rt.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
