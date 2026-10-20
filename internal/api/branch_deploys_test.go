package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const zeroSHA = "0000000000000000000000000000000000000000"

func sendBranchPush(rt *Router, secret, ref, after string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"ref": ref, "after": after, "before": "base1", "repository": map[string]any{"full_name": "org/web"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sign([]byte(secret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func setBranchDeployPolicy(t *testing.T, rt *Router, db *store.DB, body string) {
	t.Helper()
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/branch-deploys", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("set branch deploy policy: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestBranchDeploy_PolicyGatesPushes(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)

	rec := sendBranchPush(rt, secret, "refs/heads/dev", "sha-a")
	if rec.Code != http.StatusOK || len(builder.calls) != 0 {
		t.Fatalf("default policy must not branch deploy: status=%d calls=%d", rec.Code, len(builder.calls))
	}

	setBranchDeployPolicy(t, rt, db, `{"mode":"patterns","patterns":["dev","feature/*"]}`)
	tests := []struct {
		name, ref string
		wantCalls int
	}{
		{"exact match deploys", "refs/heads/dev", 1},
		{"redeploy on next push", "refs/heads/dev", 2},
		{"glob match deploys", "refs/heads/feature/login", 3},
		{"unlisted branch ignored", "refs/heads/other", 3},
		{"production branch uses normal flow", "refs/heads/main", 4},
		{"tag push ignored", "refs/tags/v1", 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := sendBranchPush(rt, secret, tc.ref, "sha-b"); rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if len(builder.calls) != tc.wantCalls {
				t.Fatalf("Deploy calls = %d, want %d", len(builder.calls), tc.wantCalls)
			}
		})
	}

	rows, err := db.ListBranchDeploysByApp(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (dev redeploy reuses its row)", len(rows))
	}
	if builder.calls[0].ServiceName != "web-br-dev" {
		t.Fatalf("ServiceName = %q, want web-br-dev", builder.calls[0].ServiceName)
	}
	if !strings.HasPrefix(builder.calls[2].ServiceName, "web-br-feature-login-") {
		t.Fatalf("slash branch must get a collision-safe name, got %q", builder.calls[2].ServiceName)
	}
}

func TestBranchDeploy_AllModeSkipsProductionAndHonoursLimit(t *testing.T) {
	t.Setenv(envBranchDeployMaxPerApp, "1")
	rt, db, secret, builder := setUpPreviewApp(t)
	setBranchDeployPolicy(t, rt, db, `{"mode":"all"}`)

	if rec := sendBranchPush(rt, secret, "refs/heads/a", "sha1"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	rec := sendBranchPush(rt, secret, "refs/heads/b", "sha2")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "limit") {
		t.Fatalf("second branch must be refused at the cap: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(builder.calls) != 1 {
		t.Fatalf("Deploy calls = %d, want 1", len(builder.calls))
	}
}

func TestBranchDeploy_DeleteEventRemovesDeploy(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	setBranchDeployPolicy(t, rt, db, `{"mode":"all"}`)
	sendBranchPush(rt, secret, "refs/heads/dev", "sha1")

	if rec := sendBranchPush(rt, secret, "refs/heads/dev", zeroSHA); rec.Code != http.StatusOK {
		t.Fatalf("delete push: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rows, _ := db.ListBranchDeploysByApp(context.Background(), "web")
	if len(rows) != 0 {
		t.Fatalf("rows after delete = %d, want 0", len(rows))
	}
	if _, err := db.GetDesiredService(context.Background(), "web-br-dev"); err == nil {
		t.Fatal("branch deploy service must be removed")
	}
	if rec := sendBranchPush(rt, secret, "refs/heads/main", zeroSHA); !strings.Contains(rec.Body.String(), "production branch was deleted") {
		t.Fatalf("production branch delete must be ignored, got %q", rec.Body.String())
	}
}

func TestBranchDeploy_TeardownHalfSuccessKeepsRow(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	setBranchDeployPolicy(t, rt, db, `{"mode":"all"}`)
	sendBranchPush(rt, secret, "refs/heads/dev", "sha1")

	rt.apps = &failingDeleteAppStore{AppStore: rt.apps, failName: "web-br-dev"}
	rec := sendBranchPush(rt, secret, "refs/heads/dev", zeroSHA)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want 207, body = %s", rec.Code, rec.Body.String())
	}
	row, err := db.GetBranchDeployBySlug(context.Background(), "web", "dev")
	if err != nil {
		t.Fatalf("row must survive a partial teardown: %v", err)
	}
	if row.Status != store.BranchDeployFailed || !strings.Contains(row.StatusReason, "undeleted") {
		t.Fatalf("row = %+v, want failed with a reason", row)
	}

	rt.apps = rt.apps.(*failingDeleteAppStore).AppStore
	if rec := sendBranchPush(rt, secret, "refs/heads/dev", zeroSHA); rec.Code != http.StatusOK {
		t.Fatalf("retry status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetBranchDeployBySlug(context.Background(), "web", "dev"); err == nil {
		t.Fatal("retry must remove the row")
	}
}

func TestBranchDeploy_SweepRemovesIdleOnly(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	setBranchDeployPolicy(t, rt, db, `{"mode":"all"}`)
	sendBranchPush(rt, secret, "refs/heads/old", "sha1")
	sendBranchPush(rt, secret, "refs/heads/fresh", "sha2")

	old, err := db.GetBranchDeployBySlug(context.Background(), "web", "old")
	if err != nil {
		t.Fatal(err)
	}
	old.UpdatedAt = time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	if err := db.UpdateBranchDeploy(context.Background(), *old); err != nil {
		t.Fatal(err)
	}

	swept, err := rt.SweepStaleBranchDeploys(context.Background())
	if err != nil || swept != 1 {
		t.Fatalf("swept=%d err=%v, want 1 nil", swept, err)
	}
	rows, _ := db.ListBranchDeploysByApp(context.Background(), "web")
	if len(rows) != 1 || rows[0].Slug != "fresh" {
		t.Fatalf("rows = %+v, want only fresh", rows)
	}
}

func TestBranchDeploy_SettingsValidation(t *testing.T) {
	rt, db, _, _ := setUpPreviewApp(t)
	cookie := loginTestSession(t, rt, db)
	tests := []struct {
		name, body string
		want       int
	}{
		{"bad mode", `{"mode":"sometimes"}`, http.StatusBadRequest},
		{"patterns mode needs entries", `{"mode":"patterns","patterns":[" "]}`, http.StatusBadRequest},
		{"whitespace in pattern", `{"mode":"patterns","patterns":["a b"]}`, http.StatusBadRequest},
		{"ok", `{"mode":"patterns","patterns":["dev"," dev ","qa/*"]}`, http.StatusOK},
		{"off", `{"mode":"off"}`, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/branch-deploys", tc.body))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestBranchDeploy_ListAndDeleteRoutes(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	setBranchDeployPolicy(t, rt, db, `{"mode":"all"}`)
	sendBranchPush(rt, secret, "refs/heads/dev", "sha1")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/branch-deploys", ""))
	var got branchDeploysResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list: status=%d err=%v body=%s", rec.Code, err, rec.Body.String())
	}
	if got.Settings.Mode != "all" || got.Settings.ProductionBranch != "main" || len(got.Deploys) != 1 || got.Deploys[0].Slug != "dev" {
		t.Fatalf("list = %+v", got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/branch-deploys/dev", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/branch-deploys/dev", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second delete: status = %d, want 404", rec.Code)
	}
}
