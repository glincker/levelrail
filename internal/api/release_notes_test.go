package api

import (
	"context"
	"github.com/GLINCKER/levelrail/internal/store"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/repolayout"
)

func TestParseConventionalSubject(t *testing.T) {
	tests := []struct {
		in, typ, scope, summary string
		breaking                bool
	}{
		{"feat: add x", "feat", "", "add x", false},
		{"fix(api): handle nil", "fix", "api", "handle nil", false},
		{"feat(web)!: drop old route", "feat", "web", "drop old route", true},
		{"Merge branch main", "", "", "Merge branch main", false},
		{"Update README: typo", "", "", "Update README: typo", false},
	}
	for _, tt := range tests {
		typ, scope, summary, breaking := parseConventionalSubject(tt.in)
		if typ != tt.typ || scope != tt.scope || summary != tt.summary || breaking != tt.breaking {
			t.Errorf("%q = %q %q %q %v", tt.in, typ, scope, summary, breaking)
		}
	}
}

func TestGroupReleaseNotes(t *testing.T) {
	commits := []repolayout.Commit{
		{SHA: "aaaaaaa1", Message: "fix(api): crash on empty body"},
		{SHA: "bbbbbbb2", Message: "feat: add promotion"},
		{SHA: "ccccccc3", Message: "chore: bump deps"},
		{SHA: "ddddddd4", Message: "refactor: split file\n\nBREAKING CHANGE: renamed flag"},
		{SHA: "eeeeeee5", Message: "feat: another"},
	}
	groups := groupReleaseNotes(commits)
	var keys []string
	for _, g := range groups {
		keys = append(keys, g.Key)
	}
	if want := []string{"breaking", "feat", "fix", "other"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("group keys = %v, want %v", keys, want)
	}
	if len(groups[1].Entries) != 2 || groups[1].Entries[0].Summary != "add promotion" {
		t.Errorf("feat group = %+v", groups[1])
	}
	if groups[2].Entries[0].Scope != "api" {
		t.Errorf("fix scope = %q", groups[2].Entries[0].Scope)
	}
	if got := groupReleaseNotes(nil); len(got) != 0 {
		t.Errorf("empty input gave %v", got)
	}
}

func TestCommitsBetween(t *testing.T) {
	list := []repolayout.Commit{{SHA: "eeeeeee5"}, {SHA: "ddddddd4"}, {SHA: "ccccccc3"}, {SHA: "bbbbbbb2"}}
	tests := []struct {
		name     string
		from, to string
		want     []string
		ok       bool
	}{
		{"range excludes from", "bbbbbbb2", "ddddddd4", []string{"ddddddd4", "ccccccc3"}, true},
		{"prefix match", "bbbbbbb", "eeeeeee", []string{"eeeeeee5", "ddddddd4", "ccccccc3"}, true},
		{"reversed is empty", "eeeeeee5", "bbbbbbb2", nil, true},
		{"outside window", "9999999", "eeeeeee5", nil, false},
	}
	for _, tt := range tests {
		got, ok := commitsBetween(list, tt.from, tt.to)
		var shas []string
		for _, c := range got {
			shas = append(shas, c.SHA)
		}
		if ok != tt.ok || !reflect.DeepEqual(shas, tt.want) {
			t.Errorf("%s: got %v ok=%v", tt.name, shas, ok)
		}
	}
}

func TestRenderReleaseNotesMarkdown(t *testing.T) {
	g := groupReleaseNotes([]repolayout.Commit{{SHA: "abcdef123", Message: "fix(api): x"}})
	md := renderReleaseNotesMarkdown("web", "aaa", "bbb", g)
	for _, want := range []string{"## web: aaa to bbb", "### Fixes", "- **api**: x (abcdef1)"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if !strings.Contains(renderReleaseNotesMarkdown("web", "a", "b", nil), "No commits") {
		t.Error("empty case missing")
	}
}

func TestDiffDeployCompareSidesDigest(t *testing.T) {
	got := diffDeployCompareSides(deployCompareSide{Image: "a", ImageDigest: "sha256:1"}, deployCompareSide{Image: "a", ImageDigest: "sha256:2"})
	if len(got) != 1 || got[0].Field != "image_digest" {
		t.Fatalf("changes = %+v", got)
	}
}

func seedNotesApp(t *testing.T, db *store.DB, envKind string) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_n", Name: "n", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_n", ProjectID: "proj_n", Name: "e", Kind: envKind, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "x:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env_n"); err != nil {
		t.Fatal(err)
	}
	for i, sha := range []string{"aaaaaaa1111", "bbbbbbb2222"} {
		a := store.DeployAttempt{
			ID: "dep_" + sha[:3], ServiceName: "web", Image: "x", CommitSHA: sha,
			Source: store.DeployAttemptSourceGitPush, Status: store.DeployAttemptStatusSucceeded,
			StartedAt: time.Now().Add(time.Duration(i) * time.Minute),
		}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
}

func fixedCommits(context.Context, string, string, string) ([]repolayout.Commit, string) {
	return []repolayout.Commit{{SHA: "bbbbbbb2222", Message: "feat: add thing"}}, ""
}

func TestReleaseNotesForDeploy(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		source  string
		lister  func(context.Context, string, string, string) ([]repolayout.Commit, string)
		wantSub string
	}{
		{"production", store.EnvironmentKindProduction, "", fixedCommits, "add thing"},
		{"promotion in non-production", "uat", store.DeployAttemptSourcePromote, fixedCommits, "add thing"},
		{"non-production", "uat", "", fixedCommits, ""},
		{"generator reports a note", store.EnvironmentKindProduction, "", func(context.Context, string, string, string) ([]repolayout.Commit, string) {
			return nil, "outside window"
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			seedNotesApp(t, db, tt.kind)
			rt.releaseCommits = tt.lister
			got := rt.releaseNotesForDeploy(context.Background(), "web", tt.source)
			if (tt.wantSub == "") != (got == "") || !strings.Contains(got, tt.wantSub) {
				t.Fatalf("notes = %q, want containing %q", got, tt.wantSub)
			}
		})
	}
}

func TestReleaseNotesForDeploy_GeneratorPanicIsContained(t *testing.T) {
	rt, db := newTestRouter(t)
	seedNotesApp(t, db, store.EnvironmentKindProduction)
	rt.releaseCommits = func(context.Context, string, string, string) ([]repolayout.Commit, string) { panic("boom") }
	if got := rt.releaseNotesForDeploy(context.Background(), "web", ""); got != "" {
		t.Fatalf("notes = %q, want empty", got)
	}
}
