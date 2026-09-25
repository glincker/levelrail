package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSystemDoctorRoute_RequiresAuth(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)

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

// fakeDoctorOfflineDoer/fakeDoctorOfflineDialContext simulate a host
// with no outbound network access at all: the default every test in
// this file runs under, so no doctor test ever depends on this
// machine's real internet access (or lack of it) to pass.
type fakeDoctorOfflineDoer struct{}

func (fakeDoctorOfflineDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("network disabled in tests")
}

func fakeDoctorOfflineDialContext(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("network disabled in tests")
}

// newDoctorTestRouter wraps newTestRouter with the offline network
// fakes above, so a bare "the rest of this router is unconfigured"
// test doesn't also depend on real network access for its network
// checks.
func newDoctorTestRouter(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	withOfflineDoctorNetwork(rt)
	return rt, db
}

func withOfflineDoctorNetwork(rt *Router) *Router {
	rt.doctorHTTPClient = fakeDoctorOfflineDoer{}
	rt.doctorDialContext = fakeDoctorOfflineDialContext
	return rt
}

func TestHandleSystemDoctor_NothingConfigured(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
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
	if len(got.Checks) != 18 {
		t.Fatalf("len(Checks) = %d, want 18", len(got.Checks))
	}
	for _, code := range []string{"docker", "database", "disk_space", "data_dir_writable", "master_key_rotation", "stale_secrets", "control_plane_backup", "public_ip", "external_reachability_80", "external_reachability_443", "clock_skew"} {
		if c := doctorCheckByCode(t, got.Checks, code); c.Status != doctorStatusUnknown {
			t.Errorf("%s status = %q, want %q (nothing configured / offline)", code, c.Status, doctorStatusUnknown)
		}
	}
	// acme_reachability degrades to warn, not unknown, when offline: ACME
	// is disabled by default (ingress_settings' own seeded row), so an
	// unreachable directory URL is a heads-up, not a real problem.
	if c := doctorCheckByCode(t, got.Checks, "acme_reachability"); c.Status != doctorStatusWarn {
		t.Errorf("acme_reachability status = %q, want %q (offline, but ACME isn't enabled)", c.Status, doctorStatusWarn)
	}
}

