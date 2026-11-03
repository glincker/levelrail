package authengine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicHandler_BlocksPasswordSetters(t *testing.T) {
	const prefix = DefaultPathPrefix
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := publicHandler(prefix, next)
	cases := []struct {
		path string
		want int
	}{
		{prefix + "/password/change", http.StatusNotFound},
		{prefix + "/password/set", http.StatusNotFound},
		{prefix + "/email-password/reset", http.StatusNotFound},
		{prefix + "/orgs", http.StatusNotFound},
		{prefix + "/email-password/forgot", http.StatusOK},
		{prefix + "/session", http.StatusOK},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.path, nil))
		if rec.Code != tc.want {
			t.Errorf("%s = %d, want %d", tc.path, rec.Code, tc.want)
		}
	}
}
