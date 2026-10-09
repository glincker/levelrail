package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func sleepPut(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/sleep", strings.NewReader(body))
	req.SetPathValue("name", "web")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestAppSleepSettingAndWake(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	rt.wakeToken = "tok"
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}

	if rec := sleepPut(t, rt.handleSetAppSleep, `{"idle_minutes":2}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("2 minutes = %d, want 400", rec.Code)
	}
	if rec := sleepPut(t, rt.handleSetAppSleep, `{"idle_minutes":30}`); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}

	if err := db.UpdateServiceSuspended(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleeping(ctx, "web", true, time.Now()); err != nil {
		t.Fatal(err)
	}

	wake := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, ingress.WakePath, nil)
		req.Header.Set(ingress.WakeTokenHeader, token)
		req.Header.Set(ingress.WakeAppHeader, "web")
		rec := httptest.NewRecorder()
		rt.handleWakeHook(rec, req)
		return rec.Code
	}
	if got := wake("wrong"); got != http.StatusNotFound {
		t.Fatalf("bad token = %d, want 404", got)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); !svc.Suspended {
		t.Fatal("a request with the wrong token woke the app")
	}
	if got := wake("tok"); got != http.StatusNoContent {
		t.Fatalf("wake = %d, want 204", got)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if svc.Suspended {
		t.Fatal("app still stopped after wake")
	}
	if a, _ := db.GetAppSleep(ctx, "web"); a.Sleeping {
		t.Error("sleeping flag survived the wake")
	}
	if got := wake("tok"); got != http.StatusNoContent {
		t.Errorf("repeat wake = %d, want 204", got)
	}

	if rec := sleepPut(t, rt.handleSetAppSleep, `{"idle_minutes":0}`); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d", rec.Code)
	}
	if a, _ := db.GetAppSleep(ctx, "web"); a.IdleMinutes != 0 {
		t.Errorf("idle minutes = %d after disable", a.IdleMinutes)
	}
}

func TestAppSleepDisableWakesSleepingApp(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleepIdle(ctx, "web", 30, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceSuspended(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleeping(ctx, "web", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if rec := sleepPut(t, rt.handleSetAppSleep, `{"idle_minutes":0}`); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d", rec.Code)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); svc.Suspended {
		t.Error("turning sleep off must not leave the app stopped")
	}
}

func TestWakeHookDisabledWithoutToken(t *testing.T) {
	rt, _, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	req := httptest.NewRequest(http.MethodGet, ingress.WakePath, nil)
	rec := httptest.NewRecorder()
	rt.handleWakeHook(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no token configured = %d, want 404", rec.Code)
	}
}

func wakeHook(rt *Router, app, uri string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, ingress.WakePath, nil)
	req.Host = "fn.example.com"
	req.Header.Set(ingress.WakeTokenHeader, "tok")
	req.Header.Set(ingress.WakeAppHeader, app)
	req.Header.Set(ingress.WakeURIHeader, uri)
	rec := httptest.NewRecorder()
	rt.handleWakeHook(rec, req)
	return rec
}

func TestWakeHookHoldsRequestsInFunctionMode(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	rt.wakeToken = "tok"
	ctx := context.Background()
	for _, name := range []string{"fn", "plain"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "nginx:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		if err := db.SetAppSleepIdle(ctx, name, 30, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateServiceSuspended(ctx, name, true); err != nil {
			t.Fatal(err)
		}
		if err := db.SetAppSleeping(ctx, name, true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetAppSleepHold(ctx, "fn", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	ready := []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue}}
	// A Ready from before the app slept must not end the wait.
	if err := db.UpsertConditions(ctx, "application/fn", ready); err != nil {
		t.Fatal(err)
	}
	wakeHoldTimeout, wakeHoldPoll, wakeIngressSettle = 5*time.Second, 20*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() {
		wakeHoldTimeout, wakeHoldPoll, wakeIngressSettle = 30*time.Second, 250*time.Millisecond, 200*time.Millisecond
	})
	time.Sleep(20 * time.Millisecond)
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = db.UpsertConditions(ctx, "application/fn", ready)
		time.Sleep(200 * time.Millisecond)
		_ = db.UpsertConditions(ctx, "ingress", ready)
	}()

	started := time.Now()
	rec := wakeHook(rt, "fn", "/run?x=1")
	if waited := time.Since(started); waited < 450*time.Millisecond {
		t.Fatalf("hook returned after %s, before the app and ingress reported Ready again", waited)
	}
	if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != "https://fn.example.com/run?x=1" {
		t.Fatalf("function mode = %d Location %q, want 307 to the original URL", rec.Code, rec.Header().Get("Location"))
	}
	if svc, _ := db.GetDesiredService(ctx, "fn"); svc.Suspended {
		t.Error("the app was not woken")
	}
	if rec := wakeHook(rt, "plain", "/run"); rec.Code != http.StatusNoContent {
		t.Errorf("without hold = %d, want 204", rec.Code)
	}
	if rec := wakeHook(rt, "fn", "//evil.example/x"); rec.Code == http.StatusTemporaryRedirect {
		t.Error("a protocol-relative path must never become a redirect")
	}
}

func TestWakeHookHoldTimesOutWithoutReady(t *testing.T) {
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
	wakeHoldTimeout, wakeHoldPoll, wakeIngressSettle = 150*time.Millisecond, 20*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() {
		wakeHoldTimeout, wakeHoldPoll, wakeIngressSettle = 30*time.Second, 250*time.Millisecond, 200*time.Millisecond
	})

	if rec := wakeHook(rt, "fn", "/run"); rec.Code != http.StatusNoContent {
		t.Fatalf("never ready = %d, want 204 (the waking-up page), not a redirect loop", rec.Code)
	}
}
