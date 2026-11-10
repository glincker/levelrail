package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestListNodeResources(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedNode(t, db, "node_1", "worker-1")
	seedNode(t, db, "node_2", "worker-2")
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000, Resources: &store.ServiceResources{MemoryBytes: 512 << 20}}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node_1"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main-db", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateDatabaseNode(ctx, "main-db", "node_1"); err != nil {
		t.Fatal(err)
	}

	get := func(path string) (int, nodeResourcesResponse) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		var got nodeResourcesResponse
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
		}
		return rec.Code, got
	}

	code, got := get("/api/v1/nodes/node_1/resources")
	if code != http.StatusOK || len(got.Resources) != 2 {
		t.Fatalf("node_1 = %d %+v", code, got)
	}
	kinds := map[string]nodeResourceItem{}
	for _, r := range got.Resources {
		kinds[r.Kind] = r
	}
	if a := kinds[nodeResourceKindApp]; a.Name != "web" || a.MemoryBytes != 512<<20 || a.Status != nodeResourceActive {
		t.Fatalf("app row = %+v", a)
	}
	if d := kinds[nodeResourceKindDatabase]; d.Name != "main-db" || !strings.Contains(d.Detail, "postgres") {
		t.Fatalf("database row = %+v", d)
	}

	code, got = get("/api/v1/nodes/node_2/resources")
	if code != http.StatusOK || got.Resources == nil || len(got.Resources) != 0 {
		t.Fatalf("empty node = %d %+v", code, got)
	}
	if got.NodeID != "node_2" {
		t.Fatalf("node id = %q", got.NodeID)
	}
	if code, _ = get("/api/v1/nodes/missing/resources"); code != http.StatusNotFound {
		t.Fatalf("missing node = %d", code)
	}
}

func TestListNodeResourcesHidesUnreadable(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedNode(t, db, "node_1", "worker-1")
	for _, app := range []string{"alpha-app", "beta-app"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: "img:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateServiceNode(ctx, app, "node_1"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "beta-db", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateDatabaseNode(ctx, "beta-db", "node_1"); err != nil {
		t.Fatal(err)
	}
	tok := seedMatrixToken(t, db, "scoped-tok", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-beta-app", "Deny", "*", "app:beta-app", store.PrincipalTypeToken, "scoped-tok")
	attachTestPolicy(t, db, "deny-beta-db", "Deny", "*", "database:beta-db", store.PrincipalTypeToken, "scoped-tok")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node_1/resources", nil)
	bearer(tok)(req)
	rt.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "alpha-app") {
		t.Fatalf("scoped = %d %s", rec.Code, body)
	}
	if strings.Contains(body, "beta-app") || strings.Contains(body, "beta-db") {
		t.Fatalf("scoped body leaks hidden resources: %s", body)
	}
	var got nodeResourcesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Resources) != 1 {
		t.Fatalf("scoped count = %+v, %v; want only alpha-app", got.Resources, err)
	}
}

func TestListNodeResourcesLocalNode(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.localNodeID = "node_local"
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedNode(t, db, "node_local", "control")
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main-db", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_local/resources", ""))
	var got nodeResourcesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("local node = %d %s %v", rec.Code, rec.Body.String(), err)
	}
	if len(got.Resources) != 2 {
		t.Fatalf("local node resources = %+v, want the app and database stored with an empty node id", got.Resources)
	}
}

func TestListNodeResourcesTenantForbidden(t *testing.T) {
	f := newOrgFixture(t)
	seedNode(t, f.db, "node_shared", "shared")
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{"org tenant", f.bOwner, http.StatusForbidden},
		{"root", f.root, http.StatusOK},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node_shared/resources", nil)
		withCookie(tc.cookie)(req)
		f.rt.Handler().ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s = %d %s, want %d", tc.name, rec.Code, rec.Body.String(), tc.want)
		}
	}
}
