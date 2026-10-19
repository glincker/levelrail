package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestHandleGetServiceTemplate_Requirements(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates/"+catalog.Templates[0].ID, ""))
	var got serviceTemplateDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Requirements.Level == "" || len(got.Requirements.Protections) == 0 {
		t.Errorf("requirements not populated: %+v", got.Requirements)
	}
}

func TestComposeNeedsConfiguration_ElevatedTemplate(t *testing.T) {
	body := "services:\n  a:\n    image: foo/a:1\n    privileged: true\n"
	if !composeNeedsConfiguration(body) {
		t.Error("an elevated template must not take the one-click path")
	}
	if composeNeedsConfiguration("services:\n  a:\n    image: foo/a:1\n") {
		t.Error("a plain template should stay one-click")
	}
}
