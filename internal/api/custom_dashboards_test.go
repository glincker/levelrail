package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const dashDoc = `{"name":"Web","widgets":[{"id":"cpu","type":"metric_chart","size":"M","metric":"cpu_percent","target":{"kind":"app","name":"web"}}]}`

func TestDashboardsCRUD(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := do(http.MethodPost, "/api/v1/dashboards", `{"scope":"shared","project_id":"p1","is_default":true,"doc":`+dashDoc+`}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created dashboardResource
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil || created.ID == "" || !created.IsDefault {
		t.Fatalf("created = %+v %v", created, err)
	}

	tests := []struct {
		name, body string
		want       int
	}{
		{"duplicate name", `{"scope":"shared","doc":` + dashDoc + `}`, http.StatusConflict},
		{"bad scope", `{"scope":"team","doc":` + dashDoc + `}`, http.StatusBadRequest},
		{"default needs project", `{"scope":"shared","is_default":true,"doc":{"name":"x","widgets":[]}}`, http.StatusBadRequest},
		{"missing target", `{"doc":{"name":"x","widgets":[{"id":"a","type":"log_tail","size":"S"}]}}`, http.StatusBadRequest},
		{"duplicate widget id", `{"doc":{"name":"y","widgets":[{"id":"a","type":"resource_counts","size":"S"},{"id":"a","type":"resource_counts","size":"S"}]}}`, http.StatusBadRequest},
		{"private ok", `{"doc":{"name":"mine","widgets":[]}}`, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := do(http.MethodPost, "/api/v1/dashboards", tt.body); rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}

	rec = do(http.MethodGet, "/api/v1/dashboards?project=p1", "")
	var list []dashboardResource
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil || len(list) != 1 || list[0].Doc.Widgets[0].ID != "cpu" {
		t.Fatalf("list = %+v %v", list, err)
	}

	rec = do(http.MethodPut, "/api/v1/dashboards/"+created.ID, `{"scope":"shared","project_id":"p1","doc":{"name":"Web v2","widgets":[]}}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Web v2") {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(http.MethodGet, "/api/v1/dashboards/"+created.ID+"/export", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version":1`) {
		t.Fatalf("export = %d: %s", rec.Code, rec.Body.String())
	}

	if rec = do(http.MethodDelete, "/api/v1/dashboards/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec = do(http.MethodGet, "/api/v1/dashboards/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete = %d", rec.Code)
	}
}

func TestDashboardsImportAndTemplates(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	if rec := do(http.MethodPost, "/api/v1/dashboards/import", `{"version":2,"name":"x","widgets":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("future version = %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/dashboards/import", `{"version":1,"name":"imp","widgets":[{"id":"n","type":"node_capacity","size":"L","target":{"kind":"node","name":"n1"}}]}`); rec.Code != http.StatusCreated {
		t.Fatalf("import = %d: %s", rec.Code, rec.Body.String())
	}
	rec := do(http.MethodGet, "/api/v1/dashboards/templates", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "app-health") {
		t.Fatalf("templates = %d: %s", rec.Code, rec.Body.String())
	}
}
