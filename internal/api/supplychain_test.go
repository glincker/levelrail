package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
)

const testSPDX = `{"spdxVersion":"SPDX-2.3","packages":[{"SPDXID":"p1","name":"musl","versionInfo":"1.2","licenseConcluded":"MIT","externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:apk/alpine/musl@1.2"}]},{"SPDXID":"p2","name":"busybox","versionInfo":"1.36"}]}`

const testTrivy = `{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1","PkgName":"musl","InstalledVersion":"1.2","FixedVersion":"1.3","Severity":"CRITICAL","Title":"bad"},{"VulnerabilityID":"CVE-2","PkgName":"busybox","InstalledVersion":"1.36","Severity":"LOW"}]}]}`

type scRunner struct {
	mu   sync.Mutex
	runs int
}

func (r *scRunner) EnsureImageID(context.Context, string) (string, bool, error) {
	return "sha256:x", false, nil
}

func (r *scRunner) RunOneShot(context.Context, docker.OneShotSpec) (docker.OneShotResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs++
	return docker.OneShotResult{Stdout: []byte(testTrivy)}, nil
}

func (r *scRunner) ListContainersByLabel(context.Context, string) ([]docker.LabeledContainer, error) {
	return nil, nil
}
func (r *scRunner) Remove(context.Context, string, bool) error { return nil }

type scResolver struct{}

func (scResolver) AppExists(context.Context, string) (bool, error)     { return true, nil }
func (scResolver) AttemptExists(context.Context, string) (bool, error) { return true, nil }

func newSupplyChainTestRouter(t *testing.T) (*Router, *store.DB, *supplychain.Service, *scRunner) {
	t.Helper()
	rt, db := newTestRouter(t)
	runner := &scRunner{}
	cfg := supplychain.ConfigFromEnv(func(string) (string, bool) { return "", false }, nil)
	svc := supplychain.New(cfg, supplychain.Deps{
		Store: supplychain.NewSQLStore(db), Resolver: scResolver{}, Runner: runner, DataDir: t.TempDir(), Namespace: "lr",
	})
	rt.SetSupplyChain(svc)
	return rt, db, svc, runner
}

func seedSupplyChainAttempt(t *testing.T, db *store.DB, svc *supplychain.Service, app, id string, started time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: id, ServiceName: app, Image: app + ":1", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: started}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AfterBuild(ctx, app, id, build.Attestations{SBOM: []byte(testSPDX), Provenance: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
}

func scCall(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	return rec
}

func TestSupplyChainRoutes_RequireAuth(t *testing.T) {
	rt, _, _, _ := newSupplyChainTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/supply-chain"},
		{http.MethodPut, "/api/v1/apps/web/supply-chain"},
		{http.MethodPost, "/api/v1/apps/web/supply-chain/override"},
		{http.MethodGet, "/api/v1/apps/web/deployments/dep_a/sbom"},
		{http.MethodGet, "/api/v1/apps/web/deployments/dep_a/vulnerabilities"},
		{http.MethodPost, "/api/v1/apps/web/deployments/dep_a/scan"},
	})
}

func TestSupplyChainRoutes_ReadOnlyTokenCannotMutate(t *testing.T) {
	rt, db, _, _ := newSupplyChainTestRouter(t)
	assertProviderRoutesForbiddenForAbilities(t, rt, db, "tok", "plain-secret", []string{AbilityRead}, []providerRouteCase{
		{method: http.MethodPut, path: "/api/v1/apps/web/supply-chain", body: `{"scan_enabled":true}`},
		{method: http.MethodPost, path: "/api/v1/apps/web/supply-chain/override", body: `{"reason":"x"}`},
		{method: http.MethodPost, path: "/api/v1/apps/web/deployments/dep_a/scan"},
	})
}