func TestHandleSystemDoctor_DockerFailing(t *testing.T) {
	db := openTestDB(t)
	rt := withOfflineDoctorNetwork(NewRouter(nil, testBrand(), db, WithDockerPinger(&fakeDockerPinger{err: errors.New("unreachable")})))
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
	rt := withOfflineDoctorNetwork(NewRouter(nil, testBrand(), db, WithDockerPinger(&fakeDockerPinger{})))
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
	rt := withOfflineDoctorNetwork(NewRouter(nil, testBrand(), db, WithDBPinger(db)))
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
	rt := withOfflineDoctorNetwork(NewRouter(nil, testBrand(), db, WithDataDir(dir)))
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
	rt := withOfflineDoctorNetwork(NewRouter(nil, testBrand(), db, WithDataDir(dir), WithDoctorDiskWarningBytes(1<<62)))
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

// fakeIngressPortOwner is a hand-written fake of IngressPortOwner, the
// same pattern fakeDockerPinger (status_test.go) already uses for a
// single-method optional Router dependency.
type fakeIngressPortOwner struct {
	owned map[int]bool
}

func (f *fakeIngressPortOwner) OwnsPort(port int) bool {
	return f.owned[port]
}

// TestWithIngressPortOwner proves the functional option itself actually
// wires rt.ingressPortOwner, not just the field assignment TestDoctorCheckPort
// exercises directly (newTestRouterWithDockerPruner's own single-option
// pattern, system_prune_test.go).
func TestWithIngressPortOwner(t *testing.T) {
	db := openTestDB(t)
	owner := &fakeIngressPortOwner{owned: map[int]bool{443: true}}
	rt := NewRouter(discardLogger(), testBrand(), db, WithIngressPortOwner(owner))

	if rt.ingressPortOwner == nil {
		t.Fatal("ingressPortOwner = nil, want the wired owner")
	}
	if !rt.ingressPortOwner.OwnsPort(443) {
		t.Error("ingressPortOwner.OwnsPort(443) = false, want true")
	}
}

// TestWithDoctorIngressPorts proves GET /api/v1/system/doctor's port
// checks follow a configured non-default port instead of the hardcoded
// 80/443, the shape an instance started with APP_INGRESS_HTTP_ADDR/
// APP_INGRESS_HTTPS_ADDR set to something else needs to stay accurate.
func TestWithDoctorIngressPorts(t *testing.T) {
	db := openTestDB(t)
	owner := &fakeIngressPortOwner{owned: map[int]bool{8080: true, 8443: true}}
	rt := withOfflineDoctorNetwork(NewRouter(discardLogger(), testBrand(), db,
		WithIngressPortOwner(owner),
		WithDoctorIngressPorts(8080, 8443),
	))
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/doctor", ""))

	var got systemDoctorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c := doctorCheckByCode(t, got.Checks, "port_8080"); c.Status != doctorStatusOK {
		t.Errorf("port_8080 check = %+v, want status=ok", c)
	}
	if c := doctorCheckByCode(t, got.Checks, "port_8443"); c.Status != doctorStatusOK {
		t.Errorf("port_8443 check = %+v, want status=ok", c)
	}
	for _, code := range []string{"port_80", "port_443"} {
		for _, c := range got.Checks {
			if c.Code == code {
				t.Errorf("found stale %s check, want only the configured 8080/8443 codes: %+v", code, c)
			}
		}
	}
}

// TestWithDoctorIngressPorts_ZeroKeepsDefaults proves an unset
// doctorHTTPPort/doctorHTTPSPort (the default, WithDoctorIngressPorts
// never called) still produces the original port_80/port_443 checks,
// the same "0 means unset" shape WithDoctorDiskWarningBytes already has.
func TestWithDoctorIngressPorts_ZeroKeepsDefaults(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	if rt.doctorHTTPPort != 0 || rt.doctorHTTPSPort != 0 {
		t.Fatalf("doctorHTTPPort/doctorHTTPSPort = %d/%d, want 0/0 (unset)", rt.doctorHTTPPort, rt.doctorHTTPSPort)
	}
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/doctor", ""))

	var got systemDoctorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	doctorCheckByCode(t, got.Checks, "port_80")
	doctorCheckByCode(t, got.Checks, "port_443")
}

func TestDoctorCheckPort(t *testing.T) {
	// listenOnPort starts a listener on an ephemeral port and returns it
	// still bound, for a test case that needs the port genuinely in use.
	listenOnPort := func(t *testing.T) (int, func()) {
		t.Helper()
		ln, err := net.Listen("tcp", ":0") //nolint:gosec // must match doctorCheckPort's own all-interfaces bind, or macOS lets both coexist and the test can't force EADDRINUSE
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		return ln.Addr().(*net.TCPAddr).Port, func() { _ = ln.Close() }
	}
	// freePort returns a port number that was bound and released
	// immediately, so it's very likely free for the check itself.
	freePort := func(t *testing.T) int {
		t.Helper()
		ln, err := net.Listen("tcp", ":0") //nolint:gosec // ephemeral port probe only, immediately released below
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		return port
	}

	tests := []struct {
		name         string
		usePort      func(t *testing.T) (int, func())
		ownsIt       bool // whether the fake ingress port owner claims the port under test
		noOwner      bool // no IngressPortOwner wired at all
		wantStatus   string
		wantContains string
	}{
		{
			name:       "genuinely free",
			usePort:    func(t *testing.T) (int, func()) { return freePort(t), func() {} },
			noOwner:    true,
			wantStatus: doctorStatusOK,
		},
		{
			name:         "held by this control plane's own ingress",
			usePort:      listenOnPort,
			ownsIt:       true,
			wantStatus:   doctorStatusOK,
			wantContains: "this control plane's own ingress",
		},
		{
			name:         "held by something else, ingress not using it",
			usePort:      listenOnPort,
			ownsIt:       false,
			wantStatus:   doctorStatusFail,
			wantContains: "another process",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, release := tt.usePort(t)
			defer release()

			rt, _ := newDoctorTestRouter(t)
			if !tt.noOwner {
				rt.ingressPortOwner = &fakeIngressPortOwner{owned: map[int]bool{port: tt.ownsIt}}
			}

			c := rt.doctorCheckPort(port)
			if c.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", c.Status, tt.wantStatus)
			}
			if tt.wantContains != "" && !strings.Contains(c.Message, tt.wantContains) {
				t.Errorf("message = %q, want it to contain %q", c.Message, tt.wantContains)
			}
		})
	}
}

func TestDoctorCheckMasterKeyRotation_NeverRotated(t *testing.T) {
	rt, _ := newTestRouterWithMasterKeyRotator(t, &fakeMasterKeyRotator{historyOK: false}, "")
	c := rt.doctorCheckMasterKeyRotation(context.Background())
	if c.Status != doctorStatusOK {
		t.Errorf("status = %q, want %q (never rotated is informational, not a warning)", c.Status, doctorStatusOK)
	}
}

func TestDoctorCheckMasterKeyRotation_RecentlyRotated(t *testing.T) {
	rt, _ := newTestRouterWithMasterKeyRotator(t, &fakeMasterKeyRotator{historyOK: true, historyAt: time.Now().Add(-time.Hour)}, "")
	c := rt.doctorCheckMasterKeyRotation(context.Background())
	if c.Status != doctorStatusOK {
		t.Errorf("status = %q, want %q (rotated an hour ago, well under the default warn age)", c.Status, doctorStatusOK)
	}
}

