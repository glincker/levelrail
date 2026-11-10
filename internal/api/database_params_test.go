package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDatabaseParams_StagedNotApplied(t *testing.T) {
	rt, db, _ := newAccessTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "pg", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cookie := loginTestSession(t, rt, db)

	rec := viewerDo(t, rt, cookie, http.MethodPut, "/api/v1/databases/pg/params", `{"set":{"shared_buffers":"256mb","max_connections":"200"}}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"has_pending":true`) {
		t.Fatalf("stage = %d %s", rec.Code, rec.Body.String())
	}
	d, err := db.GetDesiredDatabase(ctx, "pg")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if d.EngineParams != "" {
		t.Fatalf("staging changed the applied set: %v", d.EngineParams)
	}

	for _, body := range []string{`{"set":{"listen_addresses":"*"}}`, `{"set":{"work_mem":"4MB\nssl=off"}}`, `{"set":{"max_connections":"999999999"}}`} {
		if rec := viewerDo(t, rt, cookie, http.MethodPut, "/api/v1/databases/pg/params", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", body, rec.Code)
		}
	}
	if rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/pg/params/apply", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("apply without confirm = %d", rec.Code)
	}
	if rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/pg/params/apply", `{"confirm":true}`); rec.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	d, _ = db.GetDesiredDatabase(ctx, "pg")
	if applied := d.EngineParams.Map(); applied["shared_buffers"] != "256MB" || applied["max_connections"] != "200" {
		t.Fatalf("applied = %v", applied)
	}
}

func TestDatabaseParams_ResourceScoped(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "prod-main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	writer := storeUserWithAbilitiesForTest(t, db, "writer@example.com", []string{AbilityRead, AbilityWrite})
	attachTestPolicy(t, db, "deny-prod-db", "Deny", "*", "database:prod-main", store.PrincipalTypeUser, writer.ID)
	cookie := sessionCookieForTest(t, rt, writer.ID)

	// A policy deny is the shared resource gate's 403; an unknown name is 404.
	for path, want := range map[string]int{"/api/v1/databases/prod-main/params": http.StatusForbidden, "/api/v1/databases/missing/params": http.StatusNotFound} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, path, `{"set":{"work_mem":"8MB"}}`))
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", path, rec.Code, want)
		}
	}
	p, err := db.GetDatabaseEngineParams(ctx, "prod-main")
	if err != nil || p.Pending != nil {
		t.Fatalf("denied caller staged a change: %+v %v", p, err)
	}
}

func TestDatabaseParams_CompareAndSetConflicts(t *testing.T) {
	rt, db, _ := newAccessTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "pg", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.StageDatabaseEngineParams(ctx, "pg", map[string]string{"work_mem": "8MB"}, "a", nil); err != nil {
		t.Fatalf("first stage: %v", err)
	}
	// A second writer that read "nothing staged" loses.
	if err := db.StageDatabaseEngineParams(ctx, "pg", map[string]string{"work_mem": "16MB"}, "b", nil); !errors.Is(err, store.ErrEngineParamsConflict) {
		t.Fatalf("stale stage err = %v, want conflict", err)
	}
	read, _ := db.GetDatabaseEngineParams(ctx, "pg")
	// A PUT lands between the apply's read and its write.
	if err := db.StageDatabaseEngineParams(ctx, "pg", map[string]string{"work_mem": "32MB"}, "c", read.PendingRaw); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if ok, err := db.ApplyDatabaseEngineParams(ctx, "pg", *read.PendingRaw); err != nil || ok {
		t.Fatalf("stale apply = %v %v, want not applied", ok, err)
	}
	cur, _ := db.GetDatabaseEngineParams(ctx, "pg")
	if ok, _ := db.ApplyDatabaseEngineParams(ctx, "pg", *cur.PendingRaw); !ok {
		t.Fatal("fresh apply did not apply")
	}
	if ok, _ := db.ApplyDatabaseEngineParams(ctx, "pg", *cur.PendingRaw); ok {
		t.Fatal("second concurrent apply applied again")
	}
	cookie := loginTestSession(t, rt, db)
	if rec := viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/pg/params/apply", `{"confirm":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("apply with nothing staged = %d, want 409", rec.Code)
	}
}

func TestDatabaseParams_MemoryCeilingAndStart(t *testing.T) {
	rt, db, _ := newAccessTestRouter(t)
	ctx := context.Background()
	for _, d := range []store.DesiredDatabase{
		{Name: "small", Engine: store.EnginePostgres, Version: "16", Resources: &store.ServiceResources{MemoryBytes: 1 << 30}},
		{Name: "open", Engine: store.EngineRedis, Version: "7"},
	} {
		if err := db.SaveDesiredDatabase(ctx, d); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	cookie := loginTestSession(t, rt, db)
	cases := []struct {
		path, body string
		want       int
	}{
		{"/api/v1/databases/small/params", `{"set":{"shared_buffers":"1GB"}}`, http.StatusBadRequest},
		{"/api/v1/databases/small/params", `{"set":{"shared_buffers":"512MB"}}`, http.StatusOK},
		{"/api/v1/databases/open/params", `{"set":{"maxmemory":"8gb"}}`, http.StatusBadRequest},
		{"/api/v1/databases/open/params", `{"set":{"maxmemory":"2gb"}}`, http.StatusOK},
		{"/api/v1/databases/open/params", `{"set":{"appendonly":"yes"}}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		if rec := viewerDo(t, rt, cookie, http.MethodPut, c.path, c.body); rec.Code != c.want {
			t.Errorf("%s %s = %d, want %d", c.path, c.body, rec.Code, c.want)
		}
	}
	// Start on a running database does not promote; start after a stop does.
	viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/open/start", `{}`)
	if d, _ := db.GetDesiredDatabase(ctx, "open"); d.EngineParams != "" {
		t.Fatalf("start of a running database applied params: %q", d.EngineParams)
	}
	if err := db.UpdateDatabaseSuspended(ctx, "open", true); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	viewerDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/open/start", `{}`)
	if d, _ := db.GetDesiredDatabase(ctx, "open"); d.EngineParams.Map()["maxmemory"] != "2gb" {
		t.Fatalf("start after stop did not apply: %q", d.EngineParams)
	}
}
