package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/diskguard"
)

func TestDiskResilience_Routes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	serve := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, ""))
		return rec
	}

	if rec := serve(http.MethodGet, "/api/v1/resilience/disk"); rec.Code != http.StatusNotImplemented {
		t.Fatalf("without a guard GET = %d, want 501", rec.Code)
	}

	free := int64(10) << 20
	rt.SetDiskGuard(diskguard.New(t.TempDir(), "", diskguard.Thresholds{LowFree: 100 << 20, CriticalFree: 50 << 20}, func(string) (int64, int64, error) { return free, 1 << 40, nil }, nil))
	rec := serve(http.MethodGet, "/api/v1/resilience/disk")
	var got diskResilienceResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || !got.Refusing || got.Worst != diskguard.LevelCritical {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(http.MethodPost, "/api/v1/resilience/disk/free-space"); rec.Code != http.StatusNotImplemented {
		t.Fatalf("free-space without a janitor = %d, want 501", rec.Code)
	}
}

func TestRefuseLowDisk_Writes507WithCode(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	if rt.refuseLowDisk(rec, diskguard.OpBuild) {
		t.Fatal("refused with no guard configured")
	}

	rt.SetDiskGuard(diskguard.New(t.TempDir(), "", diskguard.Thresholds{LowFree: 100 << 20, CriticalFree: 50 << 20}, func(string) (int64, int64, error) { return 1 << 20, 1 << 40, nil }, nil))
	if !rt.refuseLowDisk(rec, diskguard.OpBuild) || rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("code = %d, want 507", rec.Code)
	}
	var body codedError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != errCodeLowDisk {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
