package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const singleWriterComposeYAML = `
services:
  gateway:
    image: ghcr.io/example/gateway:1
    x-levelrail-strategy: recreate
    x-levelrail-strategy-reason: locks its state volume
    ports: ["8080:80"]
`

func getAppBody(t *testing.T, rt *Router, cookie *http.Cookie, name string) appWithRequests {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/"+name, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET app %s: status = %d; body = %s", name, rec.Code, rec.Body.String())
	}
	var got appWithRequests
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return got
}

func TestDeployCompose_TemplateDefaults_StrategyMemoryAndNoLowering(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	const want = int64(2147483648)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/oc/compose?template=openclaw", singleWriterComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("deploy status = %d; body = %s", rec.Code, rec.Body.String())
	}
	got := getAppBody(t, rt, cookie, "oc-gateway")
	if got.Strategy != "recreate" || got.StrategyReason != "locks its state volume" {
		t.Errorf("strategy = %q reason = %q, want recreate with its reason", got.Strategy, got.StrategyReason)
	}
	if got.Resources == nil || got.Resources.MemoryBytes != want || !got.MemoryFromTemplate {
		t.Fatalf("resources = %+v from_template = %v, want %d from the template", got.Resources, got.MemoryFromTemplate, want)
	}

	// Half-success: the operator raised the limit, then the template is redeployed.
	svc, err := db.GetDesiredService(ctx, "oc-gateway")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	svc.Resources.MemoryBytes = want * 2
	if err := db.SaveDesiredService(ctx, *svc); err != nil {
		t.Fatalf("SaveDesiredService: %v", err)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/oc/compose?template=openclaw", singleWriterComposeYAML))
	if rec.Code != http.StatusOK {
		t.Fatalf("redeploy status = %d; body = %s", rec.Code, rec.Body.String())
	}
	got = getAppBody(t, rt, cookie, "oc-gateway")
	if got.Resources == nil || got.Resources.MemoryBytes != want*2 {
		t.Errorf("limit after redeploy = %+v, want the operator's %d kept", got.Resources, want*2)
	}
	if got.MemoryFromTemplate {
		t.Error("memory_from_template = true after the operator changed the limit")
	}
}

func TestListApps_HidesRetainedDeployServicesAndRefusesDelete(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, name := range []string{"web", "web-abc123"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:1", Port: 80}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	if err := db.SaveDeployURL(ctx, store.DeployURL{ServiceName: "web-abc123", AppName: "web", AttemptID: "da_1", Host: "web-abc123.example.com"}); err != nil {
		t.Fatalf("SaveDeployURL: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""))
	var list []appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 || list[0].Name != "web" {
		t.Fatalf("list = %+v, want only web", list)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web-abc123", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete status = %d, want 409", rec.Code)
	}
	if _, err := db.GetDesiredService(ctx, "web-abc123"); err != nil {
		t.Errorf("retained service gone after refused delete: %v", err)
	}
}

func TestTriggerDeploy_RollbackPreflight(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v2", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "da_old", ServiceName: "web", Image: "img:v1", Source: store.DeployAttemptSourceManual,
		Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("SaveDeployAttempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"x","rollback":true}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown target status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Code         string   `json:"code"`
		ValidTargets []string `json:"valid_targets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != errCodeRollbackTargetUnknown || len(body.ValidTargets) != 1 || body.ValidTargets[0] != "img:v1" {
		t.Errorf("body = %+v, want the code and [img:v1]", body)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); svc.Image != "img:v2" {
		t.Errorf("image = %q, a refused rollback must not change desired state", svc.Image)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"img:v1","rollback":true}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("known target status = %d, want 202; body = %s", rec.Code, rec.Body.String())
	}
}
