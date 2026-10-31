package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const stackV1 = `
services:
  db:
    image: postgres:16-alpine
    volumes: ["data:/var/lib/postgresql/data"]
  api:
    image: nginx:1.27
    depends_on:
      db: {condition: service_healthy}
  web:
    image: nginx:1.27
    ports: ["80"]
    depends_on: [api]
  debug:
    image: busybox
    profiles: [debug]
`

const stackV2 = `
services:
  db:
    image: postgres:16-alpine
    volumes: ["data:/var/lib/postgresql/data"]
  api:
    image: nginx:1.28
  web:
    image: nginx:1.28
    ports: ["80"]
`

func composeDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string, want int) []byte {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	if rec.Code != want {
		t.Fatalf("%s %s status = %d, want %d; body = %s", method, path, rec.Code, want, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func TestComposeStack_DeployStatusRollbackDown(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	var dep composeDeployResponse
	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/stack/compose", stackV1, http.StatusOK), &dep)
	if dep.Revision != 1 || len(dep.Services) != 3 || strings.Join(dep.Skipped, ",") != "debug" {
		t.Fatalf("deploy v1 = revision %d, %d services, skipped %v", dep.Revision, len(dep.Services), dep.Skipped)
	}
	api, err := db.GetDesiredService(ctx, "stack-api")
	if err != nil || strings.Join(api.DependsOnHealthy, ",") != "db" {
		t.Fatalf("stack-api DependsOnHealthy = %v, %v", api, err)
	}

	if err := db.UpsertConditions(ctx, applicationControllerName("stack-db"), []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue, Reason: "Running", LastTransitionTime: time.Now()}}); err != nil {
		t.Fatalf("UpsertConditions() error = %v", err)
	}
	if err := db.UpsertConditions(ctx, applicationControllerName("stack-api"), []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionFalse, Reason: "CreateFailed", Message: "pull denied", LastTransitionTime: time.Now()}}); err != nil {
		t.Fatalf("UpsertConditions() error = %v", err)
	}
	var stack composeStackResource
	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/stack/compose", "", http.StatusOK), &stack)
	if stack.Phase != composePhaseDegraded || stack.Revision != 1 || len(stack.Services) != 3 {
		t.Fatalf("stack = %+v, want degraded at revision 1 with 3 services", stack)
	}
	if s := stack.Services[0]; s.Key != "api" || s.Reason != "CreateFailed" || s.Message != "pull denied" {
		t.Errorf("api service view = %+v", s)
	}

	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/stack/compose?profiles=debug", stackV2, http.StatusOK), &dep)
	if dep.Revision != 2 {
		t.Fatalf("deploy v2 revision = %d, want 2", dep.Revision)
	}
	if web, _ := db.GetDesiredService(ctx, "stack-web"); web == nil || web.Image != "nginx:1.28" {
		t.Fatalf("after v2, stack-web = %+v", web)
	}

	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/stack/compose/rollback", "", http.StatusOK)
	if web, _ := db.GetDesiredService(ctx, "stack-web"); web == nil || web.Image != "nginx:1.27" {
		t.Fatalf("after rollback, stack-web = %+v, want nginx:1.27", web)
	}
	var revs struct {
		Revisions []composeRevisionResource `json:"revisions"`
	}
	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/stack/compose/revisions", "", http.StatusOK), &revs)
	if len(revs.Revisions) != 3 || revs.Revisions[0].Source != store.ComposeRevisionSourceRollback {
		t.Fatalf("revisions = %+v, want 3 with a rollback on top", revs.Revisions)
	}
	var one composeRevisionResource
	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/stack/compose/revisions/2", "", http.StatusOK), &one)
	if !strings.Contains(one.Compose, "nginx:1.28") || strings.Join(one.Profiles, ",") != "debug" {
		t.Errorf("revision 2 = %+v", one)
	}

	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/stack/compose/redeploy", `{"service":"web"}`, http.StatusAccepted)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/stack/compose/redeploy", `{"service":"nope"}`, http.StatusNotFound)

	var list struct {
		Stacks []composeStackListItem `json:"stacks"`
	}
	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodGet, "/api/v1/compose", "", http.StatusOK), &list)
	if len(list.Stacks) != 1 || list.Stacks[0].AppID != "stack" || list.Stacks[0].Revision != 3 {
		t.Fatalf("stacks = %+v", list.Stacks)
	}

	var down map[string]any
	_ = json.Unmarshal(composeDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/stack/compose", "", http.StatusOK), &down)
	if kept, _ := down["volumes_kept"].([]any); len(kept) != 1 {
		t.Errorf("down volumes_kept = %v, want the db volume", down["volumes_kept"])
	}
	if svcs, _ := db.ListServicesByApp(ctx, "stack"); len(svcs) != 0 {
		t.Errorf("after down, %d services remain", len(svcs))
	}
	composeDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/stack/compose", "", http.StatusNotFound)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/stack/compose/rollback", "", http.StatusNotFound)
}

