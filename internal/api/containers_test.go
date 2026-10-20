package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeContainerLister is a hand-written fake for ContainerLister, the
// same pattern fakeImageLister (images_test.go) already establishes in
// this package.
type fakeContainerLister struct {
	containers []docker.ContainerState
	err        error
}

func (f *fakeContainerLister) ListByPrefix(_ context.Context, _ string) ([]docker.ContainerState, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.containers, nil
}

func newTestRouterWithContainerLister(t *testing.T, l ContainerLister) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(nil, testBrand(), db, WithContainerLister(l)), db
}

// listContainers issues an authenticated GET /api/v1/system/containers,
// the shared request-and-record sequence every status-code test below
// needs before its own assertions diverge.
func listContainers(t *testing.T, rt *Router, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/containers", ""))
	return rec
}

func TestHandleListContainers_Configured(t *testing.T) {
	lister := &fakeContainerLister{
		containers: []docker.ContainerState{
			{Name: "levelrail-app-web", Image: "levelrail/web:1", Running: true, Ports: []docker.PortBinding{{ContainerPort: 3000, HostPort: 33001, Protocol: "tcp"}}},
			{Name: "some-other-container", Image: "postgres:16", Running: false},
		},
	}
	rt, db := newTestRouterWithContainerLister(t, lister)
	cookie := loginTestSession(t, rt, db)

	rec := listContainers(t, rt, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []containerResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Name != "levelrail-app-web" || !got[0].Running {
		t.Errorf("got[0] = %+v", got[0])
	}
	if len(got[0].Ports) != 1 || got[0].Ports[0].HostPort != 33001 {
		t.Errorf("got[0].Ports = %+v", got[0].Ports)
	}
	if got[1].Name != "some-other-container" || got[1].Running {
		t.Errorf("got[1] = %+v, want a non-Levelrail container still listed", got[1])
	}
}

func TestHandleListContainers_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := listContainers(t, rt, cookie)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleListContainers_ListerError(t *testing.T) {
	rt, db := newTestRouterWithContainerLister(t, &fakeContainerLister{err: context.DeadlineExceeded})
	cookie := loginTestSession(t, rt, db)

	rec := listContainers(t, rt, cookie)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleListContainers_Empty(t *testing.T) {
	rt, db := newTestRouterWithContainerLister(t, &fakeContainerLister{})
	cookie := loginTestSession(t, rt, db)

	rec := listContainers(t, rt, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []containerResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}

func TestHandleListContainers_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouterWithContainerLister(t, &fakeContainerLister{})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/system/containers", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
