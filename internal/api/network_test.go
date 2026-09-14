package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleGetAppNetwork_UnknownApp_NotFound(t *testing.T) {
	fake := &fakeExecAppRuntime{}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nonexistent/network", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestHandleGetAppNetwork_NoRuntimeConfigured proves a control plane
// with no NodeRuntimeResolver still answers with the declared container
// port, degrading rather than failing, the same "absence is a reportable
// empty state" shape handleListImages already follows.
func TestHandleGetAppNetwork_NoRuntimeConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithExecRuntime
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/network", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ContainerPort != 3000 {
		t.Errorf("ContainerPort = %d, want 3000", got.ContainerPort)
	}
	if got.Running || got.HostPort != 0 {
		t.Errorf("got %+v, want Running=false HostPort=0", got)
	}
}

func TestHandleGetAppNetwork_RunningWithHostPort(t *testing.T) {
	fake := &fakeExecAppRuntime{
		inspectState: &docker.ContainerState{
			ID:      "c1",
			Running: true,
			Ports:   []docker.PortBinding{{ContainerPort: 3000, HostPort: 52308, Protocol: "tcp"}},
		},
	}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/network", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ContainerPort != 3000 {
		t.Errorf("ContainerPort = %d, want 3000", got.ContainerPort)
	}
	if !got.Running {
		t.Error("Running = false, want true")
	}
	if got.HostPort != 52308 {
		t.Errorf("HostPort = %d, want 52308", got.HostPort)
	}
}

func TestHandleGetAppNetwork_StoppedOrMissing_NoHostPort(t *testing.T) {
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
			seedExecApp(t, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/network", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var got networkResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Running || got.HostPort != 0 {
				t.Errorf("got %+v, want Running=false HostPort=0", got)
			}
		})
	}
}

// TestHandleGetAppNetwork_NodeResolverError proves an unreachable node
// degrades to a plain 200 with no host port rather than a 502: unlike
// exec (a genuine action that cannot proceed without a live node), this
// is a passive status read that should still report the declared port
// even when the live lookup can't be completed.
func TestHandleGetAppNetwork_NodeResolverError(t *testing.T) {
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) {
		return nil, errors.New("node not registered")
	}
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver))
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/network", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ContainerPort != 3000 {
		t.Errorf("ContainerPort = %d, want 3000", got.ContainerPort)
	}
	if got.Running {
		t.Error("Running = true, want false")
	}
}

// TestHandleGetAppNetwork_InspectBoundedByTimeout proves the live
// InspectByName call is never handed the bare request context (which
// carries no deadline of its own): an unresponsive Docker daemon must
// time this request out rather than hang the Network tab's page load
// forever.
func TestHandleGetAppNetwork_InspectBoundedByTimeout(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/network", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if fake.gotInspectCtx == nil {
		t.Fatal("InspectByName was never called")
	}
	if _, ok := fake.gotInspectCtx.Deadline(); !ok {
		t.Error("InspectByName's context has no deadline, want one bounded by dockerInspectTimeout")
	}
}

// TestHandleGetAppNetwork_NoDomains_PublicHostSet_FallbackURL proves the
// same zero-config sslip.io URL internal/reconcile/ingress's Reconcile
// pass routes an app under is also surfaced here, so the dashboard/CLI
// can show an operator a copyable link without them ever configuring a
// domain.
func TestHandleGetAppNetwork_NoDomains_PublicHostSet_FallbackURL(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) { return fake, nil }
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver), WithPublicHost("203.0.113.5"))
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db) // "web", no Domains

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/network", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if want := "https://web.203-0-113-5.sslip.io"; got.FallbackURL != want {
		t.Errorf("FallbackURL = %q, want %q", got.FallbackURL, want)
	}
}

// TestHandleGetAppNetwork_HasDomain_NoFallbackURL proves an app with a
// real, operator-configured domain never gets a fallback URL alongside
// it: the domain the operator actually set is the one true address,
// there's no reason to also compute and show a second, synthetic one.
func TestHandleGetAppNetwork_HasDomain_NoFallbackURL(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) { return fake, nil }
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver), WithPublicHost("203.0.113.5"))
	cookie := loginTestSession(t, rt, db)
	svc := store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000, Domains: []string{"web.example.com"}}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/network", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.FallbackURL != "" {
		t.Errorf("FallbackURL = %q, want empty when a real domain is configured", got.FallbackURL)
	}
}

// TestHandleGetAppNetwork_NoDomains_NoPublicHost_NoFallbackURL proves
// the default (no APP_PUBLIC_HOST configured) never surfaces a
// fallback URL, matching the reconciler's own "no route" default.
func TestHandleGetAppNetwork_NoDomains_NoPublicHost_NoFallbackURL(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "c1", Running: true}}
	rt, db := newTestRouterWithExecRuntime(t, fake) // no WithPublicHost
	cookie := loginTestSession(t, rt, db)
	seedExecApp(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/network", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.FallbackURL != "" {
		t.Errorf("FallbackURL = %q, want empty with no APP_PUBLIC_HOST configured", got.FallbackURL)
	}
}

func TestNetworkAppRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/network", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
