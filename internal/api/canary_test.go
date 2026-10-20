package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func canaryCall(t *testing.T, h http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/apps/web/canary", strings.NewReader(body))
	req.SetPathValue("name", "web")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestCanaryLifecycle(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1@" + oldDigest, Port: 80}); err != nil {
		t.Fatal(err)
	}

	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2","weight":0}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("weight 0 start = %d, want 400", rec.Code)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2","weight":25}`); rec.Code != http.StatusCreated {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	clone, err := db.GetDesiredService(ctx, "web--canary")
	if err != nil || clone.Image != "nginx:2" || len(clone.Domains) != 0 {
		t.Fatalf("clone = %+v, %v", clone, err)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:3"}`); rec.Code != http.StatusConflict {
		t.Fatalf("second start = %d, want 409", rec.Code)
	}
	apps, _, err := db.ListDesiredServicesFiltered(ctx, store.AppListFilter{})
	if err != nil || len(apps) != 1 || apps[0].Name != "web" {
		t.Fatalf("app list = %+v, %v; canary clone must be hidden", apps, err)
	}

	if rec := canaryCall(t, rt.handleSetCanaryWeight, http.MethodPut, `{"weight":50}`); rec.Code != http.StatusOK {
		t.Fatalf("set weight = %d", rec.Code)
	}
	if rec := canaryCall(t, rt.handleSetCanaryWeight, http.MethodPut, `{"weight":100}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("weight 100 = %d, want 400", rec.Code)
	}
	if c, _, _ := db.GetCanaryRelease(ctx, "web"); c.Weight != 50 {
		t.Fatalf("weight = %d", c.Weight)
	}

	if rec := canaryCall(t, rt.handlePromoteCanary, http.MethodPost, ``); rec.Code != http.StatusOK {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body)
	}
	stable, _ := db.GetDesiredService(ctx, "web")
	if !strings.HasPrefix(stable.Image, "nginx:2") {
		t.Errorf("stable image = %q, want canary image", stable.Image)
	}
	if _, ok, _ := db.GetCanaryRelease(ctx, "web"); ok {
		t.Error("canary row survived promote")
	}
	if _, err := db.GetDesiredService(ctx, "web--canary"); err == nil {
		t.Error("canary clone survived promote")
	}
}

func TestCanaryAbortAndGuards(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Replicas: 3}); err != nil {
		t.Fatal(err)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2"}`); rec.Code != http.StatusConflict {
		t.Fatalf("multi-replica start = %d, want 409", rec.Code)
	}
	if rec := canaryCall(t, rt.handleAbortCanary, http.MethodDelete, ``); rec.Code != http.StatusNotFound {
		t.Fatalf("abort with none = %d, want 404", rec.Code)
	}

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Replicas: 1}); err != nil {
		t.Fatal(err)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2"}`); rec.Code != http.StatusCreated {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	if rec := canaryCall(t, rt.handleAbortCanary, http.MethodDelete, ``); rec.Code != http.StatusOK {
		t.Fatalf("abort = %d", rec.Code)
	}
	if _, err := db.GetDesiredService(ctx, "web--canary"); err == nil {
		t.Error("clone survived abort")
	}
	stable, _ := db.GetDesiredService(ctx, "web")
	if stable.Image != "nginx:1" {
		t.Errorf("abort changed stable image to %q", stable.Image)
	}
}

func TestCanaryRemovedWithApp(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2"}`); rec.Code != http.StatusCreated {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	if _, err := rt.deleteApp(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDesiredService(ctx, "web--canary"); err == nil {
		t.Error("canary clone outlived its app")
	}
	if _, ok, _ := db.GetCanaryRelease(ctx, "web"); ok {
		t.Error("canary row outlived its app")
	}
}

func TestCanaryStartRejectsUnresolvableImage(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{err: errors.New("manifest unknown")})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:nope"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("start = %d %s, want 400", rec.Code, rec.Body)
	}
	if _, err := db.GetDesiredService(ctx, "web--canary"); err == nil {
		t.Error("clone created for an unresolvable image")
	}
}

func TestCanaryStagedStartAndAutoRollback(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2","stages":[20,10]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("descending stages = %d, want 400", rec.Code)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2","stages":[10,50],"weight":5}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("stages with weight = %d, want 400", rec.Code)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2","stages":[10,50,100],"interval":"2m"}`); rec.Code != http.StatusCreated {
		t.Fatalf("staged start = %d %s", rec.Code, rec.Body)
	}
	rel, ok, _ := db.GetCanaryRelease(ctx, "web")
	if !ok || !rel.Staged() || rel.Weight != 10 || rel.IntervalSecs != 120 || rel.MaxP95Ratio <= 1 {
		t.Fatalf("release = %+v", rel)
	}

	if err := rt.CanaryActions().Rollback(ctx, rel, "error rate too high"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetCanaryRelease(ctx, "web"); ok {
		t.Error("release survived rollback")
	}
	if _, err := db.GetDesiredService(ctx, "web--canary"); err == nil {
		t.Error("clone survived rollback")
	}
	o, ok, _ := db.GetCanaryOutcome(ctx, "web")
	if !ok || o.Outcome != store.CanaryOutcomeRolledBack || o.Reason != "error rate too high" {
		t.Fatalf("outcome = %+v", o)
	}
	rec := canaryCall(t, rt.handleGetCanary, http.MethodGet, ``)
	if !strings.Contains(rec.Body.String(), `"rolled_back"`) {
		t.Errorf("GET after rollback should report the last outcome: %s", rec.Body)
	}
}

func TestCanaryStartUsesSavedPolicy(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetCanaryPolicy(ctx, store.CanaryPolicy{ServiceName: "web", Stages: []int{5, 100}, IntervalSecs: 30, MaxErrorRate: 0.1, MaxP95Ratio: 2}); err != nil {
		t.Fatal(err)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2"}`); rec.Code != http.StatusCreated {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	rel, _, _ := db.GetCanaryRelease(ctx, "web")
	if !rel.Staged() || rel.Weight != 5 || rel.IntervalSecs != 30 || rel.MaxErrorRate != 0.1 {
		t.Fatalf("saved policy not applied: %+v", rel)
	}
}
