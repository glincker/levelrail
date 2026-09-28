package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/preflight"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandlePreflightNew_RequiredEnvAndMounts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"web","required_env":["DATABASE_URL"],"env_keys":[],"bind_mounts":["/proc/x"]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/preflight", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	var rep preflight.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	statuses := map[string]preflight.Status{}
	for _, c := range rep.Checks {
		statuses[c.ID] = c.Status
	}
	if rep.Status != preflight.StatusFail || statuses["env"] != preflight.StatusFail || statuses["mounts"] != preflight.StatusFail {
		t.Fatalf("report = %+v", rep)
	}
}

func TestHandlePreflightApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/ghost/preflight", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing app status = %d", rec.Code)
	}

	svc := store.DesiredService{Name: "web", Image: "", Port: 3000, Env: map[string]string{"API_KEY": ""}}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/preflight", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	var rep preflight.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, c := range rep.Checks {
		if c.ID == "env" && c.Status == preflight.StatusFail {
			found = true
		}
	}
	if !found {
		t.Fatalf("an env var declared with an empty value should fail: %+v", rep)
	}
}

func TestHandleDiagnoseApp_TypedCauseFromLogs(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	res := &store.ServiceResources{MemoryBytes: 256 << 20}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000, Resources: res}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_1", ServiceName: "web", Image: "levelrail/web:1",
		Source: store.DeployAttemptSourceImage,
		Status: store.DeployAttemptStatusRunning, StartedAt: base,
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_1", store.DeployAttemptStatusFailed, base.Add(time.Minute), "container exited: Error: DATABASE_URL is not set"); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/diagnose", ""))
	var got diagnosisResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Causes) == 0 || got.Causes[0].Code != "MISSING_ENV" {
		t.Fatalf("causes = %+v", got.Causes)
	}
}
