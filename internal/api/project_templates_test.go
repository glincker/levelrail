package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProjectTemplates_ListAndRender(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/project-templates", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	var list struct {
		Templates []struct{ ID string } `json:"templates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Templates) < 5 {
		t.Fatalf("want at least 5 templates, got %v (%v)", list.Templates, err)
	}

	tests := []struct {
		name   string
		id     string
		body   string
		status int
		want   string
	}{
		{"valid", "web-service", `{"params":{"name":"shop","port":"8080"}}`, http.StatusOK, "port: 8080"},
		{"invalid port", "web-service", `{"params":{"name":"shop","port":"x"}}`, http.StatusUnprocessableEntity, `"param":"port"`},
		{"unknown template", "nope", `{"params":{}}`, http.StatusNotFound, ""},
		{"bad body", "web-service", `{`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/project-templates/"+tt.id+"/render", tt.body))
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.status, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("body %q missing %q", rec.Body.String(), tt.want)
			}
		})
	}
}
