package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func consoleCall(t *testing.T, rt *Router, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	rt.Handler().ServeHTTP(w, authedRequest(t, cookie, method, path, body))
	return w
}

func TestResourceConsole_ListTagAndCostGroup(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	seedScheduledTaskApp(t, db, "worker")

	dry := consoleCall(t, rt, http.MethodPost, "/api/v1/resources/tags/bulk",
		`{"action":"add","tag":"team:shop","resources":[{"kind":"app","name":"web"},{"kind":"app","name":"ghost"}],"dry_run":true}`, cookie)
	if dry.Code != http.StatusMultiStatus {
		t.Fatalf("dry run status = %d body=%s", dry.Code, dry.Body.String())
	}
	var resp consoleBulkTagResponse
	if err := json.Unmarshal(dry.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Counts[bulkStatusWouldRun] != 1 || resp.Counts[bulkStatusNotFound] != 1 {
		t.Fatalf("dry run counts = %v", resp.Counts)
	}
	tagged := consoleCall(t, rt, http.MethodGet, "/api/v1/resources?tag=team:shop", "", cookie)
	if strings.Contains(tagged.Body.String(), `"web"`) {
		t.Fatalf("dry run must not tag anything: %s", tagged.Body.String())
	}

	applied := consoleCall(t, rt, http.MethodPost, "/api/v1/resources/tags/bulk",
		`{"action":"add","tag":"team:shop","resources":[{"kind":"app","name":"web"}]}`, cookie)
	if applied.Code != http.StatusMultiStatus || !strings.Contains(applied.Body.String(), `"ok"`) {
		t.Fatalf("apply = %d %s", applied.Code, applied.Body.String())
	}

	var list consoleListResponse
	w := consoleCall(t, rt, http.MethodGet, "/api/v1/resources?q=we&kind=app&tag=team:shop", "", cookie)
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || list.Items[0].Name != "web" || list.Counts["app"] != 1 {
		t.Fatalf("list = %+v", list)
	}
}

func TestResourceConsole_SavedViews(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	created := consoleCall(t, rt, http.MethodPost, "/api/v1/resources/views", `{"name":"prod","query":"{\"q\":\"api\"}"}`, cookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var v resourceViewResource
	if err := json.Unmarshal(created.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if bad := consoleCall(t, rt, http.MethodPost, "/api/v1/resources/views", `{"name":"x","query":"nope"}`, cookie); bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid query = %d", bad.Code)
	}
	if del := consoleCall(t, rt, http.MethodDelete, "/api/v1/resources/views/"+v.ID, "", cookie); del.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", del.Code)
	}
	if del := consoleCall(t, rt, http.MethodDelete, "/api/v1/resources/views/"+v.ID, "", cookie); del.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", del.Code)
	}
}
