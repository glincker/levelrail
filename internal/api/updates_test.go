package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

// newTestRouterWithFetchLatestRelease builds a Router with
// fetchLatestRelease overridden directly on the unexported field, the
// same pattern newTestRouterWithLookupHost already establishes for
// lookupHost.
func newTestRouterWithFetchLatestRelease(t *testing.T, fn fetchLatestReleaseFunc) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.fetchLatestRelease = fn
	return rt, db
}

func getUpdates(t *testing.T, rt *Router, cookie *http.Cookie) (int, updateStatusResource) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/updates", ""))
	var got updateStatusResource
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
		}
	}
	return rec.Code, got
}

// setVersion pins version.Version for the duration of the test, the
// shared setup every TestHandleGetUpdates_* case below needs.
func setVersion(t *testing.T, v string) {
	t.Helper()
	orig := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = orig })
}

// getUpdatesOK is getUpdates plus the "must succeed" assertion every
// case below shares, so each test only spells out its own distinct
// response checks.
func getUpdatesOK(t *testing.T, rt *Router, cookie *http.Cookie) updateStatusResource {
	t.Helper()
	status, got := getUpdates(t, rt, cookie)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	return got
}

func TestHandleGetUpdates_DevBuildNeverReportsAvailable(t *testing.T) {
	setVersion(t, "dev")

	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		return &githubRelease{TagName: "v9.9.9", HTMLURL: "https://example.com/releases/v9.9.9", PublishedAt: "2026-01-01T00:00:00Z"}, nil
	})
	cookie := loginTestSession(t, rt, db)

	got := getUpdatesOK(t, rt, cookie)
	if got.CurrentVersion != "dev" {
		t.Errorf("current_version = %q, want %q", got.CurrentVersion, "dev")
	}
	if got.UpdateAvailable {
		t.Error("update_available = true, want false: a dev build has nothing meaningful to compare")
	}
}

func TestHandleGetUpdates_NewerVersionAvailable(t *testing.T) {
	setVersion(t, "v1.0.0")

	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		return &githubRelease{TagName: "v1.1.0", HTMLURL: "https://example.com/releases/v1.1.0", PublishedAt: "2026-02-01T00:00:00Z"}, nil
	})
	cookie := loginTestSession(t, rt, db)

	got := getUpdatesOK(t, rt, cookie)
	if !got.UpdateAvailable {
		t.Error("update_available = false, want true")
	}
	if got.LatestVersion == nil || *got.LatestVersion != "v1.1.0" {
		t.Errorf("latest_version = %v, want v1.1.0", got.LatestVersion)
	}
	if got.ReleaseURL == nil || *got.ReleaseURL != "https://example.com/releases/v1.1.0" {
		t.Errorf("release_url = %v", got.ReleaseURL)
	}
	if got.PublishedAt == nil || *got.PublishedAt != "2026-02-01T00:00:00Z" {
		t.Errorf("published_at = %v", got.PublishedAt)
	}
}

func TestHandleGetUpdates_AlreadyOnLatest(t *testing.T) {
	setVersion(t, "v1.1.0")

	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		return &githubRelease{TagName: "v1.1.0", HTMLURL: "https://example.com/releases/v1.1.0", PublishedAt: "2026-02-01T00:00:00Z"}, nil
	})
	cookie := loginTestSession(t, rt, db)

	got := getUpdatesOK(t, rt, cookie)
	if got.UpdateAvailable {
		t.Error("update_available = true, want false: already on the latest tag")
	}
}

func TestHandleGetUpdates_NoReleasesPublished(t *testing.T) {
	setVersion(t, "v1.0.0")

	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		return nil, nil // GitHub 404: no releases published yet
	})
	cookie := loginTestSession(t, rt, db)

	got := getUpdatesOK(t, rt, cookie)
	if got.LatestVersion != nil {
		t.Errorf("latest_version = %v, want nil", got.LatestVersion)
	}
	if got.UpdateAvailable {
		t.Error("update_available = true, want false")
	}
	if got.ReleaseURL != nil || got.PublishedAt != nil {
		t.Errorf("release_url/published_at = %v/%v, want nil/nil", got.ReleaseURL, got.PublishedAt)
	}
}

func TestHandleGetUpdates_FetchFailsNoCache(t *testing.T) {
	setVersion(t, "v1.0.0")

	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		return nil, errors.New("network unreachable")
	})
	cookie := loginTestSession(t, rt, db)

	got := getUpdatesOK(t, rt, cookie)
	if got.CurrentVersion != "v1.0.0" {
		t.Errorf("current_version = %q, want %q", got.CurrentVersion, "v1.0.0")
	}
	if got.LatestVersion != nil || got.UpdateAvailable {
		t.Errorf("got %+v, want zero-value latest/update_available on total fetch failure", got)
	}
}

func TestHandleGetUpdates_FetchFailsServesCache(t *testing.T) {
	setVersion(t, "v1.0.0")

	calls := 0
	fail := false
	rt, db := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		calls++
		if fail {
			return nil, errors.New("network unreachable")
		}
		return &githubRelease{TagName: "v1.2.0", HTMLURL: "https://example.com/releases/v1.2.0", PublishedAt: "2026-03-01T00:00:00Z"}, nil
	})
	cookie := loginTestSession(t, rt, db)

	// First request populates the cache with a real result.
	status, got := getUpdates(t, rt, cookie)
	if status != http.StatusOK || got.LatestVersion == nil || *got.LatestVersion != "v1.2.0" {
		t.Fatalf("priming request: status = %d, got = %+v", status, got)
	}

	// Force the cache to be treated as stale so the handler attempts a
	// fresh fetch, which now fails; the previously cached release must
	// still be served.
	rt.updatesCache.fetchedAt = rt.updatesCache.fetchedAt.Add(-2 * updatesCacheTTL)
	fail = true

	status, got = getUpdates(t, rt, cookie)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if got.LatestVersion == nil || *got.LatestVersion != "v1.2.0" {
		t.Errorf("latest_version = %v, want cached v1.2.0 despite fetch failure", got.LatestVersion)
	}
	if calls != 2 {
		t.Errorf("fetchLatestRelease called %d times, want 2 (prime + failed refresh)", calls)
	}
}

func TestHandleGetUpdates_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouterWithFetchLatestRelease(t, func(context.Context) (*githubRelease, error) {
		return nil, nil
	})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/updates", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
