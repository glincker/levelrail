package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
)

// testSupplyChainSPDX is a minimal valid SPDX SBOM, the same shape
// internal/api/supplychain_test.go's testSPDX already uses.
const testSupplyChainSPDX = `{"spdxVersion":"SPDX-2.3","packages":[{"SPDXID":"p1","name":"musl","versionInfo":"1.2","licenseConcluded":"MIT","externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:apk/alpine/musl@1.2"}]},{"SPDXID":"p2","name":"busybox","versionInfo":"1.36"}]}`

// testSupplyChainTrivyReport is a `trivy sbom --format json` report with
// one fixable critical finding and one unrelated low one, matching
// internal/supplychain/scanparse.go's ParseTrivy input shape.
const testSupplyChainTrivyReport = `{"SchemaVersion":2,"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1","PkgName":"musl","InstalledVersion":"1.2","FixedVersion":"1.3","Severity":"CRITICAL","Title":"bad"},{"VulnerabilityID":"CVE-2","PkgName":"busybox","InstalledVersion":"1.36","Severity":"LOW"}]}]}`

// scanStubRunner fakes the scanner container supplychain.Service would
// otherwise run via Docker. It satisfies supplychain.Runner, the same
// seam internal/supplychain/service_test.go's fakeRunner and
// internal/api/supplychain_test.go's scRunner already use at unit and
// handler level, reused here so the gate is proven over a real, running
// HTTP server instead of httptest.NewRecorder or a direct Service call.
type scanStubRunner struct {
	mu   sync.Mutex
	runs int
}

func (r *scanStubRunner) EnsureImageID(context.Context, string) (string, bool, error) {
	return "sha256:stub", false, nil
}

func (r *scanStubRunner) RunOneShot(context.Context, docker.OneShotSpec) (docker.OneShotResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs++
	return docker.OneShotResult{Stdout: []byte(testSupplyChainTrivyReport)}, nil
}

func (r *scanStubRunner) ListContainersByLabel(context.Context, string) ([]docker.LabeledContainer, error) {
	return nil, nil
}

func (r *scanStubRunner) Remove(context.Context, string, bool) error { return nil }

func (r *scanStubRunner) scanCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runs
}

// scanStubResolver mirrors cmd/levelrail/supplychain.go's unexported
// supplyChainResolver (not importable from package main), backed by the
// same real store.DB this test's router and service share.
type scanStubResolver struct {
	db  *store.DB
	sql *supplychain.SQLStore
}

func (r scanStubResolver) AppExists(ctx context.Context, app string) (bool, error) {
	_, err := r.db.GetDesiredService(ctx, app)
	if errors.Is(err, store.ErrServiceNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (r scanStubResolver) AttemptExists(ctx context.Context, id string) (bool, error) {
	return r.sql.AttemptExists(ctx, id)
}

// scSettingsResponse and scVulnResponse mirror internal/api's unexported
// supplyChainSettingsResource and vulnResource wire shapes: this package
// decodes the real response body into its own copy, the same
// package-boundary reasoning auth_lifecycle_test.go's authResponse
// already documents for this test package.
type scSettingsResponse struct {
	ScanEnabled    bool   `json:"scan_enabled"`
	ScanGate       string `json:"scan_gate"`
	OverrideArmed  bool   `json:"override_armed"`
	OverrideReason string `json:"override_reason,omitempty"`
}

type scVulnResponse struct {
	Scan struct {
		Status string `json:"status"`
		Counts *struct {
			Critical int `json:"critical"`
			Low      int `json:"low"`
		} `json:"counts"`
		Fixable int `json:"fixable"`
		Top     []struct {
			ID string `json:"id"`
		} `json:"top"`
	} `json:"scan"`
	Gate *struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	} `json:"gate"`
}

// testSupplyChainApp is the one app this test's whole scenario runs
// against, across every build and every real HTTP call.
const testSupplyChainApp = "sc-e2e-app"

// seedSupplyChainBuild records a succeeded deploy attempt and runs it
// through svc.AfterBuild, the real post-build hook internal/deploy calls
// once a build finishes. A full Dockerfile build adds nothing here: the
// behavior under test is what happens to a known SBOM, the same
// "construct the earlier layer's output directly" choice
// test/e2e/env_test.go already makes for store.DesiredService.
func seedSupplyChainBuild(t *testing.T, db *store.DB, svc *supplychain.Service, attemptID string) error {
	t.Helper()
	const app = testSupplyChainApp
	ctx := context.Background()
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: attemptID, ServiceName: app, Image: app + ":1",
		Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed deploy attempt %q: %v", attemptID, err)
	}
	return svc.AfterBuild(ctx, app, attemptID, build.Attestations{SBOM: []byte(testSupplyChainSPDX)})
}

