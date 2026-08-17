package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestHandleListServiceTemplates(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []serviceTemplateListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != len(catalog.Templates) {
		t.Fatalf("got %d templates, want %d", len(got), len(catalog.Templates))
	}
	for _, item := range got {
		if item.ID == "" || item.Name == "" || item.Slogan == "" || item.Category == "" || item.DocumentationURL == "" {
			t.Errorf("template %+v has an empty required field", item)
		}
	}

	body := rec.Body.String()
	if strings.Contains(body, `"compose"`) {
		t.Error("list response contains a compose field, want it omitted from the list shape")
	}
}

func TestServiceTemplatesListRoute_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/service-templates", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetServiceTemplate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	want := catalog.Templates[0]
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates/"+want.ID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got serviceTemplateDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Compose != want.Compose {
		t.Errorf("Compose does not match the catalog entry for %q", want.ID)
	}
}

func TestHandleGetServiceTemplate_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates/does-not-exist", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestServiceTemplateDetailRoute_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/service-templates/n8n", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}
