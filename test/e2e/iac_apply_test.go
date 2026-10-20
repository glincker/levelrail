// Package e2e: this file proves platform-as-code (export/plan/apply) over
// a real HTTP session end to end, closing the no-e2e gap
// docs/feature-status.md flags for internal/iac. No Docker: apply only
// ever writes desired state through the ordinary app API.
package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eIaCAdminUsername = "e2e-iac-admin"
	e2eIaCAdminPassword = "e2e-iac-correct-horse-battery" //nolint:gosec // test fixture credential, not a real secret

	e2eIaCProjectID   = "proj_e2e_iac"
	e2eIaCProjectName = "e2e-iac-shop"
	e2eIaCAppName     = "e2e-iac-web"
)

// e2eIaCFile and e2eIaCRequest mirror internal/api's unexported iacFile and
// iacRequest JSON shapes, for the same reason auth_lifecycle_test.go's
// authResponse mirrors loginResponse: this test sits on the far side of a
// real HTTP boundary and decodes/encodes the actual wire contract rather
// than reaching across the package boundary.
type e2eIaCFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type e2eIaCRequest struct {
	Files            []e2eIaCFile `json:"files"`
	Source           string       `json:"source,omitempty"`
	ExpectedPlanHash string       `json:"expected_plan_hash,omitempty"`
}

// e2eIaCPlanResponse and e2eIaCApplyResponse mirror internal/api's
// unexported iacPlanResponse/iacApplyResponse envelopes. iac.Plan,
// iac.ApplyResult and iac.Issue are exported by internal/iac itself, so
// only the envelope needs mirroring here.
type e2eIaCPlanResponse struct {
	Plan   *iac.Plan   `json:"plan,omitempty"`
	Issues []iac.Issue `json:"issues,omitempty"`
}

type e2eIaCApplyResponse struct {
	Result *iac.ApplyResult `json:"result,omitempty"`
	Issues []iac.Issue      `json:"issues,omitempty"`
}

// e2eExperimentalErrorResponse mirrors internal/api's unexported
// experimentalError body, for the same mirroring reason as above.
type e2eExperimentalErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Feature string `json:"feature"`
}

