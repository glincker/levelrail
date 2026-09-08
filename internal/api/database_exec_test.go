package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedExecDatabase(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{
		Name: "main", Engine: store.EngineRedis, Version: "7",
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
}

func TestHandleExecDatabase_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithExecRuntime
	cookie := loginTestSession(t, rt, db)
	seedExecDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/exec", `{"command":"echo"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestExecDatabaseRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases/main/exec", strings.NewReader(`{"command":"echo"}`))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleExecDatabase_PlainWriteToken_Forbidden proves POST
// /databases/{name}/exec sits behind AbilityRoot, not AbilityWrite: a
// database's root credentials are plain container env, the same
// reasoning handleExecApp's own AbilityRoot gate documents.
func TestHandleExecDatabase_PlainWriteToken_Forbidden(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	ctx := context.Background()
	seedExecDatabase(t, db)

	const plaintext = "write-scoped-token-db-exec" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write_db_exec", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases/main/exec", strings.NewReader(`{"command":"echo"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not be able to run commands inside a container", rec.Code, http.StatusForbidden)
	}
	if fake.execCalls != 0 {
		t.Errorf("Exec called %d times, want 0: the ability check must reject before any container work happens", fake.execCalls)
	}
}

func TestHandleExecDatabase_UnknownDatabase_NotFound(t *testing.T) {
	fake := &fakeExecAppRuntime{}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/nonexistent/exec", `{"command":"echo"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleExecDatabase_MalformedBody_BadRequest(t *testing.T) {
	fake := &fakeExecAppRuntime{}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/exec", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleExecDatabase_EmptyCommand_BadRequest(t *testing.T) {
	fake := &fakeExecAppRuntime{}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/exec", `{"command":"   "}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleExecDatabase_NoRunningContainer_Conflict(t *testing.T) {
	tests := []struct {
		name  string
		state *docker.ContainerState
	}{
		{name: "never deployed", state: nil},
		{name: "exists but stopped", state: &docker.ContainerState{ID: "c1", Running: false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeExecAppRuntime{inspectState: tc.state}
			rt, db := newTestRouterWithExecRuntime(t, fake)
			cookie := loginTestSession(t, rt, db)
			seedExecDatabase(t, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/exec", `{"command":"echo"}`))
			if rec.Code != http.StatusConflict {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
			}
			if fake.execCalls != 0 {
				t.Errorf("Exec called %d times, want 0: no running container to exec into", fake.execCalls)
			}
		})
	}
}

func TestHandleExecDatabase_Success_Stdout(t *testing.T) {
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   io.NopCloser(strings.NewReader("PONG\n")),
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/exec", `{"command":"redis-cli","args":["ping"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got execResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Stdout != "PONG\n" {
		t.Errorf("Stdout = %q, want %q", got.Stdout, "PONG\n")
	}
	if got.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", got.ExitCode)
	}
	if fake.gotContainerID != "c1" {
		t.Errorf("gotContainerID = %q, want c1", fake.gotContainerID)
	}
	if len(fake.gotCmd) != 2 || fake.gotCmd[0] != "redis-cli" || fake.gotCmd[1] != "ping" {
		t.Errorf("gotCmd = %v, want [redis-cli ping]", fake.gotCmd)
	}
}

func TestHandleExecDatabase_Success_NonzeroExit(t *testing.T) {
	execErr := &docker.ExecExitError{
		Cmd:       []string{"false"},
		Container: "c1",
		ExitCode:  17,
		Stderr:    "boom: no such file",
	}
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   &errReadCloser{r: strings.NewReader("partial output\n"), err: execErr},
	}

	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/exec", `{"command":"false"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got execResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ExitCode != 17 {
		t.Errorf("ExitCode = %d, want 17", got.ExitCode)
	}
	if got.Stderr != "boom: no such file" {
		t.Errorf("Stderr = %q, want %q", got.Stderr, "boom: no such file")
	}
}

// TestHandleExecDatabase_GenuineExecTransportFailure proves an error
// reading Exec's stream that is *not* a docker.ExecExitError is
// reported as a 500, not folded into a 200, same as handleExecApp.
func TestHandleExecDatabase_GenuineExecTransportFailure(t *testing.T) {
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   &errReadCloser{r: strings.NewReader(""), err: errors.New("connection reset by peer")},
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/exec", `{"command":"echo"}`))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

// TestHandleExecDatabase_Timeout mirrors TestHandleExecApp_Timeout: a
// 1-second override keeps this test fast rather than waiting out the
// real 30-second default.
func TestHandleExecDatabase_Timeout(t *testing.T) {
	blocking := newBlockingReadCloser()
	t.Cleanup(func() { _ = blocking.Close() })
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{ID: "c1", Running: true},
		execReader:   blocking,
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecDatabase(t, db)

	start := time.Now()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/exec", `{"command":"sleep","args":["999"],"timeout_seconds":1}`))
	elapsed := time.Since(start)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusGatewayTimeout, rec.Body.String())
	}
	if elapsed > 5*time.Second {
		t.Errorf("handler took %s to respond, want well under its own requested 1s timeout (some slack for scheduling)", elapsed)
	}
}

// TestHandleExecDatabase_NodeResolverError proves a NodeRuntimeResolver
// failure is reported as a clear 502, same as handleExecApp.
func TestHandleExecDatabase_NodeResolverError(t *testing.T) {
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) {
		return nil, errors.New("node not registered")
	}
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver))
	cookie := loginTestSession(t, rt, db)
	seedExecDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/exec", `{"command":"echo"}`))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}
