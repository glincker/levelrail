package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeployURLCopiesSettingHandlers(t *testing.T) {
	t.Setenv("APP_DEPLOY_URL_COPIES", "")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, body string) (int, deployURLCopiesResource) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/settings/deploy-url-copies", body))
		var out deployURLCopiesResource
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	if code, got := do(http.MethodGet, ""); code != http.StatusOK || got.Mode != "private" || got.Source != "default" {
		t.Fatalf("fresh = %d %+v, want private from default", code, got)
	}
	if code, got := do(http.MethodPut, `{"mode":"public"}`); code != http.StatusOK || got.Mode != "public" || got.Source != "stored" {
		t.Fatalf("set = %d %+v", code, got)
	}
	if code, _ := do(http.MethodPut, `{"mode":"everywhere"}`); code != http.StatusBadRequest {
		t.Fatalf("invalid mode = %d, want 400", code)
	}
	if _, got := do(http.MethodPut, `{"mode":""}`); got.Source != "default" || got.Mode != "private" {
		t.Fatalf("cleared = %+v, want default private", got)
	}
}