func TestIaCApply_Live_ExportPlanApplyRoundTrip(t *testing.T) {
	// All features, not just IaC: export's appExtras also reads an app's
	// load balancer config (internal/iac/state.go's loader.lb), and that
	// sub-call 404s through the experimental gate same as any other
	// request when load-balancer is off, which loader.lb's IsDenied/
	// IsUnavailable check does not treat as tolerable the way an actual
	// 403/501 would. Matches internal/api's own TestMain, which enables
	// every feature for the same reason.
	experimental.Set(experimental.All()...)
	t.Cleanup(experimental.Reset)

	db := openLiveStore(t)
	ctx := context.Background()

	// Seed the "current state" an operator would already have running,
	// written directly to the store the same way protected_environment_test.go
	// seeds its starting container: this is setup, not the thing under
	// test. Replicas: 2 (not spec.DefaultReplicas) so export.go actually
	// emits a literal "replicas: 2" line rather than omitting it.
	if err := db.SaveProject(ctx, store.Project{ID: e2eIaCProjectID, Name: e2eIaCProjectName, CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name:     e2eIaCAppName,
		Image:    "ghcr.io/glincker-e2e/web:1.0.0",
		Port:     3000,
		Domains:  []string{"e2e-iac.example.com"},
		Env:      map[string]string{"LOG_LEVEL": "info"},
		Replicas: 2,
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.UpdateServiceProject(ctx, e2eIaCAppName, e2eIaCProjectID); err != nil {
		t.Fatalf("UpdateServiceProject() error = %v", err)
	}

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, db)
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, db, e2eIaCAdminUsername, e2eIaCAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eIaCAdminUsername, e2eIaCAdminPassword)

	// Step 1: export the real current state as code, over real HTTP,
	// through the real loopback doer that re-enters the real app/project
	// handlers with the caller's own session.
	status, body := requestJSON(t, client, "GET", ts.URL+"/api/v1/export?project="+e2eIaCProjectName, "")
	if status != 200 {
		t.Fatalf("export: status = %d, body = %s", status, body)
	}
	var exported iac.ExportResult
	if err := json.Unmarshal(body, &exported); err != nil {
		t.Fatalf("decode export response: %v, body = %s", err, body)
	}
	exportedDocs := exported.Join()
	if !strings.Contains(exportedDocs, "name: "+e2eIaCAppName) {
		t.Fatalf("export does not mention the app:\n%s", exportedDocs)
	}
	if n := strings.Count(exportedDocs, "replicas: 2"); n != 1 {
		t.Fatalf("export has %d occurrences of \"replicas: 2\", want exactly 1:\n%s", n, exportedDocs)
	}

	// Step 2: modify the exported code exactly the way an operator would
	// hand-edit a checked-out app.yaml: bump replicas from the live 2 to 4.
	modifiedDocs := strings.Replace(exportedDocs, "replicas: 2", "replicas: 4", 1)
	planReq := e2eIaCRequest{Files: []e2eIaCFile{{Name: "infra/export.yaml", Content: modifiedDocs}}}
	planBody, err := json.Marshal(planReq)
	if err != nil {
		t.Fatalf("marshal plan request: %v", err)
	}

	// Step 3: submit the plan over real HTTP and verify it reports the
	// exact, real intended change: an update to the app with a replicas
	// 2 -> 4 field diff, nothing else created, removed, or errored.
	status, body = requestJSON(t, client, "POST", ts.URL+"/api/v1/apply/plan", string(planBody))
	if status != 200 {
		t.Fatalf("plan: status = %d, body = %s", status, body)
	}
	var planResp e2eIaCPlanResponse
	if err := json.Unmarshal(body, &planResp); err != nil {
		t.Fatalf("decode plan response: %v, body = %s", err, body)
	}
	if planResp.Plan == nil {
		t.Fatalf("plan response has no plan: issues = %+v", planResp.Issues)
	}
	if planResp.Plan.Summary.Update != 1 || planResp.Plan.Summary.Create != 0 || planResp.Plan.Summary.Delete != 0 || planResp.Plan.Summary.Error != 0 {
		t.Fatalf("plan summary = %+v, want exactly one update", planResp.Plan.Summary)
	}
	if !hasFieldChange(planResp.Plan.Changes, e2eIaCAppName, "replicas", "2", "4") {
		t.Fatalf("plan does not show the real replicas 2 -> 4 change: %+v", planResp.Plan.Changes)
	}

	// Planning must never itself mutate anything: the live replicas must
	// still be 2 until apply actually runs.
	if svc, err := db.GetDesiredService(ctx, e2eIaCAppName); err != nil {
		t.Fatalf("GetDesiredService() after plan: error = %v", err)
	} else if svc.Replicas != 2 {
		t.Fatalf("planning changed live state: replicas = %d, want still 2", svc.Replicas)
	}

	// Step 4: apply the same plan, pinned to its own hash so a changed
	// live state between review and apply would be refused.
	applyReq := e2eIaCRequest{Files: planReq.Files, ExpectedPlanHash: planResp.Plan.Hash}
	applyBody, err := json.Marshal(applyReq)
	if err != nil {
		t.Fatalf("marshal apply request: %v", err)
	}
	status, body = requestJSON(t, client, "POST", ts.URL+"/api/v1/apply", string(applyBody))
	if status != 200 {
		t.Fatalf("apply: status = %d, body = %s", status, body)
	}
	var applyResp e2eIaCApplyResponse
	if err := json.Unmarshal(body, &applyResp); err != nil {
		t.Fatalf("decode apply response: %v, body = %s", err, body)
	}
	if applyResp.Result == nil || !applyResp.Result.OK() || applyResp.Result.Applied != 1 {
		t.Fatalf("apply result = %+v, issues = %+v", applyResp.Result, applyResp.Issues)
	}

	// Step 5: verify the real resulting state, read back independently
	// from the store rather than trusting the HTTP response alone, the
	// same independent-verification rigor protected_environment_test.go's
	// assertRunningWithImage applies against Docker directly.
	svc, err := db.GetDesiredService(ctx, e2eIaCAppName)
	if err != nil {
		t.Fatalf("GetDesiredService() after apply: error = %v", err)
	}
	if svc.Replicas != 4 {
		t.Fatalf("applied replicas = %d, want the planned 4", svc.Replicas)
	}
	if svc.Image != "ghcr.io/glincker-e2e/web:1.0.0" || svc.Port != 3000 || len(svc.Domains) != 1 || svc.Domains[0] != "e2e-iac.example.com" {
		t.Fatalf("apply changed fields the plan never touched: %+v", svc)
	}

	// Step 6: planning the identical modified file again must now be a
	// true no-op: the resulting state genuinely matches what was
	// planned, not just what the apply response claimed.
	status, body = requestJSON(t, client, "POST", ts.URL+"/api/v1/apply/plan", string(planBody))
	if status != 200 {
		t.Fatalf("replan: status = %d, body = %s", status, body)
	}
	var replanResp e2eIaCPlanResponse
	if err := json.Unmarshal(body, &replanResp); err != nil {
		t.Fatalf("decode replan response: %v, body = %s", err, body)
	}
	if replanResp.Plan == nil || replanResp.Plan.Pending() {
		t.Fatalf("replan after apply is not a no-op: %+v", replanResp.Plan)
	}
}