func TestSupplyChain_NotConfiguredIs501(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	if rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/supply-chain", ""); rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestSupplyChain_IAMDenyScopedToApp(t *testing.T) {
	rt, db, svc, _ := newSupplyChainTestRouter(t)
	bootstrapTestAdmin(t, db)
	seedScheduledTaskApp(t, db, "secret-app")
	seedScheduledTaskApp(t, db, "open-app")
	seedSupplyChainAttempt(t, db, svc, "secret-app", "dep_secret", time.Now())
	seedSupplyChainAttempt(t, db, svc, "open-app", "dep_open", time.Now())
	reader := storeUserWithAbilitiesForTest(t, db, "sc-reader@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, reader.ID)
	attachTestPolicy(t, db, "deny-secret-sc", "Deny", AbilityRead, "app:secret-app", store.PrincipalTypeUser, reader.ID)

	for _, leaf := range []string{"sbom", "sbom?download=true", "vulnerabilities"} {
		if rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/secret-app/deployments/dep_secret/"+leaf, ""); rec.Code != http.StatusForbidden {
			t.Errorf("denied app %s status = %d, want 403", leaf, rec.Code)
		}
	}
	if rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/open-app/deployments/dep_open/sbom", ""); rec.Code != http.StatusOK {
		t.Errorf("open app status = %d, want 200", rec.Code)
	}
	if rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/open-app/deployments/dep_secret/sbom", ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-app deployment status = %d, want 404", rec.Code)
	}
}

func TestSupplyChainSettings_RoundTripAndValidation(t *testing.T) {
	rt, db, _, _ := newSupplyChainTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/supply-chain", "")
	var got supplyChainSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	if got.ScanEnabled || got.ScanGate != "off" || !got.ServerEnabled || got.BuildAttest || got.Scanner != "trivy" || got.ScannerImage == "" {
		t.Errorf("defaults = %+v", got)
	}

	if rec := scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/supply-chain", `{"scan_gate":"warn"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("gate without scanning status = %d, want 400", rec.Code)
	}
	if rec := scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/supply-chain", `{"scan_enabled":true,"scan_gate":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad gate status = %d, want 400", rec.Code)
	}
	if rec := scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/supply-chain", `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad body status = %d, want 400", rec.Code)
	}
	rec = scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/supply-chain", `{"scan_enabled":true,"scan_gate":"block_on_critical"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.ScanEnabled || got.ScanGate != "block_on_critical" {
		t.Errorf("after put = %+v (%v)", got, err)
	}
	if rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/ghost/supply-chain", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown app status = %d, want 404", rec.Code)
	}
}

func TestSupplyChainOverride_NeedsReasonAndRecordsAnEvent(t *testing.T) {
	rt, db, _, _ := newSupplyChainTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/supply-chain", `{"scan_enabled":true,"scan_gate":"block_on_critical"}`)

	if rec := scCall(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/supply-chain/override", `{"reason":"  "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty reason status = %d, want 400", rec.Code)
	}
	rec := scCall(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/supply-chain/override", `{"reason":"customer outage"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("override: %d %s", rec.Code, rec.Body.String())
	}
	var got supplyChainSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.OverrideArmed || got.OverrideReason != "customer outage" || got.OverrideExpires == nil {
		t.Errorf("override state = %+v (%v)", got, err)
	}
	events, err := db.ListAppEvents(context.Background(), "web", nil, time.Time{}, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		found = found || e.Kind == store.AppEventScanGateOverride
	}
	if !found {
		t.Errorf("the override must leave an audit event, got %+v", events)
	}
}

