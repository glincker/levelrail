package main

import (
	"reflect"
	"testing"
)

func TestParseCommit(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want Change
	}{
		{"feat with pr", "feat: setup wizard (#587)", Change{Type: "feat", Subject: "setup wizard", PR: 587}},
		{"scoped fix", "fix(ingress): renew certs (#12)\n\nbody", Change{Type: "fix", Scope: "ingress", Subject: "renew certs", PR: 12}},
		{"bang breaking", "feat(api)!: drop v0 routes (#9)", Change{Type: "feat", Scope: "api", Subject: "drop v0 routes", PR: 9, Breaking: true}},
		{"footer breaking", "refactor: rename flag\n\nBREAKING CHANGE: use --data-dir instead\nof --dir.", Change{Type: "refactor", Subject: "rename flag", Breaking: true, BreakingNote: "use --data-dir instead of --dir."}},
		{"non conventional", "Merge branch x", Change{Type: "other", Subject: "Merge branch x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCommit("", tt.msg)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestExtractReleaseNote(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantNote string
		wantNone bool
	}{
		{"section", "## What changed\nx\n\n## Release note\n\n<!-- hint -->\nDeploys now retry on registry timeouts.\n\n## Test plan\n- [ ] y", "Deploys now retry on registry timeouts.", false},
		{"none", "## Release note\n\nNONE\n", "", true},
		{"fenced", "text\n```release-note\nAdds Gitea support.\n```\n", "Adds Gitea support.", false},
		{"only comment", "## Release note\n\n<!-- write one sentence -->\n\n## Test plan", "", false},
		{"missing", "## What changed\nstuff", "", false},
		{"crlf", "## Release note\r\n\r\nFaster builds.\r\n", "Faster builds.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note, none := ExtractReleaseNote(tt.body)
			if note != tt.wantNote || none != tt.wantNone {
				t.Fatalf("got (%q, %v), want (%q, %v)", note, none, tt.wantNote, tt.wantNone)
			}
		})
	}
}

func TestApplyPR(t *testing.T) {
	c := ParseCommit("abc", "feat: add thing (#5)")
	c.ApplyPR("## Release note\n\nYou can now add things.\n\nBREAKING CHANGE: old things are removed.", "alice", []string{"highlight"})
	if c.Text() != "You can now add things" {
		t.Fatalf("text override: got %q", c.Text())
	}
	if !c.Breaking || c.BreakingNote != "old things are removed." {
		t.Fatalf("breaking: got %v %q", c.Breaking, c.BreakingNote)
	}
	if c.Author != "alice" || !c.HasLabel("HIGHLIGHT") {
		t.Fatalf("author/labels: %+v", c)
	}
}

func TestGroup(t *testing.T) {
	changes := []Change{
		{Type: "fix", Subject: "bump dependencies for security advisories"},
		{Type: "feat", Subject: "a"},
		{Type: "chore", Subject: "release 0.2.0"},
		{Type: "ci", Subject: "cache"},
		{Type: "feat", Subject: "internal", NoteNone: true},
		{Type: "fix", Subject: "b"},
		{Type: "docs", Subject: "c"},
		{Type: "perf", Subject: "d"},
	}
	sections, maint := Group(changes)
	var titles []string
	for _, s := range sections {
		titles = append(titles, s.Title)
	}
	want := []string{"Features", "Security", "Bug fixes", "Performance", "Documentation"}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("sections: got %v, want %v", titles, want)
	}
	if len(maint) != 2 || maint[0].Subject != "cache" || maint[1].Subject != "internal" {
		t.Fatalf("maintenance: got %+v", maint)
	}
}

func TestHighlights(t *testing.T) {
	feats := []Change{{Type: "feat", Subject: "1"}, {Type: "feat", Subject: "2"}, {Type: "fix", Subject: "x"}, {Type: "feat", Subject: "3"}, {Type: "feat", Subject: "4"}}
	if got := Highlights(feats, 3); len(got) != 3 || got[2].Subject != "3" {
		t.Fatalf("fallback: got %+v", got)
	}
	labeled := append(feats, Change{Type: "fix", Subject: "big fix", Labels: []string{"highlight"}})
	if got := Highlights(labeled, 3); len(got) != 1 || got[0].Subject != "big fix" {
		t.Fatalf("labeled: got %+v", got)
	}
}
