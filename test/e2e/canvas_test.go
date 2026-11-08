// Package e2e: this file drives the canvas path (draft compiled to files,
// plan, apply) over a real HTTP session. It is not a real host run: apply
// only writes desired state, no container engine is started.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eCanvasAdminUsername = "e2e-canvas-admin"
	e2eCanvasAdminPassword = "e2e-canvas-correct-horse-battery" //nolint:gosec // test fixture credential, not a real secret

	e2eCanvasProjectID   = "proj_e2e_canvas"
	e2eCanvasProjectName = "e2e-canvas-shop"
	e2eCanvasAppName     = "e2e-canvas-web"
)

// e2eCanvasDraftFiles mirrors web/src/lib/canvasDraft.ts draftFiles for one
// add-app op, so the wire payload is what the editor sends.
func e2eCanvasDraftFiles(t *testing.T) []e2eIaCFile {
	t.Helper()
	doc := map[string]any{
		"version":  1,
		"kind":     "App",
		"metadata": map[string]string{"name": e2eCanvasAppName},
		"spec": map[string]any{
			"project": e2eCanvasProjectName,
			"service": map[string]any{
				"build": map[string]string{"type": "image", "image": "ghcr.io/glincker-e2e/web:1.0.0"},
				"port":  3000,
			},
		},
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal draft: %v", err)
	}
	return []e2eIaCFile{{Name: "canvas-app-" + e2eCanvasAppName + ".yaml", Content: string(raw)}}
}

func startCanvasServer(t *testing.T) (*store.DB, string, *http.Client) {
	t.Helper()
	db := openLiveStore(t)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: e2eCanvasProjectID, Name: e2eCanvasProjectName, CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	ts := newE2ETestServer(t, api.NewRouter(discardTestLogger(), b, db))
	if err := api.BootstrapAdmin(ctx, db, e2eCanvasAdminUsername, e2eCanvasAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	return db, ts.URL, loginE2EClient(t, ts.URL, e2eCanvasAdminUsername, e2eCanvasAdminPassword)
}

func TestCanvas_Live_DraftPlanApplyAuditAndRefusals(t *testing.T) {
	experimental.Set(experimental.All()...)
	t.Cleanup(experimental.Reset)

	db, baseURL, admin := startCanvasServer(t)
	ctx := context.Background()
	files := e2eCanvasDraftFiles(t)
	planBody, err := json.Marshal(e2eIaCRequest{Files: files})
	if err != nil {
		t.Fatalf("marshal plan request: %v", err)
	}

	status, body := requestJSON(t, admin, http.MethodPost, baseURL+"/api/v1/apply/plan", string(planBody))
	if status != http.StatusOK {
		t.Fatalf("plan: status = %d, body = %s", status, body)
	}
	var planResp e2eIaCPlanResponse
	if err := json.Unmarshal(body, &planResp); err != nil || planResp.Plan == nil {
		t.Fatalf("decode plan: %v, body = %s", err, body)
	}
	if planResp.Plan.Summary.Create != 1 || planResp.Plan.Summary.Error != 0 {
		t.Fatalf("plan summary = %+v, want one create", planResp.Plan.Summary)
	}
	if _, err := db.GetDesiredService(ctx, e2eCanvasAppName); err == nil {
		t.Fatal("planning created the app")
	}

	// A read-only token may plan but must not apply.
	readToken := createE2EToken(t, admin, baseURL, "e2e-canvas-read", `["read"]`)
	applyBody, err := json.Marshal(e2eIaCRequest{Files: files, ExpectedPlanHash: planResp.Plan.Hash})
	if err != nil {
		t.Fatalf("marshal apply request: %v", err)
	}
	viewer := &http.Client{Timeout: e2eHTTPTimeout, Transport: bearerTransport{token: readToken}}
	if status, raw := requestJSON(t, viewer, http.MethodPost, baseURL+"/api/v1/apply/plan", string(planBody)); status != http.StatusOK {
		t.Fatalf("viewer plan: status = %d, body = %s", status, raw)
	}
	if status, raw := requestJSON(t, viewer, http.MethodPost, baseURL+"/api/v1/apply", string(applyBody)); status != http.StatusForbidden {
		t.Fatalf("viewer apply: status = %d, want 403, body = %s", status, raw)
	}
	if _, err := db.GetDesiredService(ctx, e2eCanvasAppName); err == nil {
		t.Fatal("refused viewer apply still created the app")
	}

	status, body = requestJSON(t, admin, http.MethodPost, baseURL+"/api/v1/apply", string(applyBody))
	if status != http.StatusOK {
		t.Fatalf("apply: status = %d, body = %s", status, body)
	}
	var applyResp e2eIaCApplyResponse
	if err := json.Unmarshal(body, &applyResp); err != nil || applyResp.Result == nil || !applyResp.Result.OK() || applyResp.Result.Applied != 1 {
		t.Fatalf("apply result = %+v, err = %v, body = %s", applyResp.Result, err, body)
	}
	svc, err := db.GetDesiredService(ctx, e2eCanvasAppName)
	if err != nil {
		t.Fatalf("GetDesiredService() after apply: %v", err)
	}
	if svc.Image != "ghcr.io/glincker-e2e/web:1.0.0" || svc.Port != 3000 {
		t.Fatalf("applied service = %+v", svc)
	}

	status, raw := requestJSON(t, admin, http.MethodGet, baseURL+"/api/v1/audit-log?limit=100", "")
	if status != http.StatusOK || !strings.Contains(string(raw), "/apply") {
		t.Fatalf("audit log: status = %d, want an apply entry, body = %.300s", status, raw)
	}

	// Flag off: the plan and apply routes the canvas calls refuse cleanly.
	experimental.Set()
	for _, p := range []string{"/api/v1/apply/plan", "/api/v1/apply"} {
		status, raw := requestJSON(t, admin, http.MethodPost, baseURL+p, string(applyBody))
		if status != http.StatusNotFound {
			t.Fatalf("flag off %s: status = %d, want 404, body = %s", p, status, raw)
		}
	}
}
