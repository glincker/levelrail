package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAssembleBuildRows_QueueOrderAndPositions(t *testing.T) {
	base := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	attempts := []store.DeployAttempt{
		{ID: "done-old", ServiceName: "web", Status: store.DeployAttemptStatusSucceeded, StartedAt: base.Add(-2 * time.Hour)},
		{ID: "q2", ServiceName: "web", Status: store.DeployAttemptStatusQueued, StartedAt: base, QueuedAt: ptrTime(base.Add(2 * time.Minute))},
		{ID: "run", ServiceName: "api", Status: store.DeployAttemptStatusRunning, StartedAt: base.Add(-time.Minute)},
		{ID: "q1", ServiceName: "api", Status: store.DeployAttemptStatusQueued, StartedAt: base, QueuedAt: ptrTime(base.Add(time.Minute))},
		{ID: "done-new", ServiceName: "api", Status: store.DeployAttemptStatusFailed, StartedAt: base.Add(-time.Hour)},
	}
	rows := assembleBuildRows(attempts, nil)
	var got []string
	for _, r := range rows {
		got = append(got, r.ID)
	}
	if want := "q1,q2,run,done-new,done-old"; strings.Join(got, ",") != want {
		t.Fatalf("order = %v, want %s", got, want)
	}
	if rows[0].QueuePosition != 1 || rows[1].QueuePosition != 2 || rows[2].QueuePosition != 0 {
		t.Fatalf("positions = %d %d %d", rows[0].QueuePosition, rows[1].QueuePosition, rows[2].QueuePosition)
	}
	if !rows[0].CanCancel || !rows[2].CanCancel || rows[3].CanCancel || !rows[3].CanRetry || rows[4].CanRetry {
		t.Fatal("cancel and retry flags do not follow status")
	}
	if c := countBuildRows(rows); c.Queued != 2 || c.Running != 1 || c.Finished != 2 || c.Failed != 1 {
		t.Fatalf("counts = %+v", c)
	}
}

func TestAssembleBuildRows_JoinsRunStatsAndImageDelta(t *testing.T) {
	base := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	fin := base.Add(time.Minute)
	attempts := []store.DeployAttempt{
		{ID: "a1", ServiceName: "web", Image: "web:aaa", Status: store.DeployAttemptStatusSucceeded, StartedAt: base, FinishedAt: &fin},
		{ID: "a2", ServiceName: "web", Image: "web:bbb", Status: store.DeployAttemptStatusSucceeded, StartedAt: base.Add(time.Hour), FinishedAt: ptrTime(base.Add(time.Hour + time.Minute))},
	}
	runs := []store.BuildRun{
		{App: "web", Tag: "web:bbb", StartedAt: base.Add(time.Hour + time.Second), DurationMS: 42000, Status: "success", CachedSteps: 3, TotalSteps: 4, NodeID: "n1", Platform: "linux/amd64", ImageBytes: 150},
		{App: "web", Tag: "web:aaa", StartedAt: base.Add(time.Second), DurationMS: 9000, Status: "success", CachedSteps: 0, TotalSteps: 4, ImageBytes: 100},
	}
	byID := map[string]buildRow{}
	for _, r := range assembleBuildRows(attempts, runs) {
		byID[r.ID] = r
	}
	r2 := byID["a2"]
	if r2.Node != "n1" || r2.Platform != "linux/amd64" || r2.ImageBytes != 150 {
		t.Fatalf("a2 = %+v", r2)
	}
	if r2.CacheHitRatio == nil || *r2.CacheHitRatio != 0.75 || r2.DurationSeconds == nil || *r2.DurationSeconds != 42 {
		t.Fatalf("a2 stats = %+v", r2)
	}
	if r2.ImageDeltaBytes == nil || *r2.ImageDeltaBytes != 50 {
		t.Fatalf("a2 delta = %v", r2.ImageDeltaBytes)
	}
	if byID["a1"].ImageDeltaBytes != nil {
		t.Fatal("the first build of an app has no previous build to compare with")
	}
}

func TestFailureHint_NoSpaceOffersCachePrune(t *testing.T) {
	tests := []struct {
		name    string
		err     string
		wantFix string
		wantNil bool
	}{
		{name: "disk full", err: "write /var/lib/docker/x: no space left on device", wantFix: buildFixPruneCache},
		{name: "platform mismatch", err: "exec /bin/sh: exec format error"},
		{name: "empty", err: "", wantNil: true},
		{name: "unknown", err: "something odd happened", wantNil: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := failureHint("web", tc.err)
			if tc.wantNil {
				if h != nil {
					t.Fatalf("hint = %+v, want none", h)
				}
				return
			}
			if h == nil || h.Fix != tc.wantFix {
				t.Fatalf("hint = %+v, want fix %q", h, tc.wantFix)
			}
		})
	}
}

func TestBuildCenter_ListCancelRetry(t *testing.T) {
	var retried []string
	rt, db, cookie, gb := newQueueRouter(t)
	rt.buildPerf = db
	if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "web", RepoURL: "https://example.com/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	rt.buildRetrier = func(_ context.Context, app, branch string) (string, error) {
		retried = append(retried, app+"@"+branch)
		return "queued", nil
	}
	first := postBuild(t, rt, cookie, "web", "main")
	gb.awaitStart(t)
	second := postBuild(t, rt, cookie, "web", "feature")

	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/builds", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	var resp buildCenterResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Counts.Queued != 1 || resp.Counts.Running != 1 || resp.Platforms.MultiPlatform {
		t.Fatalf("counts = %+v platforms = %+v", resp.Counts, resp.Platforms)
	}

	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds/"+second.ID+"/cancel", "")); rec.Code != http.StatusOK {
		t.Fatalf("cancel = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds/"+first.ID+"/retry", "")); rec.Code != http.StatusConflict {
		t.Fatalf("retry of a running build = %d, want 409", rec.Code)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds/"+second.ID+"/retry", "")); rec.Code != http.StatusAccepted {
		t.Fatalf("retry of a canceled build = %d %s", rec.Code, rec.Body.String())
	}
	if len(retried) != 1 || !strings.HasPrefix(retried[0], "web@") {
		t.Fatalf("retrier calls = %v", retried)
	}
	gb.release <- struct{}{}
}
