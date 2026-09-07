package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedRestartDatabase(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{
		Name: "main", Engine: store.EngineRedis, Version: "7",
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
}

func TestHandleRestartDatabase_RunningContainer_StopsThenStarts(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "db-c1", Running: true}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedRestartDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restart", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fake.stopCalls != 1 || fake.stopID != "db-c1" {
		t.Errorf("stopCalls=%d stopID=%q, want 1/db-c1", fake.stopCalls, fake.stopID)
	}
	if fake.startCalls != 1 || fake.startID != "db-c1" {
		t.Errorf("startCalls=%d startID=%q, want 1/db-c1", fake.startCalls, fake.startID)
	}
}

func TestHandleRestartDatabase_StoppedContainer_StartsWithoutStopping(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "db-c1", Running: false}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedRestartDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restart", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fake.stopCalls != 0 {
		t.Errorf("stopCalls = %d, want 0 for an already-stopped container", fake.stopCalls)
	}
	if fake.startCalls != 1 || fake.startID != "db-c1" {
		t.Errorf("startCalls=%d startID=%q, want 1/db-c1", fake.startCalls, fake.startID)
	}
}

func TestHandleRestartDatabase_NoContainerYet_Conflict(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: nil}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedRestartDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restart", ""))
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleRestartDatabase_UnknownDatabase_NotFound(t *testing.T) {
	fake := &fakeExecAppRuntime{}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/nonexistent/restart", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleRestartDatabase_NoRuntimeConfigured_NotImplemented(t *testing.T) {
	rt, db := newTestRouter(t) // no WithExecRuntime
	cookie := loginTestSession(t, rt, db)
	seedRestartDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restart", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleRestartDatabase_NodeUnreachable_BadGateway(t *testing.T) {
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) {
		return nil, errors.New("node not registered")
	}
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver))
	cookie := loginTestSession(t, rt, db)
	seedRestartDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restart", ""))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestHandleRestartDatabase_StopFails_BadGateway(t *testing.T) {
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "db-c1", Running: true},
		stopErr:      errors.New("boom"),
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedRestartDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/restart", ""))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if fake.startCalls != 0 {
		t.Errorf("startCalls = %d, want 0 when stop fails", fake.startCalls)
	}
}

func TestRestartDatabaseRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases/main/restart", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