// hasFieldChange reports whether changes contains a Change for appName
// carrying a FieldChange at path with exactly old and new values.
func hasFieldChange(changes []iac.Change, appName, path, old, new string) bool { //nolint:revive // "new" reads clearest here, this is a test helper not exported API
	for _, c := range changes {
		if c.Kind != iac.KindApp || c.Name != appName || c.Action != iac.ActionUpdate {
			continue
		}
		for _, fc := range c.Fields {
			if fc.Path == path && fc.Old == old && fc.New == new {
				return true
			}
		}
	}
	return false
}

// TestIaCApply_Live_GateOffReturns404 proves the two routes the round
// trip above depends on refuse cleanly, over the same real HTTP
// boundary, when the iac experimental feature is off: an operator
// running a release without APP_EXPERIMENTAL=iac set gets a clean 404,
// not a half-wired handler.
func TestIaCApply_Live_GateOffReturns404(t *testing.T) {
	experimental.Set()
	t.Cleanup(experimental.Reset)

	db := openLiveStore(t)
	ctx := context.Background()

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, db)
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, db, e2eIaCAdminUsername, e2eIaCAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eIaCAdminUsername, e2eIaCAdminPassword)

	planReq := e2eIaCRequest{Files: []e2eIaCFile{{Name: "infra/app.yaml", Content: "version: 1\nkind: Tag\nmetadata: {name: whatever}\n"}}}
	planBody, err := json.Marshal(planReq)
	if err != nil {
		t.Fatalf("marshal plan request: %v", err)
	}

	for _, tc := range []struct {
		name, method, url, body string
	}{
		{"plan", "POST", ts.URL + "/api/v1/apply/plan", string(planBody)},
		{"export", "GET", ts.URL + "/api/v1/export", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := requestJSON(t, client, tc.method, tc.url, tc.body)
			if status != 404 {
				t.Fatalf("%s %s: status = %d, body = %s, want 404", tc.method, tc.url, status, body)
			}
			var errResp e2eExperimentalErrorResponse
			if err := json.Unmarshal(body, &errResp); err != nil {
				t.Fatalf("decode error response: %v, body = %s", err, body)
			}
			if errResp.Code != api.ExperimentalDisabledCode || errResp.Feature != string(experimental.IaC) {
				t.Fatalf("error body = %+v, want code %q feature %q", errResp, api.ExperimentalDisabledCode, experimental.IaC)
			}
		})
	}
}
