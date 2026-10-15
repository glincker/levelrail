package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy/secretscan"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newSecretScanTestRouter(t *testing.T) (*Router, *store.DB, *secretscan.Service) {
	t.Helper()
	rt, db := newTestRouter(t)
	svc := secretscan.New(secretscan.NewSQLStore(db), secretscan.DefaultLimits(), slog.Default())
	rt.SetSecretScan(svc)
	return rt, db, svc
}

func TestSecretScanRoutes_RequireAuth(t *testing.T) {
	rt, _, _ := newSecretScanTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/secret-scan", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSecretScanSettingsAndAllowList(t *testing.T) {
	rt, db, _ := newSecretScanTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	var got secretScanSettingsResource
	rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/secret-scan", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || got.Policy != "warn" {
		t.Fatalf("default: %d %s", rec.Code, rec.Body.String())
	}
	if rec := scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secret-scan", `{"policy":"strict"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad policy status = %d, want 400", rec.Code)
	}
	rec = scCall(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secret-scan", `{"policy":"block"}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || got.Policy != "block" {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}

	const fp = "0123456789abcdef"
	if rec := scCall(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/secret-scan/allow", `{"fingerprint":"`+fp+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("allow without reason status = %d, want 400", rec.Code)
	}
	if rec := scCall(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/secret-scan/allow", `{"fingerprint":"`+fp+`","reason":"public test key"}`); rec.Code != http.StatusCreated {
		t.Fatalf("allow: %d %s", rec.Code, rec.Body.String())
	}
	rec = scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/secret-scan", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.AllowList) != 1 || got.AllowList[0].Reason != "public test key" || got.AllowList[0].Actor == "" {
		t.Fatalf("allow list = %+v", got.AllowList)
	}
	if rec := scCall(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/secret-scan/allow/"+fp, ""); rec.Code != http.StatusNoContent {
		t.Errorf("unallow status = %d", rec.Code)
	}
	if rec := scCall(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/secret-scan/allow/"+fp, ""); rec.Code != http.StatusNotFound {
		t.Errorf("second unallow status = %d, want 404", rec.Code)
	}
}

func TestSecretScanDeploymentRun(t *testing.T) {
	rt, db, svc := newSecretScanTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	if rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/deployments/none/secret-scan", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing run status = %d, want 404", rec.Code)
	}
	if _, err := svc.SetPolicy(context.Background(), "web", "block"); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"STRIPE": "sk" + "_live_" + strings.Repeat("a1B2", 6)}
	if err := svc.BeforeDeploy(context.Background(), secretscan.Input{App: "web", AttemptID: "att-9", Env: env}); err == nil {
		t.Fatal("expected a block")
	}
	rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/deployments/att-9/secret-scan", "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), env["STRIPE"]) {
		t.Fatalf("run: %d (value must not be echoed) %s", rec.Code, rec.Body.String())
	}
	var run secretscan.Run
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil || run.Action != secretscan.ActionBlock || len(run.Findings) != 1 {
		t.Fatalf("run = %+v err=%v", run, err)
	}
	if rec := scCall(t, rt, cookie, http.MethodGet, "/api/v1/apps/other/deployments/att-9/secret-scan", ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-app read status = %d, want 404", rec.Code)
	}
}
