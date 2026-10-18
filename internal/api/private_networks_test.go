package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func privnetCall(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	rt.Handler().ServeHTTP(w, authedRequest(t, cookie, method, path, body))
	return w
}

func seedPrivnetApps(t *testing.T, db *store.DB, names ...string) {
	t.Helper()
	for _, n := range names {
		app := store.App{ID: "app_" + n, Name: n, CreatedAt: "2026-08-14T00:00:00Z", UpdatedAt: "2026-08-14T00:00:00Z"}
		if err := db.SaveApp(context.Background(), app); err != nil {
			t.Fatalf("SaveApp %s: %v", n, err)
		}
	}
}

func TestPrivateNetworks_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, path := range []string{"/api/v1/private-networks", "/api/v1/private-networks/map"} {
		if w := privnetCall(t, rt, cookie, http.MethodGet, path, ""); w.Code != http.StatusNotImplemented {
			t.Fatalf("GET %s = %d, want 501", path, w.Code)
		}
	}
	if w := privnetCall(t, rt, cookie, http.MethodPost, "/api/v1/private-networks", `{"name":"x"}`); w.Code != http.StatusNotImplemented {
		t.Fatalf("POST = %d, want 501", w.Code)
	}
}

func TestPrivateNetworks_CreateAndRules(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.SetPrivateNetworks(PrivateNetworkConfig{Store: db})
	seedPrivnetApps(t, db, "web", "api", "cache")

	if w := privnetCall(t, rt, cookie, http.MethodPost, "/api/v1/private-networks", `{"name":"Bad Name"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad name = %d", w.Code)
	}
	if w := privnetCall(t, rt, cookie, http.MethodPost, "/api/v1/private-networks", `{"name":"core","members":["ghost"]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown member = %d", w.Code)
	}
	created := privnetCall(t, rt, cookie, http.MethodPost, "/api/v1/private-networks", `{"name":"core","default_deny":true,"members":["web","api"]}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	if w := privnetCall(t, rt, cookie, http.MethodPost, "/api/v1/private-networks", `{"name":"core"}`); w.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d", w.Code)
	}

	var list privateNetworkListResponse
	w := privnetCall(t, rt, cookie, http.MethodGet, "/api/v1/private-networks", "")
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Networks) != 1 || len(list.Networks[0].Members) != 2 {
		t.Fatalf("list = %s err=%v", w.Body.String(), err)
	}

	rule := privnetCall(t, rt, cookie, http.MethodPost, "/api/v1/private-networks/rules", `{"from":"web","to":"cache"}`)
	if rule.Code != http.StatusCreated {
		t.Fatalf("rule create = %d %s", rule.Code, rule.Body.String())
	}
	var ruleResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rule.Body.Bytes(), &ruleResp); err != nil || ruleResp.ID == "" {
		t.Fatalf("rule resp = %s", rule.Body.String())
	}
	if w := privnetCall(t, rt, cookie, http.MethodPost, "/api/v1/private-networks/rules", `{"from":"web","to":"web"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("self rule = %d", w.Code)
	}
	if w := privnetCall(t, rt, cookie, http.MethodDelete, "/api/v1/private-networks/rules/"+ruleResp.ID, ""); w.Code != http.StatusOK {
		t.Fatalf("rule delete = %d", w.Code)
	}
	if w := privnetCall(t, rt, cookie, http.MethodDelete, "/api/v1/private-networks/rules/"+ruleResp.ID, ""); w.Code != http.StatusNotFound {
		t.Fatalf("second rule delete = %d, want 404", w.Code)
	}
}

func TestPrivateNetworks_MapVerdicts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.SetLocalNodeID("node-local")
	seedPrivnetApps(t, db, "web", "api", "cache", "remote")
	rt.SetPrivateNetworks(PrivateNetworkConfig{
		Store: db,
		Nodes: func(context.Context) (map[string]string, error) {
			return map[string]string{"app_web": "", "app_api": "node-local", "app_cache": "", "app_remote": "node-b"}, nil
		},
	})
	if w := privnetCall(t, rt, cookie, http.MethodPost, "/api/v1/private-networks", `{"name":"core","members":["web","api","remote"]}`); w.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", w.Code, w.Body.String())
	}
	if w := privnetCall(t, rt, cookie, http.MethodPost, "/api/v1/private-networks", `{"name":"deny","default_deny":true,"members":["cache"]}`); w.Code != http.StatusCreated {
		t.Fatalf("create deny = %d", w.Code)
	}

	var m privateNetworkMapResponse
	w := privnetCall(t, rt, cookie, http.MethodGet, "/api/v1/private-networks/map", "")
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	edge := func(a, b string) *privateNetworkMapEdge {
		for i := range m.Edges {
			if (m.Edges[i].A == a && m.Edges[i].B == b) || (m.Edges[i].A == b && m.Edges[i].B == a) {
				return &m.Edges[i]
			}
		}
		return nil
	}
	local := edge("app_web", "app_api")
	if local == nil || !local.Allowed || local.CrossNode {
		t.Fatalf("empty and explicit local node must not be cross-node: %+v", local)
	}
	if cross := edge("app_web", "app_remote"); cross == nil || !cross.Allowed || !cross.CrossNode {
		t.Fatalf("remote edge = %+v", cross)
	}
	if blocked := edge("app_web", "app_cache"); blocked == nil || blocked.Allowed || blocked.Fix != privateNetworkFixAllow {
		t.Fatalf("blocked edge = %+v", blocked)
	}
}