func TestSupplyChain_SBOMSummaryDownloadAndVulnerabilities(t *testing.T) {
	rt, db, svc, runner := newSupplyChainTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	seedSupplyChainAttempt(t, db, svc, "web", "dep_1", time.Now())

	rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/deployments/dep_1/sbom", "")
	var sbom sbomResource
	if err := json.Unmarshal(rec.Body.Bytes(), &sbom); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("sbom: %d %s", rec.Code, rec.Body.String())
	}
	if sbom.Format != "spdx" || sbom.PackageCount != 2 || !sbom.Provenance || !sbom.Available || sbom.DownloadURL == "" || len(sbom.TopPackages) != 2 {
		t.Errorf("sbom = %+v", sbom)
	}

	rec = scCall(t, rt, cookie, http.MethodGet, sbom.DownloadURL, "")
	if rec.Code != http.StatusOK || rec.Body.String() != testSPDX {
		t.Fatalf("download: %d %q", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "dep_1.sbom.json") {
		t.Errorf("content disposition = %q", cd)
	}

	rec = scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/deployments/dep_1/vulnerabilities", "")
	var vulns vulnResource
	if err := json.Unmarshal(rec.Body.Bytes(), &vulns); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("vulns: %d %s", rec.Code, rec.Body.String())
	}
	if vulns.Scan.Status != "" || vulns.Scan.Counts != nil || len(vulns.Scan.Top) != 0 {
		t.Errorf("unscanned deploy = %+v", vulns)
	}

	if rec := scCall(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/deployments/dep_1/scan", ""); rec.Code != http.StatusConflict {
		t.Errorf("scan while disabled status = %d, want 409", rec.Code)
	}
	scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/supply-chain", `{"scan_enabled":true}`)
	rec = scCall(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/deployments/dep_1/scan", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &vulns); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("scan: %d %s", rec.Code, rec.Body.String())
	}
	if vulns.Scan.Status != "ok" || vulns.Scan.Counts == nil || vulns.Scan.Counts.Critical != 1 || vulns.Scan.Fixable != 1 || vulns.Scan.Top[0].ID != "CVE-1" || runner.runs != 1 {
		t.Errorf("scan result = %+v runs %d", vulns, runner.runs)
	}

	if rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/deployments/missing/sbom", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing deployment status = %d, want 404", rec.Code)
	}
}

func TestSupplyChain_AdditiveFieldsOnDeployLists(t *testing.T) {
	rt, db, svc, _ := newSupplyChainTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedScheduledTaskApp(t, db, "web")
	seedSupplyChainAttempt(t, db, svc, "web", "dep_sbom", time.Now().Add(-time.Minute))
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "dep_plain", ServiceName: "web", Image: "web:2", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/supply-chain", `{"scan_enabled":true}`)
	scCall(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/deployments/dep_sbom/scan", "")

	rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", "")
	var attempts []deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &attempts); err != nil {
		t.Fatal(err)
	}
	byID := map[string]deployAttemptResource{}
	for _, a := range attempts {
		byID[a.ID] = a
	}
	if p := byID["dep_sbom"].SBOMPackages; p == nil || *p != 2 || byID["dep_sbom"].VulnCounts == nil || byID["dep_sbom"].VulnCounts.Critical != 1 {
		t.Errorf("attempt with sbom = %+v", byID["dep_sbom"])
	}
	if byID["dep_plain"].SBOMPackages != nil || byID["dep_plain"].VulnCounts != nil {
		t.Errorf("attempt without sbom must stay null: %+v", byID["dep_plain"])
	}

	rec = scCall(t, rt, cookie, http.MethodGet, "/api/v1/deployments", "")
	var list deploymentListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("deployments: %d %s", rec.Code, rec.Body.String())
	}
	seen := 0
	for _, d := range list.Items {
		switch d.ID {
		case "dep_sbom":
			seen++
			if d.SBOMPackages == nil || *d.SBOMPackages != 2 || d.VulnCounts == nil || d.VulnCounts.Critical != 1 {
				t.Errorf("deployment with sbom = %+v", d)
			}
		case "dep_plain":
			seen++
			if d.SBOMPackages != nil || d.VulnCounts != nil {
				t.Errorf("deployment without sbom must stay null: %+v", d)
			}
		}
	}
	if seen != 2 {
		t.Errorf("saw %d of 2 seeded deployments", seen)
	}
}

func TestAttachSupplyChain_NotConfiguredLeavesFieldsNull(t *testing.T) {
	rt, _ := newTestRouter(t)
	items := []deploymentResource{{ID: "dep_1", App: "web"}}
	rt.attachSupplyChain(context.Background(), items)
	if items[0].SBOMPackages != nil || items[0].VulnCounts != nil {
		t.Errorf("items = %+v", items[0])
	}
}
