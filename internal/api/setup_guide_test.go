package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/setupguide"
	"github.com/GLINCKER/levelrail/internal/store"
)

func decodeGuide(t *testing.T, rec *httptest.ResponseRecorder) setupguide.Guide {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var g setupguide.Guide
	if err := json.NewDecoder(rec.Body).Decode(&g); err != nil {
		t.Fatalf("decode guide: %v", err)
	}
	return g
}

func guideStep(t *testing.T, g setupguide.Guide, id setupguide.StepID) setupguide.Step {
	t.Helper()
	for _, s := range g.Steps {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("step %q missing from guide", id)
	return setupguide.Step{}
}

func seedGuideToken(t *testing.T, db *store.DB, id string, abilities ...string) string {
	t.Helper()
	plaintext := "tok-" + id //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: id, Name: id, TokenHash: hashToken(plaintext), Abilities: abilities, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	return plaintext
}

func guideTokenRequest(method, target, body, plaintext string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	return req
}

func TestSetupGuideRequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	for _, c := range []struct{ method, target string }{
		{http.MethodGet, "/api/v1/setup/guide"},
		{http.MethodPut, "/api/v1/setup/guide/steps/teammate"},
		{http.MethodPut, "/api/v1/setup/guide/visibility"},
		{http.MethodPost, "/api/v1/setup/guide/reset"},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(c.method, c.target, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", c.method, c.target, rec.Code)
		}
	}
}

func TestSetupGuideAbilities(t *testing.T) {
	rt, db := newTestRouter(t)
	reader := seedGuideToken(t, db, "tok_guide_read", AbilityRead)
	writer := seedGuideToken(t, db, "tok_guide_write", AbilityWrite)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, guideTokenRequest(http.MethodGet, "/api/v1/setup/guide", "", reader))
	if rec.Code != http.StatusOK {
		t.Fatalf("read token GET = %d, want 200", rec.Code)
	}

	for _, tok := range []string{reader, writer} {
		for _, c := range []struct{ method, target, body string }{
			{http.MethodPut, "/api/v1/setup/guide/steps/teammate", `{"state":"skipped"}`},
			{http.MethodPut, "/api/v1/setup/guide/visibility", `{"hidden":true}`},
			{http.MethodPost, "/api/v1/setup/guide/reset", ""},
		} {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, guideTokenRequest(c.method, c.target, c.body, tok))
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s with %s = %d, want 403", c.method, c.target, tok, rec.Code)
			}
		}
	}
	state, _ := db.GetSetupGuideState(context.Background())
	if state.Hidden || len(state.Overrides) != 0 {
		t.Fatalf("a forbidden call changed state: %+v", state)
	}
}

func TestSetupGuideRootFlow(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, target, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
		return rec
	}

	g := decodeGuide(t, do(http.MethodGet, "/api/v1/setup/guide", ""))
	if s := guideStep(t, g, setupguide.StepFirstApp); s.State != setupguide.StateTodo {
		t.Fatalf("first_app on a fresh instance = %s, want todo", s.State)
	}
	if g.ReadyPercent != 0 || g.Hidden || g.Complete {
		t.Fatalf("fresh guide = percent %d hidden %v complete %v", g.ReadyPercent, g.Hidden, g.Complete)
	}

	g = decodeGuide(t, do(http.MethodPut, "/api/v1/setup/guide/steps/teammate", `{"state":"skipped"}`))
	if s := guideStep(t, g, setupguide.StepTeammate); s.State != setupguide.StateSkipped {
		t.Fatalf("teammate after skip = %s, want skipped", s.State)
	}
	g = decodeGuide(t, do(http.MethodPut, "/api/v1/setup/guide/steps/teammate", `{"state":"todo"}`))
	if s := guideStep(t, g, setupguide.StepTeammate); s.State == setupguide.StateSkipped {
		t.Fatal("undo did not clear the skip")
	}

	if rec := do(http.MethodPut, "/api/v1/setup/guide/steps/bogus", `{"state":"skipped"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown step = %d, want 404", rec.Code)
	}
	if rec := do(http.MethodPut, "/api/v1/setup/guide/steps/teammate", `{"state":"maybe"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad state = %d, want 400", rec.Code)
	}

	g = decodeGuide(t, do(http.MethodPut, "/api/v1/setup/guide/visibility", `{"hidden":true}`))
	if !g.Hidden {
		t.Fatal("hide did not persist")
	}
	_ = decodeGuide(t, do(http.MethodPut, "/api/v1/setup/guide/steps/upgrades", `{"state":"skipped"}`))
	g = decodeGuide(t, do(http.MethodPost, "/api/v1/setup/guide/reset", ""))
	if g.Hidden || guideStep(t, g, setupguide.StepUpgrades).State == setupguide.StateSkipped {
		t.Fatal("reset did not clear hidden and overrides")
	}
}

func TestSetupGuideFollowsRealState(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	get := func() setupguide.Guide {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/setup/guide", ""))
		return decodeGuide(t, rec)
	}

	if s := guideStep(t, get(), setupguide.StepBackupTarget); s.State != setupguide.StateTodo {
		t.Fatalf("backup_target before = %s, want todo", s.State)
	}
	if err := db.SaveBackupTarget(ctx, store.BackupTarget{ID: "tgt_1", Name: "s3", Provider: "aws", Bucket: "backups"}); err != nil {
		t.Fatalf("save backup target: %v", err)
	}
	g := get()
	if s := guideStep(t, g, setupguide.StepBackupTarget); s.State != setupguide.StateDone {
		t.Fatalf("backup_target after = %s, want done", s.State)
	}
	if s := guideStep(t, g, setupguide.StepAppDomain); len(s.BlockedBy) == 0 {
		t.Fatal("app_domain should be blocked until an app exists")
	}

	seedApp(t, db, "web")
	if s := guideStep(t, get(), setupguide.StepFirstApp); s.State != setupguide.StateDone {
		t.Fatalf("first_app after seeding an app = %s, want done", s.State)
	}

	if err := db.SetDashboardURL(ctx, "http://203.0.113.7:8080"); err != nil {
		t.Fatalf("set dashboard url: %v", err)
	}
	s := guideStep(t, get(), setupguide.StepPublicAddress)
	if s.State != setupguide.StateInProgress || s.Reason != setupguide.ReasonHTTPOnly {
		t.Fatalf("public_address with http url = (%s, %q), want in_progress/http_only", s.State, s.Reason)
	}
}
