package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimitMiddleware(t *testing.T) {
	t.Setenv(envMaxBodyBytes, "")
	h := (&Router{}).bodyLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	}))
	cases := []struct {
		name, path string
		size       int
		want       int
	}{
		{"pre-auth over cap", "/api/v1/auth/login", 2 << 20, http.StatusRequestEntityTooLarge},
		{"pre-auth under cap", "/api/v1/auth/login", 512 << 10, http.StatusOK},
		{"authed under backstop", "/api/v1/apps", 2 << 20, http.StatusOK},
		{"authed over backstop", "/api/v1/apps", 17 << 20, http.StatusRequestEntityTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, c.path, strings.NewReader(strings.Repeat("a", c.size))))
			if rec.Code != c.want {
				t.Errorf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}
