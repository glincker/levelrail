package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestFunctionSettingsAndConcurrency(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	rt.wakeToken = "tok"
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "fn", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	call := func(h http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/functions/fn/settings", strings.NewReader(body))
		req.SetPathValue("name", "fn")
		rec := httptest.NewRecorder()
		h(rec, req)
		return rec
	}
	if rec := call(rt.handleGetFunctionSettings, http.MethodGet, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("plain app = %d, want 404", rec.Code)
	}
	if err := db.SetAppSleepIdle(ctx, "fn", 10, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleepHold(ctx, "fn", true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if rec := call(rt.handlePutFunctionSettings, http.MethodPut, `{"max_concurrency":-1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative = %d, want 400", rec.Code)
	}
	rec := call(rt.handlePutFunctionSettings, http.MethodPut, `{"request_timeout_seconds":20,"max_concurrency":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	var got functionSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.EffectiveTimeoutSeconds != 20 || got.EffectiveConcurrency != 1 || got.IdleMinutes != 10 {
		t.Fatalf("resource = %+v", got)
	}

	release, ok := rt.functionLimiter.Acquire("fn", 1)
	if !ok {
		t.Fatal("slot refused")
	}
	req := httptest.NewRequest(http.MethodGet, ingress.WakePath, nil)
	req.Header.Set(ingress.WakeTokenHeader, "tok")
	req.Header.Set(ingress.WakeAppHeader, "fn")
	w := httptest.NewRecorder()
	rt.handleWakeHook(w, req)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("at limit = %d, retry-after %q", w.Code, w.Header().Get("Retry-After"))
	}
	release()
}
