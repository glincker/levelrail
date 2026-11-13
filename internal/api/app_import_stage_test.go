package api

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestImportBuildPaths(t *testing.T) {
	cases := []struct {
		name, buildType, base, path, wantBase, wantPath string
		warn                                            bool
	}{
		{"railpack path becomes base", spec.BuildRailpack, "", "/apps/api/", "apps/api", "", false},
		{"railpack keeps base", spec.BuildRailpack, "apps/api", "apps/api", "apps/api", "", false},
		{"dockerfile under base", spec.BuildDockerfile, "/apps/web", "apps/web/Dockerfile", "apps/web", "apps/web/Dockerfile", false},
		{"dockerfile at root", spec.BuildDockerfile, "", "Dockerfile", "", "Dockerfile", false},
		{"static path equals base", spec.BuildStatic, "site", "site", "", "site", false},
		{"dockerfile outside base", spec.BuildDockerfile, "apps/web", "Dockerfile", "", "Dockerfile", true},
		{"escaping base", spec.BuildRailpack, "../../etc", "", "", "", true},
	}
	for _, c := range cases {
		base, path, warn := importBuildPaths(c.buildType, c.base, c.path)
		if base != c.wantBase || path != c.wantPath || (warn != "") != c.warn {
			t.Errorf("%s: got %q %q warn=%q", c.name, base, path, warn)
		}
	}
}

func TestImportConnectRepositoryMapsSSHAndKeepsBuildSettings(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	rt.gitSources = db
	seedApp(t, db, "api")
	a := &importApplier{rt: rt}
	warns := a.connectRepository(t.Context(), platformimport.AppPlan{
		Name: "api", GitURL: "git@github.com:acme/api.git", GitBranch: "release",
		BuildMethod: "railpack", BuildPath: "services/api",
	})
	gs, err := db.GetGitSource(t.Context(), "api")
	if err != nil {
		t.Fatalf("git source not saved: %v (warnings %v)", err, warns)
	}
	if gs.RepoURL != "https://github.com/acme/api" || gs.Branch != "release" || gs.BaseDirectory != "services/api" || gs.BuildPath != "" {
		t.Errorf("git source: %+v", gs)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "over ssh") {
		t.Errorf("warnings: %v", warns)
	}
}

func TestImportConnectRepositoryReportsUnmappableURL(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	rt.gitSources = db
	seedApp(t, db, "api")
	a := &importApplier{rt: rt}
	warns := a.connectRepository(t.Context(), platformimport.AppPlan{Name: "api", GitURL: "file:///srv/api.git", BuildMethod: "dockerfile"})
	if len(warns) != 1 || !strings.Contains(warns[0], "not a repository URL") {
		t.Errorf("warnings: %v", warns)
	}
	if _, err := db.GetGitSource(t.Context(), "api"); err == nil {
		t.Error("an unmappable URL was connected")
	}
}

func TestImportConnectRepositoryNotesDefaultBranch(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	rt.gitSources = db
	seedApp(t, db, "web")
	a := &importApplier{rt: rt}
	warns := a.connectRepository(t.Context(), platformimport.AppPlan{
		Name: "web", GitURL: "https://github.com/acme/web", BuildMethod: "dockerfile",
		BaseDirectory: "apps/web", BuildPath: "apps/web/Dockerfile",
	})
	gs, err := db.GetGitSource(t.Context(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if gs.Branch != "main" || gs.BaseDirectory != "apps/web" || gs.BuildPath != "apps/web/Dockerfile" {
		t.Errorf("git source: %+v", gs)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "no branch") {
		t.Errorf("warnings: %v", warns)
	}
}
