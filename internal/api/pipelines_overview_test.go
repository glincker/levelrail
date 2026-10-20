package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type overviewSeed struct {
	id, app, pipeline, trigger, status, hold string
	age                                      time.Duration
}

func seedOverviewRuns(t *testing.T, db *store.DB, seeds []overviewSeed) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	pipes := map[string]store.Pipeline{}
	for _, s := range seeds {
		key := s.app + "/" + s.pipeline
		if _, ok := pipes[key]; !ok {
			p, err := db.SavePipeline(ctx, store.Pipeline{ID: "p-" + s.app + "-" + s.pipeline, AppName: s.app, Name: s.pipeline, YAML: apiPipelineYAML, Enabled: true, CreatedAt: now, UpdatedAt: now})
			if err != nil {
				t.Fatalf("seed pipeline: %v", err)
			}
			pipes[key] = p
		}
		created := now.Add(-s.age)
		if _, err := db.CreatePipelineRun(ctx, store.PipelineRun{ID: s.id, PipelineID: pipes[key].ID, AppName: s.app, TriggerKind: s.trigger,
			CommitSHA: "0123456789abcdef", Definition: apiPipelineYAML, CreatedAt: created, HoldState: s.hold}); err != nil {
			t.Fatalf("seed run: %v", err)
		}
		started := created
		var finished *time.Time
		if store.IsPipelineTerminal(s.status) {
			f := created.Add(30 * time.Second)
			finished = &f
		}
		if err := db.SetPipelineRunStatus(ctx, s.id, s.status, "", &started, finished); err != nil {
			t.Fatalf("seed status: %v", err)
		}
	}
}

func getOverview(t *testing.T, rt *Router, cookie *http.Cookie, path string) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
	return rec.Code, rec.Body.Bytes()
}

func TestPipelineOverviewRuns_RequireAuth(t *testing.T) {
	rt, _, _ := newPipelineRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/pipeline-runs"},
		{http.MethodGet, "/api/v1/pipelines/summary"},
	})
}

func TestPipelineOverviewRuns_FiltersAndFields(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedOverviewRuns(t, db, []overviewSeed{
		{"r1", "web", "ci", "push", store.PipelineStatusSucceeded, "", 5 * time.Minute},
		{"r2", "web", "ci", "manual", store.PipelineStatusFailed, "", 4 * time.Minute},
		{"r3", "api", "release", "push", store.PipelineStatusRunning, "", 3 * time.Minute},
		{"r4", "api", "release", "pull_request", store.PipelineStatusQueued, store.HoldPending, 2 * time.Minute},
	})

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"all newest first", "", []string{"r4", "r3", "r2", "r1"}},
		{"failed", "?status=failed", []string{"r2"}},
		{"running", "?status=running", []string{"r4", "r3"}},
		{"held", "?status=held", []string{"r4"}},
		{"app", "?app=web", []string{"r2", "r1"}},
		{"pipeline", "?pipeline=release", []string{"r4", "r3"}},
		{"trigger", "?trigger=push", []string{"r3", "r1"}},
		{"combined", "?app=api&trigger=push", []string{"r3"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body := getOverview(t, rt, cookie, "/api/v1/pipeline-runs"+tc.query)
			if code != http.StatusOK {
				t.Fatalf("status %d: %s", code, body)
			}
			var page pipelineRunRowsPage
			if err := json.Unmarshal(body, &page); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range page.Runs {
				got = append(got, r.ID)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ids = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ids = %v, want %v", got, tc.want)
				}
			}
		})
	}

	_, body := getOverview(t, rt, cookie, "/api/v1/pipeline-runs?status=held")
	var page pipelineRunRowsPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	r := page.Runs[0]
	if !r.HoldPending || !r.CanDecide || r.ShortSHA != "0123456" || r.Pipeline != "release" || r.App != "api" || r.Trigger != "pull_request" {
		t.Fatalf("held row = %+v", r)
	}
}

func TestPipelineOverviewRuns_BadParams(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, q := range []string{"?status=bogus", "?trigger=bogus", "?limit=0", "?limit=x", "?cursor=!!"} {
		if code, _ := getOverview(t, rt, cookie, "/api/v1/pipeline-runs"+q); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", q, code)
		}
	}
}

