package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSystemRestart(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method string) (*httptest.ResponseRecorder, systemRestartResource) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/system/restart", ""))
		var res systemRestartResource
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		return rec, res
	}

	rec, res := do(http.MethodPost)
	if rec.Code != http.StatusConflict || res.Supported || res.Hint == "" {
		t.Fatalf("unsupervised POST = %d %+v", rec.Code, res)
	}

	var fired atomic.Int32
	rt.SetRestarter(Restarter{Supervised: true, Trigger: func() { fired.Add(1) }})
	_, before := do(http.MethodGet)
	if !before.Supported || before.BootID == "" {
		t.Fatalf("GET = %+v", before)
	}
	for i := 0; i < 2; i++ {
		rec, res = do(http.MethodPost)
		if rec.Code != http.StatusAccepted || !res.Pending || res.BootID != before.BootID {
			t.Fatalf("POST #%d = %d %+v", i, rec.Code, res)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for fired.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := fired.Load(); got != 1 {
		t.Fatalf("trigger fired %d times, want exactly 1", got)
	}
}
