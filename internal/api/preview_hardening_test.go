package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func previewRow(id, app string, pr int, created, status string) store.PreviewEnvironment {
	return store.PreviewEnvironment{ID: id, AppName: app, PRNumber: pr, CreatedAt: created, Status: status}
}

func TestSelectEvictions(t *testing.T) {
	active := store.PreviewStatusActive
	all := []store.PreviewEnvironment{
		previewRow("c", "web", 3, "2026-01-03", active),
		previewRow("a", "web", 1, "2026-01-01", active),
		previewRow("x", "api", 9, "2026-01-02", active),
		previewRow("b", "web", 2, "2026-01-04", active),
		previewRow("held", "web", 7, "2026-01-00", store.PreviewStatusAwaitingApproval),
	}
	tests := []struct {
		name     string
		app      string
		selfID   string
		limits   PreviewLimits
		wantIDs  []string
		wantNone bool
	}{
		{name: "unlimited", app: "web", limits: PreviewLimits{}, wantNone: true},
		{name: "under the per-app cap", app: "web", limits: PreviewLimits{MaxPerApp: 4}, wantNone: true},
		{name: "per-app cap evicts the oldest of that app", app: "web", limits: PreviewLimits{MaxPerApp: 3}, wantIDs: []string{"a"}},
		{name: "per-app cap of 1 evicts down to make room", app: "web", limits: PreviewLimits{MaxPerApp: 1}, wantIDs: []string{"a", "c", "b"}},
		{name: "global cap evicts the oldest across apps", app: "api", limits: PreviewLimits{MaxTotal: 4}, wantIDs: []string{"a"}},
		{name: "per-app then global", app: "web", limits: PreviewLimits{MaxPerApp: 3, MaxTotal: 3}, wantIDs: []string{"a", "x"}},
		{name: "the preview being redeployed is not counted", app: "web", selfID: "a", limits: PreviewLimits{MaxPerApp: 3}, wantNone: true},
		{name: "awaiting approval rows hold no slot", app: "web", limits: PreviewLimits{MaxPerApp: 4}, wantNone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectEvictions(all, tt.app, tt.selfID, tt.limits)
			if tt.wantNone {
				if len(got) != 0 {
					t.Fatalf("selectEvictions() = %+v, want none", got)
				}
				return
			}
			var ids []string
			for _, e := range got {
				ids = append(ids, e.victim.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tt.wantIDs, ",") {
				t.Errorf("evicted = %v, want %v", ids, tt.wantIDs)
			}
		})
	}
}

func previewDB(t *testing.T, rt *Router) *store.DB {
	t.Helper()
	db, ok := rt.previewEnvironments.(*store.DB)
	if !ok {
		t.Fatalf("previewEnvironments is %T, want *store.DB", rt.previewEnvironments)
	}
	return db
}

func previewPRs(t *testing.T, rt *Router) map[int]string {
	t.Helper()
	rows, err := previewDB(t, rt).ListPreviewEnvironmentsByApp(context.Background(), "web")
	if err != nil {
		t.Fatalf("list previews: %v", err)
	}
	out := map[int]string{}
	for _, p := range rows {
		out[p.PRNumber] = p.Status
	}
	return out
}

func openPR(t *testing.T, rt *Router, secret string, n int) *httptest.ResponseRecorder {
	t.Helper()
	return sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", n, fmt.Sprintf("sha%d", n), "main"))
}

func TestPreviewLimit_EvictsOldestByDefault(t *testing.T) {
	rt, _, secret, builder := setUpPreviewApp(t)
	rt.previewLimits = PreviewLimits{MaxPerApp: 2}

	for n := 1; n <= 3; n++ {
		if rec := openPR(t, rt, secret, n); rec.Code != http.StatusOK {
			t.Fatalf("open #%d: status = %d, body = %s", n, rec.Code, rec.Body.String())
		}
	}

	got := previewPRs(t, rt)
	if _, ok := got[1]; ok || got[2] != store.PreviewStatusActive || got[3] != store.PreviewStatusActive {
		t.Fatalf("previews = %v, want #1 evicted and #2, #3 active", got)
	}
	if len(builder.calls) != 3 {
		t.Errorf("builder deployed %d times, want 3", len(builder.calls))
	}
	if _, err := previewDB(t, rt).GetDesiredService(context.Background(), "web-pr-1"); err == nil {
		t.Error("evicted preview's service still exists")
	}
}

