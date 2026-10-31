package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedGitSourceForSchedule(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveGitSource(context.Background(), store.GitSource{
		ServiceName: "web", RepoURL: "https://example.com/org/web.git", Branch: "main", BuildType: "dockerfile",
	}); err != nil {
		t.Fatalf("seed git source for web: %v", err)
	}
}

func TestAppScheduleRoutes_RequireAuth(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/schedule"},
		{http.MethodPut, "/api/v1/apps/web/schedule"},
		{http.MethodGet, "/api/v1/apps/web/schedule/history"},
	})
}

func TestHandleGetAppSchedule_NotConfigured(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/schedule", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetAppSchedule_AppNotFound(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/missing/schedule", `{"cron":"0 3 * * *","branch":"main"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetAppSchedule_RequiresGitSource(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web") // no git source connected

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/schedule", `{"cron":"0 3 * * *","branch":"main"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "git source") {
		t.Errorf("body = %q, want a message about connecting a git source", rec.Body.String())
	}
}

func TestHandleSetAppSchedule_InvalidCron(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedGitSourceForSchedule(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/schedule", `{"cron":"not a cron","branch":"main"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetAppSchedule_InvalidTimezone(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedGitSourceForSchedule(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/schedule", `{"cron":"0 3 * * *","branch":"main","timezone":"Not/AZone"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetAppSchedule_MissingBranch(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedGitSourceForSchedule(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/schedule", `{"cron":"0 3 * * *"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetAndGetAppSchedule_Success(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedGitSourceForSchedule(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/schedule", `{"cron":"0 3 * * *","branch":"main","timezone":"UTC"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var saved appScheduleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode PUT response: %v", err)
	}
	if !saved.Enabled {
		t.Errorf("saved.Enabled = false, want true (default when enabled is omitted)")
	}
	if saved.NextRunAt == nil {
		t.Error("saved.NextRunAt = nil, want it armed synchronously by the PUT itself, not left for the scheduler's next tick")
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/schedule", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got appScheduleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if got.Cron != "0 3 * * *" || got.Branch != "main" || got.Timezone != "UTC" {
		t.Errorf("GET schedule = %+v, want the saved values", got)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(*saved.NextRunAt) {
		t.Errorf("GET NextRunAt = %v, want it to match what the PUT armed (%v)", got.NextRunAt, saved.NextRunAt)
	}
}

func TestHandleListAppScheduleHistory_EmptyByDefault(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/schedule/history", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var entries []appScheduleHistoryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want empty", entries)
	}
}

// TestTriggerScheduledDeploy_DeploysLatestCommitOnBranch checks the branch head is deployed through the git-source path.
func TestTriggerScheduledDeploy_DeploysLatestCommitOnBranch(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithGitSourceSecrets(newFakeGitSourceSecrets()))
	seedApp(t, db, "web")
	seedGitSourceForSchedule(t, db)

	fb := &fakeBuilder{tag: "web:resolved-sha"}
	rt.builder = fb
	var fetchCalls []fakeGitSourceFetchCall
	cleanupCalled := false
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), &cleanupCalled, nil)
	rt.resolveBranchSHA = func(_ context.Context, _, branch, _ string) (string, error) {
		if branch != "release" {
			t.Errorf("resolveBranchSHA branch = %q, want %q", branch, "release")
		}
		return "resolved-sha", nil
	}

	result, err := rt.TriggerScheduledDeploy(context.Background(), "web", "release")
	if err != nil {
		t.Fatalf("TriggerScheduledDeploy() error = %v", err)
	}
	if result == "" {
		t.Error("TriggerScheduledDeploy() returned an empty result on success")
	}
	if len(fetchCalls) != 1 || fetchCalls[0].sha != "resolved-sha" {
		t.Fatalf("fetchCalls = %+v, want exactly one fetch of resolved-sha", fetchCalls)
	}

	attempts, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 || attempts[0].Source != store.DeployAttemptSourceSchedule {
		t.Fatalf("attempts = %+v, want exactly one attempt sourced %q", attempts, store.DeployAttemptSourceSchedule)
	}
}

func TestTriggerScheduledDeploy_NoGitSourceConnected(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	seedApp(t, db, "web") // no git source

	if _, err := rt.TriggerScheduledDeploy(context.Background(), "web", "main"); err == nil {
		t.Fatal("TriggerScheduledDeploy() error = nil, want an error for a missing git source")
	}
}

func putSchedule(t *testing.T, rt *Router, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/schedule", body))
	return rec
}

func TestHandleSetAppSchedule_RunAtValidation(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedGitSourceForSchedule(t, db)

	soon := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	cases := []struct {
		name, body, wantMsg string
	}{
		{"both", `{"cron":"0 3 * * *","run_at":"` + soon + `","branch":"main"}`, "exactly one of cron or run_at"},
		{"neither", `{"branch":"main"}`, "one of cron or run_at is required"},
		{"past", `{"run_at":"2020-01-01T00:00:00Z","branch":"main"}`, "in the future"},
		{"beyond horizon", `{"run_at":"` + time.Now().Add(800*24*time.Hour).UTC().Format(time.RFC3339) + `","branch":"main"}`, "within"},
		{"bad format", `{"run_at":"next tuesday","branch":"main"}`, "RFC 3339"},
	}
	for _, c := range cases {
		rec := putSchedule(t, rt, cookie, c.body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.wantMsg) {
			t.Errorf("%s: status = %d body = %s, want 400 containing %q", c.name, rec.Code, rec.Body.String(), c.wantMsg)
		}
	}
}

func TestHandleSetAppSchedule_RunAtSuccessAndRearm(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	seedGitSourceForSchedule(t, db)

	at := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	rec := putSchedule(t, rt, cookie, `{"run_at":"`+at.Format(time.RFC3339)+`","branch":"main"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var saved appScheduleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if saved.RunAt == nil || !saved.RunAt.Equal(at) || saved.NextRunAt == nil || !saved.NextRunAt.Equal(at) || saved.Cron != "" {
		t.Fatalf("saved = %+v, want run_at and next_run_at both %v and no cron", saved, at)
	}

	if err := db.CompleteAppSchedule(context.Background(), "web", time.Now()); err != nil {
		t.Fatalf("CompleteAppSchedule: %v", err)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/schedule", ""))
	var done appScheduleResource
	_ = json.Unmarshal(rec.Body.Bytes(), &done)
	if done.CompletedAt == nil || done.Enabled {
		t.Fatalf("after completion = %+v, want completed_at set and disabled", done)
	}

	later := at.Add(time.Hour)
	if rec := putSchedule(t, rt, cookie, `{"run_at":"`+later.Format(time.RFC3339)+`","branch":"main"}`); rec.Code != http.StatusOK {
		t.Fatalf("re-arm PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/schedule", ""))
	var rearmed appScheduleResource
	_ = json.Unmarshal(rec.Body.Bytes(), &rearmed)
	if rearmed.CompletedAt != nil || !rearmed.Enabled {
		t.Fatalf("after re-arm = %+v, want completed_at cleared and enabled", rearmed)
	}
}
