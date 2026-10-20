package main

import (
	"strings"
	"testing"
)

const testPolicy = `
version: 1
mode: enforce
deny_paths: [".github/workflows/**", ".github/glinr-bot.yml", "internal/secrets/**"]
block_labels: [hold]
rules:
  - name: dependency-patch-minor
    authors: ["dependabot[bot]"]
    update_types: [patch, minor]
    max_changed_lines: 400
    actions: [approve, automerge]
  - name: docs-only
    authors: [gdsks]
    paths_only: ["docs/**", "*.md"]
    max_changed_lines: 600
    actions: [approve, automerge]
  - name: release
    authors: ["glinr-bot[bot]"]
    title_prefix: "chore: release"
    actions: [comment]
`

func mustPolicy(t *testing.T) Policy {
	t.Helper()
	p, err := ParsePolicy([]byte(testPolicy))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestEvaluate(t *testing.T) {
	p := mustPolicy(t)
	tests := []struct {
		name     string
		pr       PR
		wantRule string
		wantActs string
	}{
		{"dependabot patch", PR{Author: "dependabot[bot]", Title: "build(deps): bump foo from 1.2.3 to 1.2.4", Files: []string{"go.mod"}, ChangedLines: 4}, "dependency-patch-minor", "approve,automerge"},
		{"dependabot minor", PR{Author: "dependabot[bot]", Title: "bump foo from 1.2.3 to 1.3.0", Files: []string{"go.mod"}, ChangedLines: 4}, "dependency-patch-minor", "approve,automerge"},
		{"dependabot major needs a person", PR{Author: "dependabot[bot]", Title: "bump foo from 1.9.0 to 2.0.0", Files: []string{"go.mod"}, ChangedLines: 4}, "", "comment"},
		{"dependabot touching a workflow", PR{Author: "dependabot[bot]", Title: "bump a from 1.0.0 to 1.0.1", Files: []string{".github/workflows/ci.yml"}, ChangedLines: 2}, "", "comment"},
		{"docs only by owner", PR{Author: "gdsks", Title: "docs: x", Files: []string{"docs/a.md", "README.md"}, ChangedLines: 30}, "docs-only", "approve,automerge"},
		{"docs plus code", PR{Author: "gdsks", Title: "docs: x", Files: []string{"docs/a.md", "main.go"}, ChangedLines: 30}, "", "comment"},
		{"docs but too large", PR{Author: "gdsks", Title: "docs: x", Files: []string{"docs/a.md"}, ChangedLines: 5000}, "", "comment"},
		{"draft never acts", PR{Author: "gdsks", Title: "docs: x", Draft: true, Files: []string{"docs/a.md"}, ChangedLines: 3}, "", "comment"},
		{"fork never acts", PR{Author: "dependabot[bot]", Title: "bump a from 1.0.0 to 1.0.1", Fork: true, Files: []string{"go.mod"}, ChangedLines: 2}, "", "comment"},
		{"hold label", PR{Author: "gdsks", Title: "docs: x", Labels: []string{"hold"}, Files: []string{"docs/a.md"}, ChangedLines: 3}, "", "comment"},
		{"release PR is comment only", PR{Author: "glinr-bot[bot]", Title: "chore: release main", Files: []string{"CHANGELOG.md"}, ChangedLines: 40}, "release", "comment"},
		{"unknown author", PR{Author: "stranger", Title: "docs: x", Files: []string{"docs/a.md"}, ChangedLines: 3}, "", "comment"},
		{"empty file list never matches paths_only", PR{Author: "gdsks", Title: "docs: x", ChangedLines: 0}, "", "comment"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := Evaluate(p, tc.pr)
			if v.Rule != tc.wantRule {
				t.Errorf("rule = %q, want %q (%s)", v.Rule, tc.wantRule, v.Reason)
			}
			if got := strings.Join(v.Actions, ","); got != tc.wantActs {
				t.Errorf("actions = %q, want %q", got, tc.wantActs)
			}
		})
	}
}

func TestBumpType(t *testing.T) {
	tests := map[string]string{
		"bump x from 1.2.3 to 1.2.4":    "patch",
		"bump x from 1.2.3 to 1.3.0":    "minor",
		"bump x from 1.2.3 to 2.0.0":    "major",
		"Bump x from v0.9.1 to v0.10.0": "minor",
		"bump x from 3 to 4":            "major",
		"no versions here":              "",
	}
	for title, want := range tests {
		if got := bumpType(title); got != want {
			t.Errorf("bumpType(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"docs/**", "docs/a/b.md", true},
		{"docs/**", "docs", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/a.md", false},
		{".github/workflows/**", ".github/workflows/ci.yml", true},
		{"**/*.go", "a/b/c.go", true},
		{"**/*.go", "c.go", true},
		{"internal/secrets/**", "internal/secretsx/a.go", false},
	}
	for _, tc := range tests {
		if got := globMatch(tc.pattern, tc.name); got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestParsePolicyRejectsBadInput(t *testing.T) {
	for name, in := range map[string]string{
		"wrong version": "version: 2\n",
		"bad mode":      "version: 1\nmode: yolo\n",
		"bad action":    "version: 1\nrules:\n  - name: a\n    actions: [delete]\n",
		"unnamed rule":  "version: 1\nrules:\n  - actions: [comment]\n",
		"not yaml":      ":\t:",
	} {
		if _, err := ParsePolicy([]byte(in)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestRenderCommentShadowChangesNothing(t *testing.T) {
	v := Verdict{Rule: "docs-only", Actions: []string{ActionApprove, ActionAutomerge}, Gates: []Gate{{"author is gdsks", true, "gdsks"}}, Reason: "All gates pass."}
	got := renderComment(v, ModeShadow, nil)
	for _, want := range []string{commentMarker, "Shadow mode: would approve and automerge", "Nothing was changed"} {
		if !strings.Contains(got, want) {
			t.Errorf("comment missing %q:\n%s", want, got)
		}
	}
}
