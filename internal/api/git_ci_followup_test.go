package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestOlderDeployFinishingLateDoesNotDisplaceNewer(t *testing.T) {
	fs, rt, rows, f := newDeploymentFixture(t, nil)
	older := newTestDeployment(rt, f, "older", 1)
	newer := newTestDeployment(rt, f, "newer", 2)

	newer.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	older.finish(context.Background(), githubapp.DeploymentSuccess, "ok")

	if rows.rows["newer"].State != "success" || rows.rows["older"].State != "inactive" {
		t.Fatalf("states newer=%s older=%s, want success/inactive", rows.rows["newer"].State, rows.rows["older"].State)
	}
	for _, c := range fs.calls {
		if strings.HasSuffix(c.Path, "/deployments/2/statuses") && c.Body["state"] == "inactive" {
			t.Fatalf("the newer deployment was marked inactive: %+v", fs.calls)
		}
	}
}

func TestPreviewDeploymentsAreScopedPerPullRequest(t *testing.T) {
	_, rt, rows, f := newDeploymentFixture(t, nil)
	prA := newScopedDeployment(rt, f, "a", 1, previewScope(1))
	prA.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	prB := newScopedDeployment(rt, f, "b", 2, previewScope(2))
	prB.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	prA2 := newScopedDeployment(rt, f, "a2", 3, previewScope(1))
	prA2.finish(context.Background(), githubapp.DeploymentSuccess, "ok")

	if rows.rows["b"].State != "success" {
		t.Fatalf("PR 2's preview was superseded by another pull request: %s", rows.rows["b"].State)
	}
	if rows.rows["a"].State != "inactive" || rows.rows["a2"].State != "success" {
		t.Fatalf("PR 1 states a=%s a2=%s, want inactive/success", rows.rows["a"].State, rows.rows["a2"].State)
	}
}

func TestDeactivateForgeDeploymentsOnTeardown(t *testing.T) {
	fs, rt, rows, f := newDeploymentFixture(t, nil)
	d := newScopedDeployment(rt, f, "p1", 1, previewScope(7))
	d.finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	other := newScopedDeployment(rt, f, "p2", 2, previewScope(8))
	other.finish(context.Background(), githubapp.DeploymentSuccess, "ok")

	rt.deactivateLive(context.Background(), f, "web", previewScope(7))

	if c := fs.last(t); c.Body["state"] != "inactive" || !strings.HasSuffix(c.Path, "/deployments/1/statuses") {
		t.Fatalf("call = %+v", c)
	}
	if rows.rows["p1"].State != "inactive" {
		t.Fatalf("torn down preview stayed %s", rows.rows["p1"].State)
	}
	if rows.rows["p2"].State != "success" {
		t.Fatalf("another pull request's preview changed: %s", rows.rows["p2"].State)
	}
}

func TestDeactivateSkippedWhenReportingOff(t *testing.T) {
	fs, rt, rows, f := newDeploymentFixture(t, nil)
	newScopedDeployment(rt, f, "p1", 1, previewScope(7)).finish(context.Background(), githubapp.DeploymentSuccess, "ok")
	calls := len(fs.calls)
	rt.deactivateForgeDeployments(context.Background(), "web", store.GitSource{ReportStatus: false}, previewScope(7))
	t.Setenv(EnvGitStatusEnabled, "false")
	rt.deactivateForgeDeployments(context.Background(), "web", store.GitSource{ReportStatus: true}, previewScope(7))
	if len(fs.calls) != calls || rows.rows["p1"].State != "success" {
		t.Fatalf("deactivated with reporting off: %+v", rows.rows["p1"])
	}
}

func TestTagPushesAreNeverPathFiltered(t *testing.T) {
	rt := &Router{}
	gs := store.GitSource{DeployPaths: []string{"src/**"}}
	if msg := rt.skipPushForPaths(context.Background(), "web", gs, "refs/tags/v1", "", "abc", []string{"README.md"}); msg != "" {
		t.Fatalf("tag push skipped: %q", msg)
	}
	if msg := rt.skipPushForPaths(context.Background(), "web", gs, "refs/heads/main", "b", "abc", []string{"README.md"}); msg == "" {
		t.Fatal("branch push with no matching path was not skipped")
	}
}

func TestChangedFilesTruncatedFailsOpen(t *testing.T) {
	_, srv := newForgeServer(t, func(*http.Request) (int, map[string]string, string) {
		var b strings.Builder
		b.WriteString("[")
		for i := 0; i < 100; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"filename":"f.txt"}`)
		}
		b.WriteString("]")
		return 200, nil, b.String()
	})
	_, err := forgeFor(forgeGitHub, srv.URL).changedFiles(context.Background(), changeQuery{PR: 1})
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("err = %v, want a truncated error so filters fail open", err)
	}
}
