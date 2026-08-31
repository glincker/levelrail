package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// fakeDockerPinger is a hand-written fake for DockerPinger, the same
// pattern fakeSecretSetter (secrets_test.go) already establishes in
// this package instead of a mocking framework.
type fakeDockerPinger struct {
	err error
}

func (f *fakeDockerPinger) Ping(_ context.Context) error {
	return f.err
}

func TestSystemStatusRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/status", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleSystemStatus_NothingConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got systemStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SecretsConfigured {
		t.Error("SecretsConfigured = true, want false (no WithSecretSetter)")
	}
	if got.TelemetryConfigured {
		t.Error("TelemetryConfigured = true, want false (no WithTelemetryQuerier)")
	}
	if got.AlertsConfigured {
		t.Error("AlertsConfigured = true, want false (no WithAlertRules)")
	}
	if got.DataDirTotalBytes != 0 || got.DataDirFreeBytes != 0 {
		t.Errorf("data dir bytes = (%d, %d), want (0, 0) with no WithDataDir", got.DataDirTotalBytes, got.DataDirFreeBytes)
	}
	if got.DockerConnected {
		t.Error("DockerConnected = true, want false (no WithDockerPinger)")
	}
}

func TestHandleSystemStatus_DockerConnected_Healthy(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDockerPinger(&fakeDockerPinger{}))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got systemStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.DockerConnected {
		t.Error("DockerConnected = false, want true (WithDockerPinger configured and healthy)")
	}
}

func TestHandleSystemStatus_DockerConnected_Erroring(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDockerPinger(&fakeDockerPinger{err: errors.New("daemon unreachable")}))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got systemStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DockerConnected {
		t.Error("DockerConnected = true, want false (WithDockerPinger configured but Ping errors)")
	}
	if got.DockerError != "daemon unreachable" {
		t.Errorf("DockerError = %q, want %q", got.DockerError, "daemon unreachable")
	}
}

func TestHandleSystemStatus_DockerConnected_Healthy_NoError(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDockerPinger(&fakeDockerPinger{}))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/status", ""))

	var got systemStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DockerError != "" {
		t.Errorf("DockerError = %q, want empty when Ping succeeds", got.DockerError)
	}
}

// fakeDockerDiskUsager is a hand-written fake for DockerDiskUsager,
// mirroring fakeDockerPinger just above.
type fakeDockerDiskUsager struct {
	usage docker.DiskUsage
	err   error
}

func (f *fakeDockerDiskUsager) DiskUsage(_ context.Context) (docker.DiskUsage, error) {
	return f.usage, f.err
}

func TestHandleSystemStatus_DockerDiskUsage_Reported(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDockerDiskUsager(&fakeDockerDiskUsager{
		usage: docker.DiskUsage{
			ImagesTotalBytes:           1000,
			ImagesReclaimableBytes:     200,
			ContainersTotalBytes:       50,
			ContainersReclaimableBytes: 10,
			VolumesTotalBytes:          300,
			VolumesReclaimableBytes:    30,
			BuildCacheTotalBytes:       400,
			BuildCacheReclaimableBytes: 40,
		},
	}))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got systemStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DockerDiskUsage == nil {
		t.Fatal("DockerDiskUsage = nil, want populated (WithDockerDiskUsager configured and healthy)")
	}
	if got.DockerDiskUsage.ImagesTotalBytes != 1000 || got.DockerDiskUsage.ImagesReclaimableBytes != 200 {
		t.Errorf("images = %+v, want total=1000 reclaimable=200", got.DockerDiskUsage)
	}
	if got.DockerDiskUsage.BuildCacheReclaimableBytes != 40 {
		t.Errorf("BuildCacheReclaimableBytes = %d, want 40", got.DockerDiskUsage.BuildCacheReclaimableBytes)
	}
}

func TestHandleSystemStatus_DockerDiskUsage_ErroringDegradesToOmitted(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDockerDiskUsager(&fakeDockerDiskUsager{err: errors.New("daemon unreachable")}))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got systemStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DockerDiskUsage != nil {
		t.Errorf("DockerDiskUsage = %+v, want nil: a failed DiskUsage call must degrade to omitted, never a 5xx", got.DockerDiskUsage)
	}
}

func TestHandleSystemStatus_DockerDiskUsage_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithDockerDiskUsager
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/status", ""))

	var got systemStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DockerDiskUsage != nil {
		t.Errorf("DockerDiskUsage = %+v, want nil with no WithDockerDiskUsager configured", got.DockerDiskUsage)
	}
}

func TestHandleSystemStatus_SecretsConfigured(t *testing.T) {
	rt, db := newTestRouterWithSecrets(t, &fakeSecretSetter{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/status", ""))

	var got systemStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.SecretsConfigured {
		t.Error("SecretsConfigured = false, want true")
	}
}

func TestHandleSystemStatus_DataDirConfigured(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDataDir(dir))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got systemStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DataDirTotalBytes <= 0 {
		t.Errorf("DataDirTotalBytes = %d, want > 0", got.DataDirTotalBytes)
	}
	if got.DataDirFreeBytes <= 0 {
		t.Errorf("DataDirFreeBytes = %d, want > 0", got.DataDirFreeBytes)
	}
}
