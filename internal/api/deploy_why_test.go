package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploywhy"
	"github.com/GLINCKER/levelrail/internal/store"
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
		{DeployID: "bad", TemplateID: "t2", Template: "panic <*>", Count: 4, Sample: "panic: nil"},
	}); err != nil {
		t.Fatal(err)
	}
	tok := seedMatrixToken(t, db, "why-tok", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-web", "Deny", "*", "app:web", store.PrincipalTypeToken, "why-tok")
	get := func(mutate func(*http.Request)) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/deploys/bad/why", nil)
		mutate(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
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
}
