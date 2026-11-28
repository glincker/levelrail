package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

func TestHandleGetDatabaseEngineStats_Refusals(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for name, engine := range map[string]string{"main": store.EnginePostgres, "search": store.EngineMeilisearch} {
		if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: name, Engine: engine, Version: "v1.55"}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	tests := []struct {
		path string
		want int
	}{
		{"/api/v1/databases/missing/engine-stats", http.StatusNotFound},
		{"/api/v1/databases/main/engine-stats", http.StatusBadRequest},
		{"/api/v1/databases/search/engine-stats", http.StatusNotImplemented},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, tc.path, ""))
		if rec.Code != tc.want {
			t.Errorf("GET %s = %d, want %d (%s)", tc.path, rec.Code, tc.want, rec.Body.String())
		}
	}
}

func TestHandleCreateAppConnection_SearchEngineUsesConventionalNames(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "search", Engine: store.EngineMeilisearch, Version: "v1.55"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	for field, want := range map[string]string{"url": "MEILISEARCH_URL", "password": "MEILISEARCH_API_KEY"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"search","field":"`+field+`"}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("field %s: status = %d, body = %s", field, rec.Code, rec.Body.String())
		}
		var got appConnectionResource
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.EnvVar != want {
			t.Errorf("field %s: env_var = %q, want %q", field, got.EnvVar, want)
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"search","field":"url"}`))
	var second appConnectionResource
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	if second.EnvVar == "MEILISEARCH_URL" {
		t.Fatal("a second connection must not overwrite the conventional name")
	}
}

func TestHandleGetDatabaseExplorerCapabilities(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/explorer/capabilities", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Engine       string                           `json:"engine"`
		Kind         string                           `json:"kind"`
		Capabilities map[string]map[string]string     `json:"capabilities"`
		Limits       struct{ MaxRows, TimeoutMs int } `json:"limits"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Engine != "postgres" || got.Kind != "sql" || got.Capabilities["query"]["status"] != "available" {
		t.Errorf("shape = %+v", got)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/missing/explorer/capabilities", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing status = %d", rec.Code)
	}
}

func TestHandleGetDatabaseExplorerCapabilities_ShowsLiveLimits(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyDBViewerMaxRows, json.RawMessage(`123`), "test"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/explorer/capabilities", ""))
	var got struct {
		Limits struct {
			MaxRows int `json:"max_rows"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Limits.MaxRows != 123 {
		t.Fatalf("limits = %+v err=%v body=%s", got, err, rec.Body.String())
	}
}
