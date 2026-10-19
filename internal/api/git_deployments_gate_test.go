package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBeginForgeDeploymentIsOffWithoutItsPrerequisites(t *testing.T) {
	_, rt, rows, _ := newDeploymentFixture(t, func(*http.Request) (int, map[string]string, string) {
		return 201, nil, `{"id":1}`
	})
	tests := []struct {
		name string
		gs   store.GitSource
		sha  string
		off  bool
	}{
		{"status reporting disabled on the source", store.GitSource{RepoURL: "https://github.com/acme/web", ReportStatus: false}, "sha1", true},
		{"no commit to report", store.GitSource{RepoURL: "https://github.com/acme/web", ReportStatus: true}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := rt.beginForgeDeployment(context.Background(), "web", tc.gs, tc.sha, forgeEnvProduction, forgeEnvProduction, "")
			if tc.off && d != nil {
				t.Fatal("a deployment was created although reporting is off")
			}
			if len(rows.rows) != 0 {
				t.Errorf("rows = %+v, want none", rows.rows)
			}
		})
	}
}

func TestBeginForgeDeploymentWithoutAStoreIsOff(t *testing.T) {
	rt := &Router{}
	if d := rt.beginForgeDeployment(context.Background(), "web", store.GitSource{ReportStatus: true}, "sha1", forgeEnvProduction, forgeEnvProduction, ""); d != nil {
		t.Fatal("a deployment was created without a store")
	}
}

func TestPreviewScopeKeysByPullRequest(t *testing.T) {
	if previewScope(3) == previewScope(4) {
		t.Fatal("two pull requests share a preview scope")
	}
	if !strings.HasPrefix(previewScope(3), forgeEnvPreview+":") {
		t.Errorf("previewScope(3) = %q, want the preview prefix", previewScope(3))
	}
}

func TestNewForgeDeploymentIDIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := newForgeDeploymentID()
		if !strings.HasPrefix(id, "fdep_") || seen[id] {
			t.Fatalf("id %q is malformed or repeated", id)
		}
		seen[id] = true
	}
}
