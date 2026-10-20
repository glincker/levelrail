package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBuildPromoteDiff(t *testing.T) {
	src := store.DesiredService{
		Image: "app:2", Replicas: 3,
		Env:       map[string]string{"A": "1", "NEW": "x", "SAME": "s", "DIFF": "src"},
		SecretEnv: []store.SecretEnvRef{{Name: "TOKEN"}, {Name: "EXTRA"}},
	}
	dst := store.DesiredService{
		Image: "app:1", Replicas: 1,
		Env:       map[string]string{"A": "1", "OLD": "y", "SAME": "s", "DIFF": "dst"},
		SecretEnv: []store.SecretEnvRef{{Name: "TOKEN"}, {Name: "GONE"}},
	}
	d := buildPromoteDiff(src, dst)
	if d.Image == nil || d.Image.From != "app:1" || d.Image.To != "app:2" {
		t.Errorf("image diff = %+v", d.Image)
	}
	if d.Replicas == nil || d.Replicas.To != "3" {
		t.Errorf("replicas diff = %+v", d.Replicas)
	}
	checks := map[string][2][]string{
		"env added":       {d.EnvAdded, {"NEW"}},
		"env removed":     {d.EnvRemoved, {"OLD"}},
		"env changed":     {d.EnvChanged, {"DIFF"}},
		"secrets added":   {d.SecretsAdded, {"EXTRA"}},
		"secrets removed": {d.SecretsGone, {"GONE"}},
	}
	for name, c := range checks {
		if !reflect.DeepEqual(c[0], c[1]) {
			t.Errorf("%s = %v, want %v", name, c[0], c[1])
		}
	}
	same := buildPromoteDiff(dst, dst)
	if same.Image != nil || same.Replicas != nil || len(same.EnvAdded)+len(same.EnvRemoved) != 0 {
		t.Errorf("identical apps produced a diff: %+v", same)
	}
}

func TestApplyPromoteEnv_KeepsTargetValues(t *testing.T) {
	src := store.DesiredService{Env: map[string]string{"NEW": "x", "DIFF": "src"}}
	dst := store.DesiredService{Env: map[string]string{"OLD": "y", "DIFF": "dst"}}
	applyPromoteEnv(&dst, src, buildPromoteDiff(src, dst))
	want := map[string]string{"NEW": "x", "DIFF": "dst"}
	if !reflect.DeepEqual(dst.Env, want) {
		t.Errorf("env = %v, want %v", dst.Env, want)
	}
}

func promoteCall(t *testing.T, rt *Router, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/promote", body))
	return w
}

func TestPromote_GuardrailsAndAudit(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	ctx := t.Context()

	if w := promoteCall(t, rt, cookie, `{"to":"env_prod"}`); w.Code != http.StatusConflict {
		t.Fatalf("production without confirm = %d, want 409", w.Code)
	}

	now := time.Now().UTC()
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "att1", ServiceName: "web-staging", Status: store.DeployAttemptStatusFailed, StartedAt: now,
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if w := promoteCall(t, rt, cookie, `{"to":"env_prod","confirm":true}`); w.Code != http.StatusConflict {
		t.Fatalf("failed last deploy = %d, want 409 (%s)", w.Code, w.Body.String())
	}
	if target, _ := db.GetDesiredService(ctx, "web-prod"); target.Image != "levelrail/web:1" {
		t.Fatalf("blocked promote changed the target: %s", target.Image)
	}

	if w := promoteCall(t, rt, cookie, `{"to":"env_prod","confirm":true,"force":true}`); w.Code != http.StatusAccepted {
		t.Fatalf("forced promote = %d (%s)", w.Code, w.Body.String())
	}
	if target, _ := db.GetDesiredService(ctx, "web-prod"); target.Image != "levelrail/web:2" {
		t.Errorf("target image = %s, want levelrail/web:2", target.Image)
	}
	entries, err := db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Path] = true
	}
	if !seen["/api/v1/apps/web-staging/bulk-promote-from"] || !seen["/api/v1/apps/web-prod/bulk-promote-to"] {
		t.Errorf("both sides must be audited, got %v", seen)
	}
}
