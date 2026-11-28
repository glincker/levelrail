package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/deploywhy"
	"github.com/glincker/levelrail/internal/store"
)

func TestDeployWhy_VerdictAndScopedDeny(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-time.Hour)
	for _, id := range []string{"good", "bad"} {
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: "web", Image: "img:" + id, Source: store.DeployAttemptSourceGitPush, Status: store.DeployAttemptStatusRunning, StartedAt: start}); err != nil {
			t.Fatal(err)
		}
		start = start.Add(time.Minute)
	}
	if err := db.FinishDeployAttempt(ctx, "good", store.DeployAttemptStatusSucceeded, start, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "bad", store.DeployAttemptStatusFailed, start.Add(time.Minute), "boom"); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceDeployLogTemplates(ctx, "good", []store.DeployLogTemplate{{DeployID: "good", TemplateID: "t1", Template: "ok", Count: 3, Sample: "ok"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceDeployLogTemplates(ctx, "bad", []store.DeployLogTemplate{
		{DeployID: "bad", TemplateID: "t1", Template: "ok", Count: 1, Sample: "ok"},
		{DeployID: "bad", TemplateID: "t2", Template: "panic <*>", Count: 4, Sample: "panic: Bearer abc123xyz"},
	}); err != nil {
		t.Fatal(err)
	}
	tok := seedMatrixToken(t, db, "why-tok", []string{AbilityRead})
	get := func(mutate func(*http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deploys/bad/why", nil)
		mutate(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := get(bearer(tok)); rec.Code != http.StatusOK {
		t.Fatalf("allowed token status = %d, want 200", rec.Code)
	}
	attachTestPolicy(t, db, "deny-web", "Deny", "*", "app:web", store.PrincipalTypeToken, "why-tok")
	if rec := get(bearer(tok)); rec.Code != http.StatusNotFound {
		t.Fatalf("denied token status = %d, want 404", rec.Code)
	}
	cookie := loginTestSession(t, rt, db)
	rec := get(func(r *http.Request) { r.AddCookie(cookie) })
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var rep deploywhy.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != deploywhy.VerdictNewLogs || rep.NewTemplateCount != 1 || rep.BaselineID != "good" {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if got := rep.NewTemplates[0].Sample; got != "panic: Bearer [redacted]" {
		t.Fatalf("sample not scrubbed: %q", got)
	}
}

type countingExits struct {
	DeployExitStore
	gets int
}

func (c *countingExits) GetDeployExit(ctx context.Context, svc, id string) (*store.DeployExit, error) {
	c.gets++
	return c.DeployExitStore.GetDeployExit(ctx, svc, id)
}

func TestDeployWhy_EmptyMiningIsRemembered(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-time.Hour)
	for _, id := range []string{"old", "fresh"} {
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: "web", Image: "img:1", Source: store.DeployAttemptSourceGitPush, Status: store.DeployAttemptStatusRunning, StartedAt: start}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.FinishDeployAttempt(ctx, "old", store.DeployAttemptStatusFailed, start.Add(time.Minute), "boom"); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "fresh", store.DeployAttemptStatusFailed, time.Now(), "boom"); err != nil {
		t.Fatal(err)
	}
	exits := &countingExits{DeployExitStore: db}
	rt.deployExits = exits
	cookie := loginTestSession(t, rt, db)
	why := func(id string) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deploys/"+id+"/why", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
	}
	why("old")
	first := exits.gets
	why("old")
	if first != 4 || exits.gets-first != 3 {
		t.Fatalf("exit reads: first call %d (want 4), second call %d (want 3)", first, exits.gets-first)
	}
	exits.gets = 0
	why("fresh")
	if exits.gets != 3 {
		t.Fatalf("a deploy inside the log tail window was mined: %d exit reads", exits.gets)
	}
	if rows, _ := db.ListDeployLogTemplates(ctx, "fresh"); len(rows) != 0 || rt.whyEmpty.has("fresh") {
		t.Fatalf("a deploy inside the log tail window was stored")
	}
}