func TestDoctorCheckMasterKeyRotation_StaleWarns(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithMasterKeyRotation(&fakeMasterKeyRotator{historyOK: true, historyAt: time.Now().Add(-48 * time.Hour)}, ""),
		WithDoctorMasterKeyRotationWarnAge(24*time.Hour),
	)
	c := rt.doctorCheckMasterKeyRotation(context.Background())
	if c.Status != doctorStatusWarn {
		t.Errorf("status = %q, want %q (48h old, past the 24h configured threshold)", c.Status, doctorStatusWarn)
	}
}

func TestDoctorCheckMasterKeyRotation_NotConfigured(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	c := rt.doctorCheckMasterKeyRotation(context.Background())
	if c.Status != doctorStatusUnknown {
		t.Errorf("status = %q, want %q (no master key configured)", c.Status, doctorStatusUnknown)
	}
}

func TestDoctorCheckStaleSecrets_NotConfigured(t *testing.T) {
	rt, _ := newDoctorTestRouter(t) // no WithStaleSecretCounter
	c := rt.doctorCheckStaleSecrets(context.Background())
	if c.Status != doctorStatusUnknown {
		t.Errorf("status = %q, want %q (no stale secret counter configured)", c.Status, doctorStatusUnknown)
	}
}

func TestDoctorCheckStaleSecrets_NoneStale(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithStaleSecretCounter(db))
	if err := db.SaveServiceDEK(context.Background(), "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(context.Background(), "web", "API_KEY", []byte("ct")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	c := rt.doctorCheckStaleSecrets(context.Background())
	if c.Status != doctorStatusOK {
		t.Errorf("status = %q, want %q (secret just set, well under the default 90 day threshold)", c.Status, doctorStatusOK)
	}
}

func TestDoctorCheckStaleSecrets_WarnsWhenOverThreshold(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithStaleSecretCounter(db),
		WithSecretRotationWarnAge(24*time.Hour),
	)
	if err := db.SaveServiceDEK(context.Background(), "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(context.Background(), "web", "API_KEY", []byte("ct")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		UPDATE service_secret_values SET updated_at = ? WHERE service_name = ? AND env_key = ?
	`, time.Now().Add(-48*time.Hour).UTC().Format("2006-01-02T15:04:05.000Z"), "web", "API_KEY"); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	c := rt.doctorCheckStaleSecrets(context.Background())
	if c.Status != doctorStatusWarn {
		t.Errorf("status = %q, want %q (48h old, past the 24h configured threshold)", c.Status, doctorStatusWarn)
	}
	if !strings.Contains(c.Message, "1 secret") {
		t.Errorf("message = %q, want it to mention the 1 stale secret found", c.Message)
	}
}

type fakeCPBackups struct {
	list []cpbackup.Info
	err  error
}

func (f fakeCPBackups) Create(context.Context) (cpbackup.Info, error) { return cpbackup.Info{}, nil }
func (f fakeCPBackups) List() ([]cpbackup.Info, error)                { return f.list, f.err }
func (f fakeCPBackups) Open(string) (*os.File, cpbackup.Info, error) {
	return nil, cpbackup.Info{}, cpbackup.ErrNotFound
}
func (f fakeCPBackups) Delete(string) error { return nil }
func (f fakeCPBackups) Verify(context.Context, string) (cpbackup.VerifyResult, error) {
	return cpbackup.VerifyResult{}, nil
}

func TestDoctorCheckControlPlaneBackup(t *testing.T) {
	recent := cpbackup.Info{Name: "a", CreatedAt: time.Now().Add(-2 * time.Hour)}
	old := cpbackup.Info{Name: "b", CreatedAt: time.Now().Add(-4 * 24 * time.Hour)}
	tests := []struct {
		name    string
		mgr     ControlPlaneBackupManager
		off     bool
		want    string
		wantFix bool
	}{
		{"not configured", nil, false, doctorStatusUnknown, false},
		{"disabled", fakeCPBackups{}, true, doctorStatusUnknown, false},
		{"list error", fakeCPBackups{err: errors.New("boom")}, false, doctorStatusUnknown, false},
		{"none yet", fakeCPBackups{}, false, doctorStatusOK, false},
		{"recent", fakeCPBackups{list: []cpbackup.Info{recent}}, false, doctorStatusOK, false},
		{"stale", fakeCPBackups{list: []cpbackup.Info{old}}, false, doctorStatusWarn, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{cpBackups: tc.mgr, cpBackupScheduleOff: tc.off}
			c := rt.doctorCheckControlPlaneBackup()
			if c.Status != tc.want {
				t.Errorf("status = %q, want %q (%s)", c.Status, tc.want, c.Message)
			}
			if (c.Fix != "" && c.DocsPath != "") != tc.wantFix {
				t.Errorf("fix/docs = %q/%q, want present=%v", c.Fix, c.DocsPath, tc.wantFix)
			}
		})
	}
}