func TestPreviewLimit_GlobalCapEvictsOldest(t *testing.T) {
	rt, _, secret, _ := setUpPreviewApp(t)
	rt.previewLimits = PreviewLimits{MaxTotal: 2}

	for n := 1; n <= 3; n++ {
		openPR(t, rt, secret, n)
	}
	got := previewPRs(t, rt)
	if _, ok := got[1]; ok || len(got) != 2 {
		t.Fatalf("previews = %v, want only #2 and #3", got)
	}
}

func TestPreviewLimit_RejectPolicyKeepsExistingAndRecordsReason(t *testing.T) {
	rt, _, secret, builder := setUpPreviewApp(t)
	rt.previewLimits = PreviewLimits{MaxPerApp: 2}
	db := previewDB(t, rt)
	if err := db.SavePreviewAppSettings(context.Background(), store.PreviewAppSettings{AppName: "web", OnLimit: store.PreviewOnLimitReject}); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	openPR(t, rt, secret, 1)
	openPR(t, rt, secret, 2)
	rec := openPR(t, rt, secret, 3)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Preview limit reached") {
		t.Fatalf("third open: status = %d, body = %q, want ignored with the limit reason", rec.Code, rec.Body.String())
	}

	got := previewPRs(t, rt)
	if got[1] != store.PreviewStatusActive || got[2] != store.PreviewStatusActive || got[3] != store.PreviewStatusLimitReached {
		t.Fatalf("previews = %v", got)
	}
	if len(builder.calls) != 2 {
		t.Errorf("builder deployed %d times, want 2 (the rejected PR must not build)", len(builder.calls))
	}

	sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 1, "sha1", "main"))
	if rec := openPR(t, rt, secret, 3); rec.Code != http.StatusOK || previewPRs(t, rt)[3] != store.PreviewStatusActive {
		t.Fatalf("retry after a slot freed: status = %d, previews = %v, want #3 active", rec.Code, previewPRs(t, rt))
	}
}

func TestPreviewLimit_RedeployOfLivePreviewNeverEvicts(t *testing.T) {
	rt, _, secret, _ := setUpPreviewApp(t)
	rt.previewLimits = PreviewLimits{MaxPerApp: 1}

	openPR(t, rt, secret, 1)
	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("synchronize", 1, "sha9", "main"))
	if rec.Code != http.StatusOK || previewPRs(t, rt)[1] != store.PreviewStatusActive {
		t.Fatalf("synchronize: status = %d, previews = %v, want #1 still active", rec.Code, previewPRs(t, rt))
	}
}

func TestForkPreview_SkippedWithoutSecretsUntilApproved(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)
	cookie := loginTestSession(t, rt, db)

	fork := githubPullRequestBodyFrom("opened", 42, "sha1", "main", "mallory/web")
	rec := sendPullRequestWebhook(rt, secret, fork)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "needs operator approval") {
		t.Fatalf("fork open: status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if len(builder.calls) != 0 {
		t.Fatalf("builder ran %d times for an unapproved fork, want 0", len(builder.calls))
	}
	row, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil || row.Status != store.PreviewStatusAwaitingApproval || !row.IsFork || row.HeadRepo != "mallory/web" {
		t.Fatalf("row = %+v, err = %v, want awaiting_approval fork row", row, err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews/42/approve", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("approve without confirm: status = %d, want 400", rec.Code)
	}
	if len(builder.calls) != 0 {
		t.Fatal("approve without confirm deployed")
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews/42/approve", `{"confirm":true}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("approve: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rt.previewDeploys.Wait()

	row, err = db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil || row.Status != store.PreviewStatusActive {
		t.Fatalf("after approve row = %+v, err = %v, want active", row, err)
	}
	if len(builder.calls) != 1 {
		t.Fatalf("builder ran %d times, want 1", len(builder.calls))
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews/42/approve", `{"confirm":true}`))
	if rec.Code != http.StatusConflict {
		t.Errorf("second approve: status = %d, want 409 (nothing awaiting)", rec.Code)
	}
}

func TestForkPreview_NewForkPushAfterApprovalNeedsApprovalAgain(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)
	cookie := loginTestSession(t, rt, db)

	sendPullRequestWebhook(rt, secret, githubPullRequestBodyFrom("opened", 42, "sha1", "main", "mallory/web"))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews/42/approve", `{"confirm":true}`))
	rt.previewDeploys.Wait()
	if rec.Code != http.StatusAccepted {
		t.Fatalf("approve: status = %d", rec.Code)
	}

	sendPullRequestWebhook(rt, secret, githubPullRequestBodyFrom("synchronize", 42, "sha2", "main", "mallory/web"))
	row, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil || row.Status != store.PreviewStatusAwaitingApproval || row.HeadSHA != "sha2" {
		t.Fatalf("row = %+v, err = %v, want awaiting_approval at sha2", row, err)
	}
	if len(builder.calls) != 1 {
		t.Errorf("builder ran %d times, want 1 (only the approved commit)", len(builder.calls))
	}
	if _, err := db.GetDesiredService(context.Background(), "web-pr-42"); err == nil {
		t.Error("previously approved deployment still serving after an unapproved fork push")
	}
}

func TestForkPreview_AllowedWhenOperatorOptsIn(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)
	if err := db.SavePreviewAppSettings(context.Background(), store.PreviewAppSettings{AppName: "web", OnLimit: store.PreviewOnLimitEvictOldest, AllowForkPreviews: true}); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBodyFrom("opened", 42, "sha1", "main", "mallory/web"))
	if rec.Code != http.StatusOK || len(builder.calls) != 1 {
		t.Fatalf("status = %d, builds = %d, want a deployed fork preview", rec.Code, len(builder.calls))
	}
}

