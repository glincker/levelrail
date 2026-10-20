package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// containerPort extracts the real loopback port an httptest.Server bound,
// so a fake docker.Runtime's InspectByName can report it as the probed
// container's own published port: the same loopback-HTTP-round-trip
// approach internal/probe's own tests use, rather than mocking the
// transport.
func containerPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return port
}

func TestHandleDiscoverAppHealth_FindsTheRealWorkingPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{
		ID: "c1", Name: "web", Running: true,
		Ports: []docker.PortBinding{{HostPort: containerPort(t, srv)}},
	}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/health/discover", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var resp healthDiscoveryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Found != "/healthz" {
		t.Errorf("Found = %q, want /healthz", resp.Found)
	}
	if len(resp.Attempts) != len(discoverHealthCandidatePaths) {
		t.Errorf("len(Attempts) = %d, want every candidate path tried (%d)", len(resp.Attempts), len(discoverHealthCandidatePaths))
	}
	var sawHealthz bool
	for _, a := range resp.Attempts {
		if a.Path != "/healthz" {
			if a.Success {
				t.Errorf("path %q reported success, want only /healthz to succeed against this fixture", a.Path)
			}
			continue
		}
		sawHealthz = true
		if !a.Success {
			t.Errorf("/healthz attempt = %+v, want Success true", a)
		}
	}
	if !sawHealthz {
		t.Fatal("/healthz was never attempted")
	}
}

func TestHandleDiscoverAppHealth_NoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{
		ID: "c1", Name: "web", Running: true,
		Ports: []docker.PortBinding{{HostPort: containerPort(t, srv)}},
	}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/health/discover", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var resp healthDiscoveryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Found != "" {
		t.Errorf("Found = %q, want empty: nothing matched", resp.Found)
	}
	for _, a := range resp.Attempts {
		if a.Success {
			t.Errorf("path %q reported success against a fixture that always 503s", a.Path)
		}
		if a.Error == "" {
			t.Errorf("path %q has no Error text despite failing", a.Path)
		}
	}
}

func TestHandleDiscoverAppHealth_NoPort(t *testing.T) {
	fake := &fakeExecAppRuntime{}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1"}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/health/discover", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	if fake.inspectByNameCalls != nil {
		t.Error("InspectByName should never be reached when the app has no port")
	}
}

func TestHandleDiscoverAppHealth_NoRunningContainer(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Name: "web", Running: false}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/health/discover", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no running container") {
		t.Errorf("body = %s, want a message naming the missing running container", rec.Body.String())
	}
}

func TestHandleDiscoverAppHealth_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithExecRuntime
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/health/discover", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501, body = %s", rec.Code, rec.Body.String())
	}
}
