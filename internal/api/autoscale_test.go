package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAutoscaleLifecycle(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Replicas: 3}); err != nil {
		t.Fatal(err)
	}
	if rec := budgetDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/autoscale", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("empty get = %d", rec.Code)
	}
	for name, body := range map[string]string{
		"max required":     `{"min_replicas":1,"metric":"cpu_percent","target":60}`,
		"max below min":    `{"min_replicas":4,"max_replicas":2,"metric":"cpu_percent","target":60}`,
		"over the ceiling": `{"min_replicas":1,"max_replicas":500,"metric":"cpu_percent","target":60}`,
		"bad metric":       `{"min_replicas":1,"max_replicas":5,"metric":"latency","target":60}`,
		"bad target":       `{"min_replicas":1,"max_replicas":5,"metric":"cpu_percent","target":0}`,
		"junk":             `nope`,
	} {
		if rec := budgetDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/autoscale", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, rec.Code)
		}
	}
	if rec := budgetDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/ghost/autoscale", `{"min_replicas":1,"max_replicas":5,"metric":"cpu_percent","target":60}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown app = %d", rec.Code)
	}

	rec := budgetDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/autoscale", `{"min_replicas":2,"max_replicas":6,"metric":"cpu_percent","target":60}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	var st autoscale.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.MaxStepUp != autoscale.DefaultMaxStepUp || st.ScaleDownCooldownSecs <= st.ScaleUpCooldownSecs || st.Current != 3 {
		t.Errorf("defaults not applied: %+v", st)
	}
	if st.Condition.Reason != autoscale.ReasonMetricsUnavailable || st.Condition.Message == "" || st.ValueKnown {
		t.Errorf("without telemetry the controller must hold and say why: %+v", st.Condition)
	}

	rec = budgetDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/autoscale/history", "")
	var hist autoscaleHistoryResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &hist) != nil || len(hist.Events) != 2 {
		t.Fatalf("history = %d %s", rec.Code, rec.Body)
	}
	if hist.Events[0].Kind != autoscale.EventHeld || hist.Events[1].Kind != autoscale.EventConfigured {
		t.Errorf("history = %+v", hist.Events)
	}
	if rec := budgetDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/autoscale/history?limit=0", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad limit = %d", rec.Code)
	}

	if rec := budgetDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/autoscale", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := budgetDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/autoscale", ""); rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d", rec.Code)
	}
	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil || svc.Replicas != 3 {
		t.Fatalf("clearing must leave replicas alone: %+v %v", svc, err)
	}
}

func TestAutoscaleRejectsManualReplicaEdits(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Replicas: 3}); err != nil {
		t.Fatal(err)
	}
	put := func(body string) *httptest.ResponseRecorder {
		return budgetDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web", body)
	}
	if rec := put(`{"name":"web","image":"nginx:1","port":80,"replicas":5}`); rec.Code != http.StatusOK {
		t.Fatalf("without a policy replicas are editable, got %d %s", rec.Code, rec.Body)
	}
	pol := autoscale.Policy{App: "web", MinReplicas: 1, MaxReplicas: 8, Metric: autoscale.MetricCPUPercent, Target: 60}.Normalize()
	if err := db.SaveAutoscalePolicy(ctx, pol, autoscale.Event{App: "web", Kind: autoscale.EventConfigured, At: time.Now()}, time.Now()); err != nil {
		t.Fatal(err)
	}
	rec := put(`{"name":"web","image":"nginx:1","port":80,"replicas":2}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "autoscale") {
		t.Fatalf("manual change = %d %s, want 409 naming autoscale", rec.Code, rec.Body)
	}
	if rec := put(`{"name":"web","image":"nginx:1","port":80,"replicas":5}`); rec.Code != http.StatusOK {
		t.Fatalf("an unchanged count is fine, got %d %s", rec.Code, rec.Body)
	}
	if rec := put(`{"name":"web","image":"nginx:1","port":80}`); rec.Code != http.StatusOK {
		t.Fatalf("an unset count is fine, got %d %s", rec.Code, rec.Body)
	}
	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil || svc.Replicas != 5 {
		t.Fatalf("replicas = %d, err = %v; an unset count must keep the autoscaler's value", svc.Replicas, err)
	}
}

func TestAutoscaleAppDeleteCascades(t *testing.T) {
	t.Parallel()
	_, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	pol := autoscale.Policy{App: "web", MinReplicas: 1, MaxReplicas: 3, Metric: autoscale.MetricCPUPercent, Target: 50}.Normalize()
	if err := db.SaveAutoscalePolicy(ctx, pol, autoscale.Event{App: "web", Kind: autoscale.EventConfigured, At: time.Now()}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetAutoscalePolicy(ctx, "web"); err == nil {
		t.Error("policy survived its app")
	}
}