func TestForkPreview_MissingRepoInfoFailsClosed(t *testing.T) {
	rt, _, secret, builder := setUpPreviewApp(t)
	body, _ := json.Marshal(map[string]any{
		"action": "opened", "number": 42,
		"pull_request": map[string]any{
			"head": map[string]any{"ref": "feature-x", "sha": "sha1"},
			"base": map[string]any{"ref": "main"},
		},
	})
	sendPullRequestWebhook(rt, secret, body)
	if len(builder.calls) != 0 {
		t.Fatal("a pull request with no repository info deployed, want it held")
	}
}

func TestPreviewEnv_InjectedIntoPreviewContainer(t *testing.T) {
	rt, _, secret, builder := setUpPreviewApp(t)
	openPR(t, rt, secret, 7)

	if len(builder.calls) != 1 {
		t.Fatalf("builder calls = %d, want 1", len(builder.calls))
	}
	env := builder.calls[0].Service.Env
	if env["PREVIEW_PR_NUMBER"].Value != "7" || env["PREVIEW_BRANCH"].Value != "feature-x" {
		t.Errorf("env = %+v, want PREVIEW_PR_NUMBER=7 and PREVIEW_BRANCH=feature-x", env)
	}
	if _, ok := env["PREVIEW_URL"]; ok {
		t.Errorf("PREVIEW_URL set with no primary domain: %+v", env["PREVIEW_URL"])
	}
}

func TestPipelineRunEnv_ResolvesLivePreview(t *testing.T) {
	rt, _, secret, _ := setUpPreviewApp(t)
	openPR(t, rt, secret, 7)

	run := store.PipelineRun{AppName: "web", TriggerKind: "pull_request", Ref: "refs/heads/feature-x"}
	env := rt.PipelineRunEnv(context.Background(), run)
	if env["PREVIEW_PR_NUMBER"] != "7" || env["PREVIEW_BRANCH"] != "feature-x" {
		t.Errorf("env = %v", env)
	}
	run.TriggerKind = "push"
	if got := rt.PipelineRunEnv(context.Background(), run); got != nil {
		t.Errorf("push run got %v, want nothing", got)
	}
}

func TestPreviewPolicyRoutes(t *testing.T) {
	rt, db, _, _ := setUpPreviewApp(t)
	cookie := loginTestSession(t, rt, db)
	rt.previewLimits = PreviewLimits{MaxPerApp: 5, MaxTotal: 9}

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/preview-policy", body))
		return rec
	}
	if rec := put(`{"on_limit":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad on_limit: status = %d, want 400", rec.Code)
	}
	if rec := put(`{"ttl_hours":-1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("negative ttl: status = %d, want 400", rec.Code)
	}
	rec := put(`{"on_limit":"reject","allow_fork_previews":true,"ttl_hours":12}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/preview-policy", ""))
	var got previewPolicyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OnLimit != "reject" || !got.AllowForkPreviews || got.TTLHours != 12 || got.EffectiveTTLHours != 12 || got.MaxPerApp != 5 || got.MaxTotal != 9 {
		t.Errorf("policy = %+v", got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/previews", ""))
	var overview previewsOverviewResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &overview) != nil || overview.Limits.MaxTotal != 9 {
		t.Errorf("overview: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
