package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/store"
)

func shortWakeHold(t *testing.T, hold time.Duration) {
	t.Helper()
	d := hold.String()
	t.Setenv(appsleep.EnvHoldMin, d)
	t.Setenv(appsleep.EnvHoldMax, d)
	t.Setenv(appsleep.EnvHoldDefault, d)
	t.Setenv(appsleep.EnvWakeProbeInterval, "20ms")
	prev := wakeIngressSettle
	wakeIngressSettle = 10 * time.Millisecond
	t.Cleanup(func() { wakeIngressSettle = prev })
}

func ptr64(v int64) *int64 { return &v }

func TestWakingResponseNegotiation(t *testing.T) {
	rt, _, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	rt.brand = &brand.Brand{Name: "Acme Cloud", PrimaryColor: "#aa5500"}
	history := store.WakeTiming{Samples: 3, ReadyP50Ms: ptr64(4200)}
	tests := []struct {
		name        string
		accept      string
		timing      store.WakeTiming
		contentType string
		contains    []string
		absent      []string
		retryAfter  string
	}{
		{"browser with history", "text/html,application/xhtml+xml", history, "text/html", []string{"Waking up", "usually wakes in about 5 seconds", "Acme Cloud", "http-equiv=\"refresh\" content=\"3\"", "#aa5500"}, nil, "3"},
		{"browser no history", "text/html", store.WakeTiming{}, "text/html", []string{"Waking up", "Acme Cloud"}, []string{"usually wakes"}, "3"},
		{"api client json", "application/json", history, "application/json", []string{`"retry_after_seconds":3`, `"app":"web"`}, []string{"<html"}, "3"},
		{"curl default", "*/*", history, "text/plain", []string{"waking up", "Retry in 3s"}, []string{"<html"}, "3"},
		{"no accept header", "", store.WakeTiming{}, "text/plain", []string{"Retry in 3s"}, nil, "3"},
		{"fast app clamps up", "text/plain", store.WakeTiming{ReadyP50Ms: ptr64(200)}, "text/plain", []string{"Retry in 1s"}, nil, "1"},
		{"slow app clamps down", "text/plain", store.WakeTiming{ReadyP50Ms: ptr64(90_000)}, "text/plain", []string{"Retry in 10s"}, nil, "10"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			rec := httptest.NewRecorder()
			rt.writeWakingResponse(rec, req, "web", tc.timing)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
			if got := rec.Header().Get("Retry-After"); got != tc.retryAfter {
				t.Errorf("Retry-After = %q, want %q", got, tc.retryAfter)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tc.contentType) {
				t.Errorf("Content-Type = %q, want %q", got, tc.contentType)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Error("a waking response must never be cached")
			}
			body := rec.Body.String()
			for _, want := range tc.contains {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q:\n%s", want, body)
				}
			}
			for _, bad := range tc.absent {
				if strings.Contains(body, bad) {
					t.Errorf("body must not contain %q:\n%s", bad, body)
				}
			}
		})
	}
}

func TestWakingResponseRejectsUnsafeBrandColor(t *testing.T) {
	rt, _, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	rt.brand = &brand.Brand{Name: "<b>x</b>", PrimaryColor: `red;" onload="x`}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	rt.writeWakingResponse(rec, req, "web", store.WakeTiming{})
	body := rec.Body.String()
	if strings.Contains(body, "onload") || strings.Contains(body, "<b>x</b>") {
		t.Fatalf("brand values must be escaped or dropped:\n%s", body)
	}
}

