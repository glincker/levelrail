package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeImageLister is a hand-written fake for ImageLister, the same
// pattern fakeDockerPinger (status_test.go) and fakeSecretSetter
// (secrets_test.go) already establish in this package instead of a
// mocking framework. images is keyed by repo so a test can assert
// handleListImages derived the expected repo from the app's current
// Image field.
type fakeImageLister struct {
	images map[string][]docker.ImageInfo
	err    error
}

func (f *fakeImageLister) ListImages(_ context.Context, repo string) ([]docker.ImageInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.images[repo], nil
}

func newTestRouterWithImageLister(t *testing.T, l ImageLister) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	return NewRouter(nil, testBrand(), db, WithImageLister(l)), db
}

func TestHandleListImages_Configured(t *testing.T) {
	created1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	created2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	lister := &fakeImageLister{
		images: map[string][]docker.ImageInfo{
			"ghcr.io/you/app": {
				{Tag: "ghcr.io/you/app:def456", CreatedAt: created2},
				{Tag: "ghcr.io/you/app:abc123", CreatedAt: created1},
			},
		},
	}
	rt, db := newTestRouterWithImageLister(t, lister)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "ghcr.io/you/app:abc123", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/images", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []imageResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2; got = %+v", len(got), got)
	}
	if got[0].Tag != "ghcr.io/you/app:def456" {
		t.Errorf("got[0].Tag = %q, want %q", got[0].Tag, "ghcr.io/you/app:def456")
	}
	if got[1].Tag != "ghcr.io/you/app:abc123" {
		t.Errorf("got[1].Tag = %q, want %q", got[1].Tag, "ghcr.io/you/app:abc123")
	}
	if !got[1].CreatedAt.Equal(created1) {
		t.Errorf("got[1].CreatedAt = %v, want %v", got[1].CreatedAt, created1)
	}
}

func TestHandleListImages_NotConfigured(t *testing.T) {
	// No WithImageLister: absence degrades to an empty list, not an
	// error, the same "optional signal" shape WithDockerPinger and
	// WithSecretSetter's own absence already establish.
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "ghcr.io/you/app:abc123", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/images", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []imageResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0 with no ImageLister configured", len(got))
	}
}

func TestHandleListImages_AppNotFound(t *testing.T) {
	rt, db := newTestRouterWithImageLister(t, &fakeImageLister{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/images", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListImages_ListerError(t *testing.T) {
	// A ListImages failure degrades to an empty list rather than a 5xx:
	// this route backs a "nice to have" dropdown, it must never block an
	// operator from seeing the deploy trigger form.
	rt, db := newTestRouterWithImageLister(t, &fakeImageLister{err: context.DeadlineExceeded})
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "ghcr.io/you/app:abc123", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/images", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []imageResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0 when ListImages errors", len(got))
	}
}

func TestHandleListImages_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouterWithImageLister(t, &fakeImageLister{})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/images", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestRepoFromImage covers the actual edge case that breaks a naive
// "split on the first/only colon" parser: a registry host that itself
// carries a port (registry:5000/...), where a colon appears before the
// last "/" as well as after it. Docker's own convention (and
// github.com/distribution/reference, which repoFromImage delegates to)
// is to split on the LAST colon that occurs AFTER the last slash.
func TestRepoFromImage(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{"simple repo with tag", "ghcr.io/you/app:abc123", "ghcr.io/you/app"},
		{"registry with port, single path segment", "registry.example.com:5000/app:latest", "registry.example.com:5000/app"},
		{"registry with port, nested path", "registry.example.com:5000/team/app:v1.2.3", "registry.example.com:5000/team/app"},
		{"no registry, single component with tag", "myapp:latest", "myapp"},
		{"no registry, no tag", "myapp", "myapp"},
		{"localhost with port, no tag", "localhost:5000/myapp", "localhost:5000/myapp"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repoFromImage(tt.image); got != tt.want {
				t.Errorf("repoFromImage(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}
