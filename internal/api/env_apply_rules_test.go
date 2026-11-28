package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestEnvApplyUsesSharedValidator(t *testing.T) {
	f := newEnvFixture(t)
	const path = "/api/v1/apps/web/env/apply"
	tests := []struct {
		name, body, wantKey string
		wantStatus          int
	}{
		{"digit start", `{"set":{"1BAD":"x"}}`, "1BAD", http.StatusBadRequest},
		{"new dotted key", `{"set":{"A.B":"x"}}`, "A.B", http.StatusBadRequest},
		{"reserved prefix", `{"set":{"LEVELRAIL_X":"x"}}`, "LEVELRAIL_X", http.StatusBadRequest},
		{"nul value", `{"set":{"GOOD":"a\u0000b"}}`, "GOOD", http.StatusBadRequest},
		{"secret previous suffix", `{"secret_set":{"TOKEN_PREVIOUS":"x"}}`, "TOKEN_PREVIOUS", http.StatusBadRequest},
		{"unset equals", `{"unset":["A=B"]}`, "A=B", http.StatusBadRequest},
		{"valid key still works", `{"set":{"GOOD_KEY":"x"}}`, "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := f.do(t, f.cookie, http.MethodPost, path, tt.body, tt.wantStatus)
			if tt.wantKey != "" && !strings.Contains(rec.Body.String(), tt.wantKey) {
				t.Fatalf("body %s should name %q", rec.Body.String(), tt.wantKey)
			}
		})
	}
}
