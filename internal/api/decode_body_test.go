package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBody(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	cases := []struct {
		name       string
		body       string
		limit      int64
		wantOK     bool
		wantName   string
		wantStatus int
	}{
		{"valid", `{"name":"x"}`, 0, true, "x", http.StatusOK},
		{"unknown field accepted", `{"name":"x","extra":1}`, 0, true, "x", http.StatusOK},
		{"malformed", `{"name":`, 0, false, "", http.StatusBadRequest},
		{"empty", ``, 0, false, "", http.StatusBadRequest},
		{"over limit", `{"name":"abcdefghij"}`, 8, false, "", http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body))
			var got payload
			var ok bool
			if c.limit > 0 {
				ok = decodeReqLimit(rec, req, &got, c.limit)
			} else {
				ok = decodeReq(rec, req, &got)
			}
			if ok != c.wantOK || got.Name != c.wantName || rec.Code != c.wantStatus {
				t.Fatalf("ok=%v name=%q status=%d", ok, got.Name, rec.Code)
			}
			if !ok && !strings.Contains(rec.Body.String(), msgInvalidRequestBody) {
				t.Fatalf("body %q lacks the standard message", rec.Body.String())
			}
		})
	}
}