// decodeSC decodes a real, successful response body from the server
// into v, failing the test with the raw body on any mismatch.
func decodeSC(t *testing.T, status int, body []byte, v any) {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

// TestSupplyChain_Live_GateBlocksAndOverrideThroughRealAPI closes
// docs/feature-status.md's "no e2e" gap: enabling the feature, running
// a scan, and the gate's block/override decision all go over a real
// HTTP server backed by a real store.DB and supplychain.Service. Only
// the scanner's own Docker container is faked, via scanStubRunner, the
// same seam this repo's unit and handler tests already use.
//
// Does not exercise a real Dockerfile build or a real trivy binary.
func TestSupplyChain_Live_GateBlocksAndOverrideThroughRealAPI(t *testing.T) {
	db := openLiveStore(t)
	const app = testSupplyChainApp
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: app, Image: "levelrail/sc-e2e:1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	runner := &scanStubRunner{}
	sqlStore := supplychain.NewSQLStore(db)
	cfg := supplychain.ConfigFromEnv(func(string) (string, bool) { return "", false }, nil)
	svc := supplychain.New(cfg, supplychain.Deps{
		Store: sqlStore, Resolver: scanStubResolver{db: db, sql: sqlStore}, Runner: runner, DataDir: t.TempDir(), Namespace: "lr",
	})

	const adminPassword = "sc-e2e-admin-password-1" //nolint:gosec // fake fixture, this test's own login
	if err := api.BootstrapAdmin(context.Background(), db, "admin", adminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	b := &brand.Brand{Name: "Test Platform", BinaryName: "testplatform"}
	router := api.NewRouter(discardTestLogger(), b, db)
	router.SetSupplyChain(svc)
	server := newE2ETestServer(t, router)
	client := newAuthClient(t, server)

	status, body := requestJSON(t, client, http.MethodPost, server.URL+"/api/v1/auth/login", `{"username":"admin","password":"`+adminPassword+`"}`)
	if status != http.StatusOK {
		t.Fatalf("login: status = %d, body = %s", status, body)
	}

	base := server.URL + "/api/v1/apps/" + app

	// Step 1: the feature is off by default and the first build only
	// stores the SBOM, over real HTTP, proving docs/feature-status.md's
	// "off by default" claim rather than assuming it.
	status, body = requestJSON(t, client, http.MethodGet, base+"/supply-chain", "")
	var settings scSettingsResponse
	decodeSC(t, status, body, &settings)
	if settings.ScanEnabled || settings.ScanGate != "off" {
		t.Fatalf("default settings = %+v, want disabled and off", settings)
	}
	if err := seedSupplyChainBuild(t, db, svc, "da_1"); err != nil {
		t.Fatalf("seedSupplyChainBuild(da_1) error = %v, want nil (scanning is off)", err)
	}
	if n := runner.scanCount(); n != 0 {
		t.Fatalf("scanner ran %d times while disabled, want 0", n)
	}

	// Step 2: enable scanning over real HTTP, then trigger the scan
	// itself over real HTTP, and see a real (faked-container) critical
	// finding come back through the vulnerabilities endpoint.
	status, body = requestJSON(t, client, http.MethodPut, base+"/supply-chain", `{"scan_enabled":true}`)
	decodeSC(t, status, body, &settings)
	if !settings.ScanEnabled || settings.ScanGate != "off" {
		t.Fatalf("after enabling = %+v", settings)
	}
	status, body = requestJSON(t, client, http.MethodPost, base+"/deployments/da_1/scan", "")
	var vulns scVulnResponse
	decodeSC(t, status, body, &vulns)
	if vulns.Scan.Status != "ok" || vulns.Scan.Counts == nil || vulns.Scan.Counts.Critical != 1 || vulns.Scan.Fixable != 1 ||
		len(vulns.Scan.Top) == 0 || vulns.Scan.Top[0].ID != "CVE-1" {
		t.Fatalf("scan result over HTTP = %+v", vulns)
	}
	if n := runner.scanCount(); n != 1 {
		t.Fatalf("scanner ran %d times, want exactly 1 real HTTP-triggered scan", n)
	}

	// Step 3: turn the gate on. The next build with the same critical
	// finding must be blocked, and that block must be visible to a
	// real HTTP caller reading the deployment's vulnerabilities, not
	// just to this test's own err return from AfterBuild.
	status, body = requestJSON(t, client, http.MethodPut, base+"/supply-chain", `{"scan_gate":"block_on_critical"}`)
	decodeSC(t, status, body, &settings)
	if settings.ScanGate != "block_on_critical" {
		t.Fatalf("after setting gate = %+v", settings)
	}
	var blocked *supplychain.BlockedError
	if err := seedSupplyChainBuild(t, db, svc, "da_2"); !errors.As(err, &blocked) {
		t.Fatalf("seedSupplyChainBuild(da_2) error = %v, want a BlockedError", err)
	}
	status, body = requestJSON(t, client, http.MethodGet, base+"/deployments/da_2/vulnerabilities", "")
	decodeSC(t, status, body, &vulns)
	if vulns.Gate == nil || vulns.Gate.Action != "block" {
		t.Fatalf("blocked release's own gate, read back over HTTP = %+v", vulns.Gate)
	}

	// Step 4: arm a one-shot override over real HTTP. The next build
	// must go through, and the override must be spent: the build after
	// that is blocked again.
	status, body = requestJSON(t, client, http.MethodPost, base+"/supply-chain/override", `{"reason":"customer outage, fix ships next release"}`)
	decodeSC(t, status, body, &settings)
	if !settings.OverrideArmed {
		t.Fatalf("after arming override = %+v", settings)
	}
	if err := seedSupplyChainBuild(t, db, svc, "da_3"); err != nil {
		t.Fatalf("seedSupplyChainBuild(da_3) error = %v, want nil (override lets this one through)", err)
	}
	status, body = requestJSON(t, client, http.MethodGet, base+"/deployments/da_3/vulnerabilities", "")
	decodeSC(t, status, body, &vulns)
	if vulns.Gate == nil || vulns.Gate.Action != "override" {
		t.Fatalf("overridden release's own gate, read back over HTTP = %+v", vulns.Gate)
	}

	if err := seedSupplyChainBuild(t, db, svc, "da_4"); !errors.As(err, &blocked) {
		t.Fatalf("seedSupplyChainBuild(da_4) error = %v, want blocked again (the override was one-shot)", err)
	}
	status, body = requestJSON(t, client, http.MethodGet, base+"/deployments/da_4/vulnerabilities", "")
	decodeSC(t, status, body, &vulns)
	if vulns.Gate == nil || vulns.Gate.Action != "block" {
		t.Fatalf("post-override release's own gate, read back over HTTP = %+v", vulns.Gate)
	}
}
