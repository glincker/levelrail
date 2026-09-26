package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCrossAppListsHideDeniedApps(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, app := range []string{"web", "api"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: app + ":1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
			ID: "dep_" + app, ServiceName: app, Image: app + ":1", Source: store.DeployAttemptSourceImage,
			Status: store.DeployAttemptStatusFailed, StartedAt: now.Add(-time.Minute), Error: "boom",
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveDeployApproval(ctx, store.DeployApproval{
			ID: "apr_" + app, ServiceName: app, EnvironmentID: "env_prod",
			Action: store.DeployApprovalActionDeploy, Image: app + ":2",
			Status:          store.DeployApprovalStatusPending,
			RequestedByType: store.PrincipalTypeUser, RequestedBy: "u", RequestedByName: "U",
			CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatal(err)
		}
	}
	tok := seedMatrixToken(t, db, "scoped-tok", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-api", "Deny", "*", "app:api", store.PrincipalTypeToken, "scoped-tok")

	get := func(path string) []byte {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		bearer(tok)(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.Bytes()
	}

	var failed []failedDeployResource
	if err := json.Unmarshal(get("/api/v1/deploys/failed"), &failed); err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].ServiceName != "web" {
		t.Fatalf("failed deploys = %+v, want only web", failed)
	}

	var approvals deployApprovalListResponse
	if err := json.Unmarshal(get("/api/v1/deploy-approvals"), &approvals); err != nil {
		t.Fatal(err)
	}
	if len(approvals.Approvals) != 1 || approvals.Approvals[0].ServiceName != "web" {
		t.Fatalf("approvals = %+v, want only web", approvals.Approvals)
	}
}
