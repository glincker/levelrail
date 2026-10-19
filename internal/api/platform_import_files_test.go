package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppImportPlanFromPastedFiles(t *testing.T) {
	const composeFile = "services:\n  web:\n    image: nginx:1.27\n    ports:\n      - \"8080:80\"\n    environment:\n      API_KEY: s3cret\n  db:\n    image: postgres:16\n"
	cases := []struct {
		name     string
		body     map[string]string
		wantCode int
		wantText string
	}{
		{"compose maps app and database", map[string]string{"platform": "compose", "url": "file://compose", "snapshot": composeFile}, http.StatusOK, "web"},
		{"compose needs content", map[string]string{"platform": "compose", "url": "file://compose"}, http.StatusBadRequest, "snapshot is required"},
		{"compose rejects invalid yaml", map[string]string{"platform": "compose", "url": "file://compose", "snapshot": "services: [oops"}, http.StatusBadGateway, "compose"},
		{"dokku maps one app", map[string]string{"platform": "dokku", "url": "file://dokku", "app_name": "blog", "snapshot": "FOO=bar\nDOKKU_APP_TYPE=herokuish\n"}, http.StatusOK, "blog"},
		{"dokku needs app name", map[string]string{"platform": "dokku", "url": "file://dokku", "snapshot": "FOO=bar"}, http.StatusBadRequest, "app_name is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			raw, _ := json.Marshal(c.body)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/migration/apps/plan", string(raw)))
			if rec.Code != c.wantCode || !strings.Contains(rec.Body.String(), c.wantText) {
				t.Fatalf("status %d body %s, want %d containing %q", rec.Code, rec.Body.String(), c.wantCode, c.wantText)
			}
			if strings.Contains(rec.Body.String(), "s3cret") {
				t.Fatalf("plan leaked a secret value: %s", rec.Body.String())
			}
		})
	}
}