func TestComposeStack_RollbackWithoutEarlierRevision(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/solo/compose", stackV2, http.StatusOK)
	composeDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/solo/compose/rollback", "", http.StatusConflict)
}

func TestComposeStackPhase(t *testing.T) {
	t.Parallel()
	svc := func(ready string, suspended bool) composeServiceStatus {
		return composeServiceStatus{Ready: ready, Suspended: suspended}
	}
	tr, fa, un := string(reconcile.ConditionTrue), string(reconcile.ConditionFalse), string(reconcile.ConditionUnknown)
	tests := []struct {
		name     string
		services []composeServiceStatus
		want     string
	}{
		{"empty", nil, composePhaseStopped},
		{"all ready", []composeServiceStatus{svc(tr, false), svc(tr, false)}, composePhaseRunning},
		{"one waiting", []composeServiceStatus{svc(tr, false), svc(un, false)}, composePhaseProgressing},
		{"one failed", []composeServiceStatus{svc(tr, false), svc(fa, false), svc(un, false)}, composePhaseDegraded},
		{"all failed", []composeServiceStatus{svc(fa, false), svc(fa, false)}, composePhaseFailed},
		{"all suspended", []composeServiceStatus{svc(un, true), svc(un, true)}, composePhaseStopped},
		{"ready plus suspended", []composeServiceStatus{svc(tr, false), svc(un, true)}, composePhaseRunning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := composeStackPhase(tt.services); got != tt.want {
				t.Errorf("composeStackPhase() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPreviewCompose(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		body      string
		profiles  []string
		wantValid bool
		check     func(t *testing.T, got composePreviewResponse)
	}{
		{
			name: "detects services, volumes, env kinds", wantValid: true,
			body: "services:\n  app:\n    image: ghost:5\n    ports: [\"2368\"]\n    volumes: [\"content:/var/lib/ghost/content\"]\n    environment:\n      DB_PASSWORD: ${SERVICE_PASSWORD_DB}\n      API_KEY: plain-secret\n      SMTP_HOST: ${SERVICE_SMTP_HOST}\n      URL: ${SERVICE_FQDN_APP}\n    healthcheck: {test: [\"CMD\", \"wget\", \"-q\", \"http://localhost:2368\"]}\n",
			check: func(t *testing.T, got composePreviewResponse) {
				svc := got.Services[0]
				if svc.Port != 2368 || len(svc.Volumes) != 1 || svc.Healthcheck == "" {
					t.Errorf("service = %+v", svc)
				}
				env := map[string]composePreviewEnv{}
				for _, e := range svc.Env {
					env[e.Key] = e
				}
				if !env["DB_PASSWORD"].Generated || env["API_KEY"].Value != "" || !env["API_KEY"].Secret || !env["SMTP_HOST"].Required || !env["URL"].Required {
					t.Errorf("env = %+v", env)
				}
			},
		},
		{
			name: "lists every validation error", wantValid: false,
			body: "services:\n  a: {image: x, privileged: true}\n  b: {image: y, network_mode: host}\n",
			check: func(t *testing.T, got composePreviewResponse) {
				if len(got.Errors) != 2 {
					t.Errorf("errors = %v, want 2", got.Errors)
				}
			},
		},
		{
			name: "profiles skip and list", wantValid: true, profiles: nil,
			body: "services:\n  a: {image: x}\n  b: {image: y, profiles: [extra]}\n",
			check: func(t *testing.T, got composePreviewResponse) {
				if len(got.Services) != 1 || strings.Join(got.Skipped, ",") != "b" || strings.Join(got.Profiles, ",") != "extra" {
					t.Errorf("got = %+v", got)
				}
			},
		},
		{
			name: "bind mount needs root", wantValid: true,
			body: "services:\n  a:\n    image: x\n    volumes: [\"/srv/data:/data\"]\n",
			check: func(t *testing.T, got composePreviewResponse) {
				if !got.NeedsRoot {
					t.Error("NeedsRoot = false, want true")
				}
			},
		},
		{name: "not yaml", body: "services: [", wantValid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := previewCompose([]byte(tt.body), tt.profiles)
			if got.Valid != tt.wantValid {
				t.Fatalf("Valid = %v, want %v; errors = %v", got.Valid, tt.wantValid, got.Errors)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
