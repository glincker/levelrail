package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResourceLabelsMergeFilterAndLimit(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedV2Apps(t, db)
	t.Setenv(envLabelsMaxPerResource, "3")

	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	if rec := do(http.MethodPatch, "/api/v1/apps/web/labels", `{"set":{"env":"prod","team":"core"}}`); rec.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPut, "/api/v1/apps/web/labels", `{}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"env":"prod"`) {
		t.Fatalf("omitted labels must keep: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPatch, "/api/v1/apps/web/labels", `{"set":{"a":"1","b":"2"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("over limit = %d", rec.Code)
	}
	if rec := do(http.MethodPatch, "/api/v1/apps/web/labels", `{"set":{"Bad Key":"x"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad key = %d", rec.Code)
	}

	rec := do(http.MethodGet, "/api/v2/apps?labelSelector=env%3Dprod", "")
	var list v2List
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Items) != 1 || list.Items[0].Metadata.Name != "web" {
		t.Fatalf("v2 selector: %s", rec.Body.String())
	}
	rec = do(http.MethodGet, "/api/v1/apps?label=env%3Dprod,team!%3Dother", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"web"`) || strings.Contains(rec.Body.String(), `"worker"`) {
		t.Fatalf("v1 selector: %d %s", rec.Code, rec.Body.String())
	}

	if rec := do(http.MethodPut, "/api/v1/apps/web/labels", `{"labels":{}}`); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "prod") {
		t.Fatalf("explicit empty must clear: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/api/v1/apps/nope/labels", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing app = %d", rec.Code)
	}
}
