package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
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

func TestResourceConsole_NodeAndNetworkTagsSurviveRename(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.SetPrivateNetworks(PrivateNetworkConfig{Store: db})
	ctx := context.Background()
	if err := db.SaveNode(ctx, store.Node{ID: "n1", Name: "edge-a", Status: store.NodeStatusOnline, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}
	if err := db.SavePrivateNetwork(ctx, store.PrivateNetwork{ID: "pn1", Name: "core", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("SavePrivateNetwork: %v", err)
	}

	list := func(query string) consoleListResponse {
		var out consoleListResponse
		w := consoleCall(t, rt, http.MethodGet, "/api/v1/resources"+query, "", cookie)
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if all := list(""); all.Counts["node"] != 1 || all.Counts["network"] != 1 {
		t.Fatalf("counts = %v", all.Counts)
	}
	w := consoleCall(t, rt, http.MethodPost, "/api/v1/resources/tags/bulk",
		`{"action":"add","tag":"env:prod","resources":[{"kind":"node","name":"edge-a"},{"kind":"network","name":"core"}]}`, cookie)
	var resp consoleBulkTagResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Counts[bulkStatusOK] != 2 || resp.Results[0].Name != "edge-a" {
		t.Fatalf("bulk = %s err=%v", w.Body.String(), err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE nodes SET name = 'edge-b' WHERE id = 'n1'`); err != nil {
		t.Fatal(err)
	}
	tagged := list("?tag=env:prod&kind=node")
	if tagged.Total != 1 || tagged.Items[0].Name != "edge-b" {
		t.Fatalf("renamed node lost its tag: %+v", tagged)
	}
}
