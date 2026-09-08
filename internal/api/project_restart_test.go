package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleRestartProject_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "levelrail/web:1", Port: 3000},
		{Name: "worker", Image: "levelrail/worker:1", Port: 3001},
		{Name: "unrelated", Image: "levelrail/unrelated:1", Port: 3002},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatalf("SaveDesiredService(%s) error = %v", svc.Name, err)
		}
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("UpdateServiceProject(web) error = %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "worker", "proj_1"); err != nil {
		t.Fatalf("UpdateServiceProject(worker) error = %v", err)
	}

	before, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService(web) error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/proj_1/restart", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got projectRestartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.RestartedCount != 2 {
		t.Errorf("RestartedCount = %d, want 2", got.RestartedCount)
	}
	if len(got.Apps) != 2 || got.Apps[0] != "web" || got.Apps[1] != "worker" {
		t.Errorf("Apps = %+v, want [web worker]", got.Apps)
	}
	if len(got.Failed) != 0 {
		t.Errorf("Failed = %+v, want empty", got.Failed)
	}

	after, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService(web) error = %v", err)
	}
	if after.RestartNonce == before.RestartNonce {
		t.Error("RestartNonce unchanged after restart")
	}

	unrelated, err := db.GetDesiredService(ctx, "unrelated")
	if err != nil {
		t.Fatalf("GetDesiredService(unrelated) error = %v", err)
	}
	if unrelated.RestartNonce != "" {
		t.Error("RestartNonce changed for an app outside the project")
	}
}

func TestHandleRestartProject_NoApps(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "empty"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/proj_1/restart", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got projectRestartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.RestartedCount != 0 || len(got.Apps) != 0 || len(got.Failed) != 0 {
		t.Errorf("response = %+v, want an all-empty result", got)
	}
}

func TestHandleRestartProject_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/does-not-exist/restart", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
