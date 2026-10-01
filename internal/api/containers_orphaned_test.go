package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeOrphanedContainerManager struct {
	stopped   []string
	removed   []string
	stopErr   error
	removeErr error
}

func (f *fakeOrphanedContainerManager) Stop(_ context.Context, id string, _ time.Duration) error {
	if f.stopErr != nil {
		return f.stopErr
	}
	f.stopped = append(f.stopped, id)
	return nil
}

func (f *fakeOrphanedContainerManager) Remove(_ context.Context, id string, _ bool) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, id)
	return nil
}

func newTestRouterWithOrphanedContainers(t *testing.T, lister ContainerLister, mgr OrphanedContainerManager) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(discardLogger(), testBrand(), db, WithContainerLister(lister), WithOrphanedContainerManager(mgr)), db
}

// The name seedWebAppForTest's service ("web", "img:v1") converges to.
func managedContainerName() string {
	return application.ContainerName("web", "img:v1", "")
}

func TestHandleStopOrphanedContainer_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no ContainerLister, no OrphanedContainerManager
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/containers/ghost/stop", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleStopOrphanedContainer_NotFound(t *testing.T) {
	lister := &fakeContainerLister{}
	rt, db := newTestRouterWithOrphanedContainers(t, lister, &fakeOrphanedContainerManager{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/containers/ghost/stop", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleOrphanedContainerAction_ManagedContainer_Conflict(t *testing.T) {
	name := managedContainerName()
	for _, action := range []string{"stop", "remove"} {
		t.Run(action, func(t *testing.T) {
			lister := &fakeContainerLister{containers: []docker.ContainerState{
				{ID: "c1", Name: name, Labels: map[string]string{spec.InstanceLabelKey: "inst_a"}},
			}}
			mgr := &fakeOrphanedContainerManager{}
			rt, db := newTestRouterWithOrphanedContainers(t, lister, mgr)
			seedWebAppForTest(t, db)
			cookie := loginTestSession(t, rt, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/containers/"+name+"/"+action, ""))
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
			}
			if len(mgr.stopped) != 0 || len(mgr.removed) != 0 {
				t.Errorf("stopped = %v, removed = %v, want none: a managed container must never be mutated", mgr.stopped, mgr.removed)
			}
		})
	}
}

func TestHandleOrphanedContainerAction_Success(t *testing.T) {
	cases := []struct {
		action string
		get    func(*fakeOrphanedContainerManager) []string
	}{
		{"stop", func(m *fakeOrphanedContainerManager) []string { return m.stopped }},
		{"remove", func(m *fakeOrphanedContainerManager) []string { return m.removed }},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			lister := &fakeContainerLister{containers: []docker.ContainerState{
				{ID: "orphan-id", Name: "leftover-nginx", Running: true},
			}}
			mgr := &fakeOrphanedContainerManager{}
			rt, db := newTestRouterWithOrphanedContainers(t, lister, mgr)
			cookie := loginTestSession(t, rt, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/containers/leftover-nginx/"+tc.action, ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if got := tc.get(mgr); len(got) != 1 || got[0] != "orphan-id" {
				t.Errorf("%s = %v, want [orphan-id]", tc.action, got)
			}
		})
	}
}

// Labeled but its owning app was deleted: still orphaned, so still stoppable.
func TestHandleStopOrphanedContainer_LabeledButNoDesiredRecord_Stops(t *testing.T) {
	lister := &fakeContainerLister{containers: []docker.ContainerState{
		{ID: "leftover-id", Name: "web-deadbeef01", Labels: map[string]string{spec.InstanceLabelKey: "inst_a"}},
	}}
	mgr := &fakeOrphanedContainerManager{}
	rt, db := newTestRouterWithOrphanedContainers(t, lister, mgr)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/containers/web-deadbeef01/stop", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(mgr.stopped) != 1 || mgr.stopped[0] != "leftover-id" {
		t.Errorf("stopped = %v, want [leftover-id]", mgr.stopped)
	}
}

func TestOrphanedContainerRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouterWithOrphanedContainers(t, &fakeContainerLister{}, &fakeOrphanedContainerManager{})

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodPost, "/api/v1/system/containers/x/stop"},
		{http.MethodPost, "/api/v1/system/containers/x/remove"},
		{http.MethodPost, "/api/v1/system/containers/x/claim"},
	})
}

// Stop/remove sit behind AbilityRoot, not AbilityWrite.
func TestOrphanedContainerStopRemove_PlainWriteToken_Forbidden(t *testing.T) {
	lister := &fakeContainerLister{containers: []docker.ContainerState{{ID: "orphan-id", Name: "leftover-nginx"}}}
	rt, db := newTestRouterWithOrphanedContainers(t, lister, &fakeOrphanedContainerManager{})
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	for _, target := range []string{
		"/api/v1/system/containers/leftover-nginx/stop",
		"/api/v1/system/containers/leftover-nginx/remove",
	} {
		req := httptest.NewRequest(http.MethodPost, target, nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want %d", target, rec.Code, http.StatusForbidden)
		}
	}
}

