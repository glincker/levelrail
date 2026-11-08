package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func nextAdvisorGet(t *testing.T, rt *Router, db *store.DB, app string) (nextAdvisorResponse, string) {
	t.Helper()
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/"+app+"/next-advisor", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var out nextAdvisorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out, rec.Body.String()
}

func nextAdvisorIDs(r nextAdvisorResponse) map[string]bool {
	ids := map[string]bool{}
	for _, f := range r.Findings {
		ids[f.ID] = true
	}
	return ids
}

// seedNextApp saves svc plus one succeeded Next.js attempt.
func seedNextApp(t *testing.T, db *store.DB, svc store.DesiredService) {
	t.Helper()
	ctx := context.Background()
	svc.Image, svc.Port = "levelrail/web:1", 3000
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "dep_1", ServiceName: svc.Name, Image: "levelrail/web:1", CommitSHA: "abc123", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(), DetectedFramework: "Next.js"}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_1", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestNextAdvisorFindings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tests := []struct {
		name  string
		svc   store.DesiredService
		setup func(t *testing.T, rt *Router, db *store.DB)
		want  string
	}{
		{"replicas without redis", store.DesiredService{Name: "web", Replicas: 2}, nil, nextFindReplicasNoCache},
		{"redis without database", store.DesiredService{Name: "web", NextCache: "redis"}, nil, nextFindCacheNoDB},
		{"user deployment id", store.DesiredService{Name: "web", Env: map[string]string{"NEXT_DEPLOYMENT_ID": "my-own"}}, nil, nextFindSkewUserID},
		{"vitals off", store.DesiredService{Name: "web"}, nil, nextFindVitalsOff},
		{"grace off", store.DesiredService{Name: "web"}, func(t *testing.T, rt *Router, _ *store.DB) {
			if _, err := rt.runtimeSettings.Set(ctx, deploy.KeyNextAssetGraceSecs, json.RawMessage("0"), "test"); err != nil {
				t.Fatal(err)
			}
		}, nextFindGraceOff},
		{"build memory near host", store.DesiredService{Name: "web"}, func(t *testing.T, rt *Router, _ *store.DB) {
			rt.hostMemory = fixedHostMemory
			if _, err := rt.runtimeSettings.Set(ctx, runtimesettings.KeyBuildsMemoryLimitMB, json.RawMessage("15500"), "test"); err != nil {
				t.Fatal(err)
			}
		}, nextFindBuildMemory},
		{"tls redis without ca", store.DesiredService{Name: "web", NextCache: "redis", DatabaseEnv: map[string]store.DatabaseEnvRef{"LEVELRAIL_NEXT_CACHE_URL": {Database: "cache", Field: "url"}}}, func(t *testing.T, _ *Router, db *store.DB) {
			if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7"}); err != nil {
				t.Fatal(err)
			}
		}, nextFindCacheNoCA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt, db := newTestRouter(t)
			if tt.setup != nil {
				tt.setup(t, rt, db)
			}
			seedNextApp(t, db, tt.svc)
			got, body := nextAdvisorGet(t, rt, db, "web")
			if !got.Detected || !nextAdvisorIDs(got)[tt.want] {
				t.Fatalf("want finding %s in %s", tt.want, body)
			}
			for _, f := range got.Findings {
				if f.Severity == "" || f.Title == "" || f.Detail == "" || f.Fix == "" {
					t.Errorf("incomplete finding %+v", f)
				}
			}
			if strings.Contains(body, "my-own") {
				t.Errorf("finding leaked an env value: %s", body)
			}
		})
	}
}

func TestNextAdvisorHealthyAndNotNext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt, db := newTestRouter(t)
	if _, err := db.SetVitalsEnabled(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	// The platform-injected id (the attempt id) is not a user value.
	seedNextApp(t, db, store.DesiredService{Name: "web", Replicas: 1, Env: map[string]string{"NEXT_DEPLOYMENT_ID": "dep_1"}})
	got, body := nextAdvisorGet(t, rt, db, "web")
	if !got.Detected || len(got.Findings) != 0 || !strings.Contains(body, `"findings":[]`) {
		t.Fatalf("healthy app should have no findings: %s", body)
	}

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "api", Image: "levelrail/api:1", Port: 8080, Replicas: 3}); err != nil {
		t.Fatal(err)
	}
	got, _ = nextAdvisorGet(t, rt, db, "api")
	if got.Detected || len(got.Findings) != 0 {
		t.Fatalf("non Next.js app must be undetected: %+v", got)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, loginTestSession(t, rt, db), http.MethodGet, "/api/v1/apps/missing/next-advisor", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing app status = %d", rec.Code)
	}
}

// An upload deploy never sends detected_framework, so the build log note is
// what must mark the attempt and flip the advisor.
func TestNextAdvisorDetectsFromUploadBuildLog(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.deployAttempts = db
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "up", Image: "levelrail/up:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "dep_up", ServiceName: "up", Image: "levelrail/up:1", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if got, _ := nextAdvisorGet(t, rt, db, "up"); got.Detected {
		t.Fatal("detected before any Next.js log line")
	}
	progress := rt.recordDetectedFramework("dep_up", func(build.ProgressEvent) {})
	progress(build.ProgressEvent{Step: "x", Log: "go build ok"})
	progress(build.ProgressEvent{Step: "next.js", Log: "next.js 16 detected (standalone: yes, app router: yes)", Stream: "stderr"})
	if got, _ := nextAdvisorGet(t, rt, db, "up"); !got.Detected {
		t.Fatal("not detected after the Next.js build log line")
	}
}
