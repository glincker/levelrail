// TestDeployFreeze_Live_BlocksThenAllows is this package's e2e proof for
// the deploy freeze gate (internal/deploy/freeze.go, handlePutAppDeployFreeze
// and the freezeGate check in handleTriggerDeploy, internal/api/deploys.go):
// a real admin session drives PUT .../deploy-freeze and POST .../deploys
// over a real HTTP server, backed by a real store.DB, with no Docker
// involved since the freeze check runs before any container work.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eDeployFreezeAdminUsername = "e2e-deploy-freeze-admin"
	e2eDeployFreezeAdminPassword = "e2e-deploy-freeze-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2eDeployFreezeServiceName   = "levelrail-test-e2e-deploy-freeze"
)

// deployFreezeStatusView mirrors internal/api's unexported
// deployFreezeResource.Status field, decoded here the same way
// protected_environment_test.go's decodePendingApprovalID reaches across
// the real HTTP boundary instead of the package boundary.
type deployFreezeStatusView struct {
	Frozen bool   `json:"frozen"`
	Reason string `json:"reason"`
}

type deployFreezeView struct {
	Status deployFreezeStatusView `json:"status"`
}

func TestDeployFreeze_Live_BlocksThenAllows(t *testing.T) {
	db := openLiveStore(t)
	ctx := context.Background()

	const serviceName = e2eDeployFreezeServiceName
	imageA := "nginx:freeze-a"
	imageB := "nginx:freeze-b"
	imageC := "nginx:freeze-c"

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: serviceName, Image: imageA, Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	// Resolver nil: this test never touches a registry or Docker, only
	// the freeze gate ahead of it.
	router := api.NewRouter(logger, b, db, api.WithDeploySafety(db, nil))
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, db, e2eDeployFreezeAdminUsername, e2eDeployFreezeAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eDeployFreezeAdminUsername, e2eDeployFreezeAdminPassword)

	deployFreezeURL := ts.URL + "/api/v1/apps/" + serviceName + "/deploy-freeze"
	deploysURL := ts.URL + "/api/v1/apps/" + serviceName + "/deploys"

	// Step 1: enable a freeze window that always covers "now", the same
	// shape internal/api/deploy_safety_test.go's alwaysFrozen uses. PUT,
	// not POST: this route only matches PUT (routes.go), so requestJSON
	// (auth_lifecycle_test.go) is used here instead of postJSON.
	status, rawBody := requestJSON(t, client, http.MethodPut, deployFreezeURL, `{"windows":[{"cron":"* * * * *","duration":"2h","reason":"release freeze"}]}`)
	body := string(rawBody)
	if status != http.StatusOK {
		t.Fatalf("enable freeze: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	var view deployFreezeView
	if err := json.Unmarshal(rawBody, &view); err != nil {
		t.Fatalf("enable freeze: decode body %s: %v", body, err)
	}
	if !view.Status.Frozen {
		t.Fatalf("enable freeze: Status.Frozen = false, want true, body = %s", body)
	}

	// Step 2: a real manual deploy attempt is refused with a real 423 and
	// a reason an operator could actually read, and desired state (the
	// thing a container would eventually be created from) never moves.
	status, body = postJSON(t, client, deploysURL, `{"image":"`+imageB+`"}`)
	if status != http.StatusLocked {
		t.Fatalf("deploy while frozen: status = %d, want %d, body = %s", status, http.StatusLocked, body)
	}
	if !strings.Contains(body, "frozen") {
		t.Errorf("deploy while frozen: body = %s, want it to mention the freeze", body)
	}
	svc, err := db.GetDesiredService(ctx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != imageA {
		t.Fatalf("desired state Image = %q after a blocked deploy, want unchanged %q", svc.Image, imageA)
	}

	// Step 3: an override with no reason is rejected; a real override
	// with a reason goes through, and the reason lands on the recorded
	// deploy attempt, proving the escape hatch is real, not decorative.
	status, body = postJSON(t, client, deploysURL, `{"image":"`+imageB+`","override_freeze":true}`)
	if status != http.StatusBadRequest {
		t.Fatalf("override without reason: status = %d, want %d, body = %s", status, http.StatusBadRequest, body)
	}
	status, body = postJSON(t, client, deploysURL, `{"image":"`+imageB+`","override_freeze":true,"override_reason":"hotfix CVE"}`)
	if status != http.StatusAccepted {
		t.Fatalf("override with reason: status = %d, want %d, body = %s", status, http.StatusAccepted, body)
	}
	svc, err = db.GetDesiredService(ctx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != imageB {
		t.Fatalf("desired state Image = %q after an overridden deploy, want %q", svc.Image, imageB)
	}
	attempts, err := db.ListDeployAttempts(ctx, serviceName)
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	if len(attempts) != 1 || attempts[0].Reason != "FreezeOverride: hotfix CVE" {
		t.Fatalf("attempts = %+v, want one attempt with the override reason recorded", attempts)
	}

	// Step 4: clearing the freeze window (the same real PUT, with an
	// empty window list) lets the identical deploy request through with
	// no override at all.
	status, rawBody = requestJSON(t, client, http.MethodPut, deployFreezeURL, `{"windows":[]}`)
	body = string(rawBody)
	if status != http.StatusOK {
		t.Fatalf("clear freeze: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	if err := json.Unmarshal(rawBody, &view); err != nil {
		t.Fatalf("clear freeze: decode body %s: %v", body, err)
	}
	if view.Status.Frozen {
		t.Fatalf("clear freeze: Status.Frozen = true, want false, body = %s", body)
	}

	status, body = postJSON(t, client, deploysURL, `{"image":"`+imageC+`"}`)
	if status != http.StatusAccepted {
		t.Fatalf("deploy outside freeze: status = %d, want %d, body = %s", status, http.StatusAccepted, body)
	}
	svc, err = db.GetDesiredService(ctx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != imageC {
		t.Fatalf("desired state Image = %q after an unfrozen deploy, want %q", svc.Image, imageC)
	}
}