func TestHoldTimeoutAdaptsToMeasuredWake(t *testing.T) {
	cfg := appsleep.ConfigFromEnv(func(k string) string {
		return map[string]string{appsleep.EnvHoldMin: "10s", appsleep.EnvHoldMax: "60s", appsleep.EnvHoldDefault: "30s", appsleep.EnvHoldHeadroom: "2"}[k]
	})
	tests := []struct {
		name string
		p95  *int64
		want time.Duration
	}{
		{"no history uses the default", nil, 30 * time.Second},
		{"zero p95 uses the default", ptr64(0), 30 * time.Second},
		{"fast app is held to the floor", ptr64(500), 10 * time.Second},
		{"p95 times headroom", ptr64(12_000), 24 * time.Second},
		{"slow app is held to the cap", ptr64(120_000), 60 * time.Second},
	}
	for _, tc := range tests {
		if got := cfg.HoldTimeout(tc.p95); got != tc.want {
			t.Errorf("%s: HoldTimeout = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestWakeHookNeverReadyRecordsFailureAndKeepsAppStarted(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	rt.wakeToken = "tok"
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "fn", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleepIdle(ctx, "fn", 30, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleepHold(ctx, "fn", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceSuspended(ctx, "fn", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleeping(ctx, "fn", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	shortWakeHold(t, 100*time.Millisecond)

	rec := wakeHook(rt, "fn", "/run")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("never ready = %d, want 503", rec.Code)
	}
	if svc, _ := db.GetDesiredService(ctx, "fn"); svc.Suspended {
		t.Error("the start half-succeeded and must stay started so the reconciler can report why")
	}
	events, err := db.ListSleepEvents(ctx, "fn", time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	var woke, failed bool
	for _, e := range events {
		woke = woke || e.Kind == store.SleepEventWoke
		failed = failed || e.Kind == store.SleepEventWakeFailed
	}
	if !woke || !failed {
		t.Errorf("events woke=%v wake_failed=%v, want both", woke, failed)
	}
	timing, err := db.GetWakeTiming(ctx, "fn")
	if err != nil {
		t.Fatal(err)
	}
	if timing.Samples != 0 {
		t.Errorf("a wake that never became ready must not count as a timed wake: %+v", timing)
	}
}

func TestSleepResourceReportsWakeTimingAndIdleLimits(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "wakeweb", Image: "nginx:1", Port: 80, Domains: []string{"web.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleepIdle(ctx, "wakeweb", 30, time.Now()); err != nil {
		t.Fatal(err)
	}
	t.Setenv(appsleep.EnvIdleMin, "10")
	t.Setenv(appsleep.EnvIdleMax, "600")
	t.Setenv(appsleep.EnvIdleDefault, "45")

	res := sleepGet(t, rt, "wakeweb")
	if res.WakeTiming.Samples != 0 || res.WakeTiming.ReadyP50Ms != nil {
		t.Errorf("no history must report no numbers: %+v", res.WakeTiming)
	}
	if res.IdleLimits != (sleepIdleLimits{MinMinutes: 10, MaxMinutes: 600, DefaultMinutes: 45}) {
		t.Errorf("idle limits = %+v", res.IdleLimits)
	}

	for i, ms := range []int64{2000, 4000, 9000} {
		id, err := db.AddSleepEvent(ctx, store.SleepEvent{ServiceName: "wakeweb", Kind: store.SleepEventWoke, At: time.Now().Add(time.Duration(i) * time.Minute), Reason: store.SleepReasonRequest})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.SetSleepEventLatency(ctx, id, ms); err != nil {
			t.Fatal(err)
		}
		if err := db.SetSleepEventReleased(ctx, id, ms+300); err != nil {
			t.Fatal(err)
		}
	}
	res = sleepGet(t, rt, "wakeweb")
	w := res.WakeTiming
	if w.Samples != 3 || w.ReadyLastMs == nil || *w.ReadyLastMs != 9000 || *w.ReadyP50Ms != 4000 || *w.ReadyP95Ms != 9000 || *w.ReleasedLastMs != 9300 {
		t.Errorf("wake timing = %+v", w)
	}

	rec := sleepPut(t, rt.handleSetAppSleep, `{"idle_minutes":5}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "between 10 and 600") {
		t.Errorf("below the env floor = %d %s", rec.Code, rec.Body)
	}
}
