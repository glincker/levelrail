package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func getAppsSummary(t *testing.T, rt *Router, tok, query string) appsSummaryResource {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps-summary?"+query, nil)
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET apps-summary = %d %s", rec.Code, rec.Body.String())
	}
	var sum appsSummaryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &sum); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return sum
}

func setAppCondition(t *testing.T, db *store.DB, app string, status reconcile.ConditionStatus) {
	t.Helper()
	err := db.UpsertConditions(context.Background(), applicationControllerName(app), []reconcile.Condition{
		{Type: "Ready", Status: status, Reason: "Test"},
	})
	if err != nil {
		t.Fatalf("seed conditions %s: %v", app, err)
	}
}

func TestAppsSummaryAndStatusFilter_AreTenantScoped(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedSearchApps(t, db, "ok-a", "ok-b", "bad-a", "secret-bad")
	setAppCondition(t, db, "ok-a", reconcile.ConditionTrue)
	setAppCondition(t, db, "ok-b", reconcile.ConditionTrue)
	setAppCondition(t, db, "bad-a", reconcile.ConditionFalse)
	setAppCondition(t, db, "secret-bad", reconcile.ConditionFalse)
	root := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	reader := seedMatrixToken(t, db, "reader", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-secret", "Deny", "*", "app:secret-bad", store.PrincipalTypeToken, "reader")

	if sum := getAppsSummary(t, rt, root, ""); sum.Total != 4 || sum.Running != 2 || sum.Failing != 2 || sum.Attention != 2 {
		t.Fatalf("root summary = %+v", sum)
	}
	if sum := getAppsSummary(t, rt, reader, ""); sum.Total != 3 || sum.Failing != 1 || sum.Attention != 1 {
		t.Fatalf("reader summary leaks a denied app: %+v", sum)
	}

	names, total := listAppNames(t, rt, root, "status=failing")
	if len(names) != 2 || total != "2" {
		t.Fatalf("root failing = %v total %s", names, total)
	}
	names, total = listAppNames(t, rt, reader, "status=failing")
	if strings.Join(names, ",") != "bad-a" || total != "1" {
		t.Fatalf("reader failing = %v total %s, must hide the denied app", names, total)
	}
	names, total = listAppNames(t, rt, root, "status=running&limit=1&offset=1")
	if len(names) != 1 || total != "2" {
		t.Fatalf("paged running = %v total %s", names, total)
	}
}

func TestAppsList_MultiEnvironmentFilterAndSummary(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedSearchApps(t, db, "one", "two", "three")
	root := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	if err := db.SaveProject(ctx, store.Project{ID: "proj-1", Name: "p1", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for app, env := range map[string]string{"one": "prod", "two": "staging"} {
		if err := db.SaveEnvironment(ctx, store.Environment{ID: "env-" + env, ProjectID: "proj-1", Name: env, CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE desired_services SET environment_id = ? WHERE name = ?`, "env-"+env, app); err != nil {
			t.Fatal(err)
		}
	}
	names, total := listAppNames(t, rt, root, "environment=prod&environment=staging")
	if len(names) != 2 || total != "2" {
		t.Fatalf("two environments = %v total %s", names, total)
	}
	sum := getAppsSummary(t, rt, root, "")
	if sum.ByEnvironment["prod"] != 1 || sum.ByEnvironment["staging"] != 1 {
		t.Fatalf("by_environment = %+v", sum.ByEnvironment)
	}
}

func TestHandleListNodes_ResourceCount(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")
	seedNode(t, db, "node_b", "beta")
	rt.SetLocalNodeID("node_b")
	seedSearchApps(t, db, "on-a", "unplaced")
	if err := db.UpdateServiceNode(context.Background(), "on-a", "node_a"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes", ""))
	var got []nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	counts := map[string]int{}
	for _, n := range got {
		counts[n.ID] = n.ResourceCount
	}
	if counts["node_a"] != 1 || counts["node_b"] != 1 {
		t.Fatalf("resource counts = %v, want node_a=1 node_b=1 (unplaced app counts for local)", counts)
	}
}
