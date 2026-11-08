package main

import (
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"strings"
	"testing"
)

func TestDisplayRepoURL(t *testing.T) {
	cases := []struct {
		name, in, want string
		changed        bool
	}{
		{"userinfo", "https://bob:" + "s3cr3t" + "@git.example.com/a/b.git", "https://git.example.com/a/b.git", true},
		{"token query", "https://git.example.com/a/b.git?access_token=abc&ref=x", "https://git.example.com/a/b.git?ref=x", true},
		{"private token", "https://git.example.com/a/b.git?private_token=abc", "https://git.example.com/a/b.git", true},
		{"clean", "https://github.com/a/b.git", "https://github.com/a/b.git", false},
		{"scp", "git@github.com:a/b.git", "git@github.com:a/b.git", false},
		{"scp password", "bob:" + "tok" + "@host:a/b.git", "", true},
	}
	for _, c := range cases {
		got, changed := platformimport.DisplayRepoURL(c.in)
		if got != c.want || changed != c.changed {
			t.Errorf("%s: got %q,%v want %q,%v", c.name, got, changed, c.want, c.changed)
		}
	}
}

func TestDokployReportNeverHoldsCredential(t *testing.T) {
	app := dokployApplication{Name: "x", BuildType: "dockerfile", SourceType: "git", CustomGitURL: "https://bob:" + "s3cr3t" + "@git.example.com/a/b.git?token=zzz", CustomGitBranch: "main"}
	got := mapDokployApplication(app, nil, false)
	if got.RepoURL != "https://git.example.com/a/b.git" {
		t.Fatalf("RepoURL = %q", got.RepoURL)
	}
	found := false
	for _, iss := range got.Issues {
		if strings.Contains(iss.Detail, "s3cr3t") || strings.Contains(iss.Detail, "zzz") {
			t.Fatalf("issue leaks credential: %s", iss.Detail)
		}
		if strings.Contains(iss.Detail, "credential was removed") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing credential-removed issue")
	}
}
