package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSleepRequestSource(t *testing.T) {
	for _, tc := range []struct{ host, path, ua, want string }{
		{"web.example.com", "/", "Mozilla/5.0", "web.example.com"},
		{"web.example.com", "/healthz", "Mozilla/5.0", "health probe"},
		{"web.example.com", "/", "kube-probe/1.29", "health probe"},
		{"web.example.com", "/", "Googlebot/2.1", "bot"},
	} {
		if got := sleepRequestSource(tc.host, tc.path, tc.ua); got != tc.want {
			t.Errorf("%+v = %q, want %q", tc, got, tc.want)
		}
	}
}

func TestParseSleepSince(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	if got, ok := parseSleepSince("7d", now); !ok || !got.Equal(now.AddDate(0, 0, -7)) {
		t.Errorf("7d = %v %v", got, ok)
	}
	if got, ok := parseSleepSince("36h", now); !ok || !got.Equal(now.Add(-36*time.Hour)) {
		t.Errorf("36h = %v %v", got, ok)
	}
	if _, ok := parseSleepSince("soon", now); ok {
		t.Error("garbage accepted")
	}
}

func TestSleepHistoryRecordsAndServes(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	rt.wakeToken = "tok"
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Domains: []string{"web.example.com"}, Resources: &store.ServiceResources{MemoryBytes: 512 << 20}}); err != nil {
		t.Fatal(err)
	}
	if rec := sleepPut(t, rt.handleSetAppSleep, `{"idle_minutes":30}`); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/sleep/now", nil)
	req.SetPathValue("name", "web")
	rt.handleSleepAppNow(httptest.NewRecorder(), req)

	hook := httptest.NewRequest(http.MethodGet, ingress.WakePath, nil)
	hook.Host = "web.example.com"
	hook.Header.Set(ingress.WakeTokenHeader, "tok")
	hook.Header.Set(ingress.WakeAppHeader, "web")
	hook.Header.Set(ingress.WakeURIHeader, "/dashboard?x=1")
	rt.handleWakeHook(httptest.NewRecorder(), hook)

	get := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/sleep/history?since=7d&limit=10", nil)
	get.SetPathValue("name", "web")
	rec := httptest.NewRecorder()
	rt.handleAppSleepHistory(rec, get)
	if rec.Code != http.StatusOK {
		t.Fatalf("history = %d %s", rec.Code, rec.Body)
	}
	var out sleepHistoryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]sleepEventResource{}
	for _, e := range out.Events {
		kinds[e.Kind] = e
	}
	if _, ok := kinds[store.SleepEventEnabled]; !ok {
		t.Errorf("no enabled event: %+v", out.Events)
	}
	if s := kinds[store.SleepEventSlept]; s.Reason != store.SleepReasonManual {
		t.Errorf("slept = %+v", s)
	}
	w := kinds[store.SleepEventWoke]
	if w.Reason != store.SleepReasonRequest || w.TriggerPath != "/dashboard" || w.TriggerSource != "web.example.com" || w.DurationSeconds == nil {
		t.Errorf("woke = %+v", w)
	}
	if out.Stats.WakeCount7d != 1 || out.Stats.LastWakeReason != store.SleepReasonRequest || out.Stats.MemoryGiBHoursSaved30d == nil {
		t.Errorf("stats = %+v", out.Stats)
	}
	tl, _ := db.ListAppEvents(ctx, "web", nil, time.Time{}, []string{store.AppEventSlept, store.AppEventWoke}, 10)
	if len(tl) != 2 {
		t.Errorf("timeline entries = %d, want 2", len(tl))
	}

	bad := httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/sleep/history?since=nope", nil)
	bad.SetPathValue("name", "web")
	br := httptest.NewRecorder()
	rt.handleAppSleepHistory(br, bad)
	if br.Code != http.StatusBadRequest {
		t.Errorf("bad since = %d", br.Code)
	}
}
