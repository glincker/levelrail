package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/changelog"
)

func getChangelog(t *testing.T, rt *Router, cookie *http.Cookie, target string) (int, changelogResource) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, target, ""))
	var got changelogResource
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
		}
	}
	return rec.Code, got
}

func TestHandleGetChangelog_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{method: http.MethodGet, target: "/api/v1/changelog"},
	})
}

func TestHandleGetChangelog_NoneConfiguredAnswersEmpty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	status, got := getChangelog(t, rt, cookie, "/api/v1/changelog")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if len(got.Entries) != 0 {
		t.Errorf("Entries = %v, want empty", got.Entries)
	}
}

func TestHandleGetChangelog_ReturnsEntriesNewestFirst(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.changelogEntries = []changelog.Entry{
		{Version: "0.3.0", Date: "2026-10-01", Bullets: []string{"c"}},
		{Version: "0.2.0", Date: "2026-09-01", Bullets: []string{"b"}},
		{Version: "0.1.0", Date: "2026-08-01", Bullets: []string{"a"}},
	}
	cookie := loginTestSession(t, rt, db)

	status, got := getChangelog(t, rt, cookie, "/api/v1/changelog?limit=2")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2", len(got.Entries))
	}
	if got.Entries[0].Version != "0.3.0" || got.Entries[1].Version != "0.2.0" {
		t.Errorf("Entries = %+v, want newest-first 0.3.0, 0.2.0", got.Entries)
	}
}

func TestHandleGetChangelog_LimitIsCapped(t *testing.T) {
	rt, db := newTestRouter(t)
	entries := make([]changelog.Entry, maxChangelogLimit+10)
	for i := range entries {
		entries[i] = changelog.Entry{Version: string(rune('a' + i%26))}
	}
	rt.changelogEntries = entries
	cookie := loginTestSession(t, rt, db)

	_, got := getChangelog(t, rt, cookie, "/api/v1/changelog?limit=1000")
	if len(got.Entries) != maxChangelogLimit {
		t.Errorf("len(Entries) = %d, want %d", len(got.Entries), maxChangelogLimit)
	}
}
