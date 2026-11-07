package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBootstrapScriptHandlers(t *testing.T) {
	t.Setenv(envBootstrapScriptMaxBytes, "32")
	t.Setenv(envBootstrapScriptMaxCount, "2")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) (int, bootstrapScriptResource) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		var out bootstrapScriptResource
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	code, created := do(http.MethodPost, "/api/v1/bootstrap-scripts", `{"name":"base","body":"echo hi"}`)
	if code != http.StatusCreated || created.ID == "" || created.Body != "echo hi" {
		t.Fatalf("create = %d %+v", code, created)
	}
	if code, _ := do(http.MethodPost, "/api/v1/bootstrap-scripts", `{"name":"base","body":"echo again"}`); code != http.StatusConflict {
		t.Fatalf("duplicate name = %d, want 409", code)
	}
	if code, _ := do(http.MethodPost, "/api/v1/bootstrap-scripts", `{"name":"big","body":"`+strings.Repeat("a", 33)+`"}`); code != http.StatusBadRequest {
		t.Fatalf("oversize body = %d, want 400", code)
	}
	if code, _ := do(http.MethodPost, "/api/v1/bootstrap-scripts", `{"name":"two","body":"echo 2"}`); code != http.StatusCreated {
		t.Fatalf("second create = %d", code)
	}
	if code, _ := do(http.MethodPost, "/api/v1/bootstrap-scripts", `{"name":"three","body":"echo 3"}`); code != http.StatusConflict {
		t.Fatalf("over count limit = %d, want 409", code)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/bootstrap-scripts", ""))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "echo hi") {
		t.Fatalf("list = %d, body must not leak script text: %s", rec.Code, rec.Body.String())
	}

	if code, upd := do(http.MethodPut, "/api/v1/bootstrap-scripts/"+created.ID, `{"name":"base","body":"echo v2"}`); code != http.StatusOK || upd.Body != "echo v2" {
		t.Fatalf("update = %d %+v", code, upd)
	}
	if code, _ := do(http.MethodDelete, "/api/v1/bootstrap-scripts/"+created.ID, ""); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	if code, _ := do(http.MethodGet, "/api/v1/bootstrap-scripts/"+created.ID, ""); code != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", code)
	}
}
