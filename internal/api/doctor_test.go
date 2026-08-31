package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemDoctorRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/doctor", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func doctorCheckByCode(t *testing.T, checks []doctorCheckResource, code string) doctorCheckResource {
	t.Helper()
	for _, c := range checks {
		if c.Code == code {
			return c
		}
	}
	t.Fatalf("no check with code %q in %+v", code, checks)
	return doctorCheckResource{}
}

func TestHandleSystemDoctor_NothingConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/doctor", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got systemDoctorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Checks) != 6 {
		t.Fatalf("len(Checks) = %d, want 6", len(got.Checks))
	}
	for _, code := range []string{"docker", "database", "disk_space", "data_dir_writable"} {
		if c := doctorCheckByCode(t, got.Checks, code); c.Status != doctorStatusUnknown {
			t.Errorf("%s status = %q, want %q (nothing configured)", code, c.Status, doctorStatusUnknown)
		}
	}
}

func TestHandleSystemDoctor_DockerFailing(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDockerPinger(&fakeDockerPinger{err: errors.New("unreachable")}))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/doctor", ""))

	var got systemDoctorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OK {
		t.Error("OK = true, want false (docker check failing)")
	}
	if c := doctorCheckByCode(t, got.Checks, "docker"); c.Status != doctorStatusFail || c.Message != "unreachable" {
		t.Errorf("docker check = %+v, want status=fail message=unreachable", c)
	}
}

func TestHandleSystemDoctor_DockerHealthy(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDockerPinger(&fakeDockerPinger{}))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/doctor", ""))

	var got systemDoctorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c := doctorCheckByCode(t, got.Checks, "docker"); c.Status != doctorStatusOK {
		t.Errorf("docker check = %+v, want status=ok", c)
	}
}

func TestHandleSystemDoctor_DatabaseHealthy(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDBPinger(db))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/doctor", ""))

	var got systemDoctorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c := doctorCheckByCode(t, got.Checks, "database"); c.Status != doctorStatusOK {
		t.Errorf("database check = %+v, want status=ok", c)
	}
}

func TestHandleSystemDoctor_DataDirConfigured(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDataDir(dir))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/doctor", ""))

	var got systemDoctorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c := doctorCheckByCode(t, got.Checks, "data_dir_writable"); c.Status != doctorStatusOK {
		t.Errorf("data_dir_writable check = %+v, want status=ok", c)
	}
	if c := doctorCheckByCode(t, got.Checks, "disk_space"); c.Status == doctorStatusFail || c.Status == doctorStatusUnknown {
		t.Errorf("disk_space check = %+v, want ok or warn on a real, configured directory", c)
	}
}

func TestHandleSystemDoctor_DiskWarningThreshold(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithDataDir(dir), WithDoctorDiskWarningBytes(1<<62))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/doctor", ""))

	var got systemDoctorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c := doctorCheckByCode(t, got.Checks, "disk_space"); c.Status != doctorStatusWarn {
		t.Errorf("disk_space check = %+v, want status=warn with an unreachably high threshold", c)
	}
}

func TestDoctorCheckPort_AddrInUse(t *testing.T) {
	ln, err := net.Listen("tcp", ":0") //nolint:gosec // must match doctorCheckPort's own all-interfaces bind, or macOS lets both coexist and the test can't force EADDRINUSE
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	c := doctorCheckPort(port)
	if c.Status != doctorStatusFail {
		t.Errorf("status = %q, want %q (port already bound)", c.Status, doctorStatusFail)
	}
}

func TestDoctorCheckPort_Available(t *testing.T) {
	ln, err := net.Listen("tcp", ":0") //nolint:gosec // ephemeral port probe only, immediately released below
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	c := doctorCheckPort(port)
	if c.Status != doctorStatusOK {
		t.Errorf("status = %q, want %q (freshly released port)", c.Status, doctorStatusOK)
	}
}