func TestPipelineOverviewRuns_CursorPagination(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	var seeds []overviewSeed
	for i := 0; i < 5; i++ {
		seeds = append(seeds, overviewSeed{"r" + strconv.Itoa(i), "web", "ci", "push", store.PipelineStatusSucceeded, "", time.Duration(i+1) * time.Minute})
	}
	seedOverviewRuns(t, db, seeds)

	var ids []string
	cursor := ""
	for pages := 0; pages < 5; pages++ {
		path := "/api/v1/pipeline-runs?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		code, body := getOverview(t, rt, cookie, path)
		if code != http.StatusOK {
			t.Fatalf("status %d: %s", code, body)
		}
		var page pipelineRunRowsPage
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		for _, r := range page.Runs {
			ids = append(ids, r.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	want := []string{"r0", "r1", "r2", "r3", "r4"}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
}

func TestPipelineOverview_PerAppAccess(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	bootstrapTestAdmin(t, db)
	seedOverviewRuns(t, db, []overviewSeed{
		{"r1", "prod", "ci", "push", store.PipelineStatusFailed, "", time.Minute},
		{"r2", "staging", "ci", "push", store.PipelineStatusFailed, "", 2 * time.Minute},
		{"r3", "staging", "ci", "push", store.PipelineStatusSucceeded, "", 3 * time.Minute},
	})
	user := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-prod-read", "Deny", AbilityRead, "app:prod", store.PrincipalTypeUser, user.ID)
	cookie := sessionCookieForTest(t, rt, user.ID)

	code, body := getOverview(t, rt, cookie, "/api/v1/pipeline-runs?limit=1")
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var page pipelineRunRowsPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Runs) != 1 || page.Runs[0].ID != "r2" || page.NextCursor == "" {
		t.Fatalf("page = %+v, want r2 with a next cursor (prod row hidden)", page)
	}

	code, body = getOverview(t, rt, cookie, "/api/v1/pipelines/summary")
	if code != http.StatusOK {
		t.Fatalf("summary status %d: %s", code, body)
	}
	var sum pipelineSummaryResource
	if err := json.Unmarshal(body, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Failed24h != 1 || sum.Succeeded24h != 1 || sum.SuccessRate24h == nil || *sum.SuccessRate24h != 0.5 {
		t.Fatalf("summary = %+v, want prod excluded", sum)
	}
}

func TestPipelineOverviewSummary(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)

	code, body := getOverview(t, rt, cookie, "/api/v1/pipelines/summary")
	var empty pipelineSummaryResource
	if err := json.Unmarshal(body, &empty); code != http.StatusOK || err != nil || empty.SuccessRate24h != nil {
		t.Fatalf("empty summary: %d %s", code, body)
	}

	seedOverviewRuns(t, db, []overviewSeed{
		{"r1", "web", "ci", "push", store.PipelineStatusSucceeded, "", time.Hour},
		{"r2", "web", "ci", "push", store.PipelineStatusSucceeded, "", 2 * time.Hour},
		{"r3", "web", "ci", "push", store.PipelineStatusSucceeded, "", 3 * time.Hour},
		{"r4", "web", "ci", "push", store.PipelineStatusFailed, "", 4 * time.Hour},
		{"r5", "web", "ci", "push", store.PipelineStatusFailed, "", 48 * time.Hour},
		{"r6", "web", "ci", "push", store.PipelineStatusRunning, "", time.Minute},
		{"r7", "web", "ci", "push", store.PipelineStatusWaitingApproval, "", 2 * time.Minute},
		{"r8", "web", "ci", "pull_request", store.PipelineStatusQueued, store.HoldPending, 3 * time.Minute},
	})
	_, body = getOverview(t, rt, cookie, "/api/v1/pipelines/summary")
	var sum pipelineSummaryResource
	if err := json.Unmarshal(body, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Running != 2 || sum.WaitingApproval != 1 || sum.Held != 1 || sum.Succeeded24h != 3 || sum.Failed24h != 1 ||
		sum.SuccessRate24h == nil || *sum.SuccessRate24h != 0.75 {
		t.Fatalf("summary = %+v", sum)
	}
}