func TestHandleClaimOrphanedContainer_DerivesNameAndCreatesApp(t *testing.T) {
	lister := &fakeContainerLister{containers: []docker.ContainerState{
		{ID: "orphan-id", Name: "Leftover_Nginx", Image: "nginx:1.27", Ports: []docker.PortBinding{{ContainerPort: 8080, HostPort: 33000, Protocol: "tcp"}}},
	}}
	rt, db := newTestRouterWithOrphanedContainers(t, lister, &fakeOrphanedContainerManager{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/containers/Leftover_Nginx/claim", ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var created appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Name != "leftover-nginx" {
		t.Errorf("created.Name = %q, want %q", created.Name, "leftover-nginx")
	}
	if created.Image != "nginx:1.27" {
		t.Errorf("created.Image = %q, want %q", created.Image, "nginx:1.27")
	}
	if created.Port != 8080 {
		t.Errorf("created.Port = %d, want 8080 (the container's own published port)", created.Port)
	}

	svc, err := db.GetDesiredService(context.Background(), "leftover-nginx")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if svc.Image != "nginx:1.27" {
		t.Errorf("stored service image = %q, want %q", svc.Image, "nginx:1.27")
	}
}

func TestHandleClaimOrphanedContainer_ExplicitNameOverride(t *testing.T) {
	lister := &fakeContainerLister{containers: []docker.ContainerState{
		{ID: "orphan-id", Name: "leftover-nginx", Image: "nginx:1.27"},
	}}
	rt, db := newTestRouterWithOrphanedContainers(t, lister, &fakeOrphanedContainerManager{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/containers/leftover-nginx/claim", `{"name":"marketing-site"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "marketing-site"); err != nil {
		t.Fatalf("GetDesiredService(marketing-site): %v", err)
	}
}

func TestHandleClaimOrphanedContainer_NameConflict(t *testing.T) {
	lister := &fakeContainerLister{containers: []docker.ContainerState{
		{ID: "orphan-id", Name: "leftover-nginx", Image: "nginx:1.27"},
	}}
	rt, db := newTestRouterWithOrphanedContainers(t, lister, &fakeOrphanedContainerManager{})
	seedWebAppForTest(t, db) // already an app named "web"
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/containers/leftover-nginx/claim", `{"name":"web"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleClaimOrphanedContainer_ManagedContainer_Conflict(t *testing.T) {
	name := managedContainerName()
	lister := &fakeContainerLister{containers: []docker.ContainerState{
		{ID: "c1", Name: name, Image: "img:v1", Labels: map[string]string{spec.InstanceLabelKey: "inst_a"}},
	}}
	rt, db := newTestRouterWithOrphanedContainers(t, lister, &fakeOrphanedContainerManager{})
	seedWebAppForTest(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/containers/"+name+"/claim", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleListContainers_ManagedField(t *testing.T) {
	managedName := managedContainerName()
	lister := &fakeContainerLister{containers: []docker.ContainerState{
		{Name: managedName, Labels: map[string]string{spec.InstanceLabelKey: "inst_a"}},
		{Name: "unrelated-nginx"},
		{Name: "web-stale01", Labels: map[string]string{spec.InstanceLabelKey: "inst_a"}},
	}}
	rt, db := newTestRouterWithContainerLister(t, lister)
	seedWebAppForTest(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := listContainers(t, rt, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []containerResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byName := make(map[string]containerResource, len(got))
	for _, c := range got {
		byName[c.Name] = c
	}
	if !byName[managedName].Managed {
		t.Errorf("%s: Managed = false, want true", managedName)
	}
	if byName["unrelated-nginx"].Managed {
		t.Errorf("unrelated-nginx: Managed = true, want false")
	}
	if byName["web-stale01"].Managed {
		t.Errorf("web-stale01: Managed = true, want false (labeled but no matching desired-state record)")
	}
}

func TestDeriveAppNameFromContainer(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"leftover-nginx", "leftover-nginx"},
		{"Leftover_Nginx", "leftover-nginx"},
		{"  weird..name__1  ", "weird-name-1"},
		{"---", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := deriveAppNameFromContainer(c.in); got != c.want {
			t.Errorf("deriveAppNameFromContainer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestContainerClaimPort(t *testing.T) {
	if got := containerClaimPort(docker.ContainerState{}); got != defaultClaimPort {
		t.Errorf("no ports: containerClaimPort() = %d, want %d", got, defaultClaimPort)
	}
	c := docker.ContainerState{Ports: []docker.PortBinding{{ContainerPort: 9090, HostPort: 1234}}}
	if got := containerClaimPort(c); got != 9090 {
		t.Errorf("containerClaimPort() = %d, want 9090", got)
	}
}
