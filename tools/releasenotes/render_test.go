package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func sampleRelease() Release {
	return Release{
		Product: "Acme", Repo: "o/acme", Tag: "v1.2.0-beta.3", PrevTag: "v1.2.0-beta.2",
		Prerelease: true, DocsURL: "https://docs.example.com", InstallURL: "https://example.com/install.sh",
		Assets: []string{"acme-linux-amd64", "checksums.txt"},
		Images: []Image{{Name: "ghcr.io/o/acme", Digest: "sha256:abc"}},
		Changes: []Change{
			{Type: "feat", Subject: "setup wizard", PR: 7, Author: "alice", Labels: []string{"highlight"}},
			{Type: "fix", Subject: "crash", PR: 8, Author: "bob", Breaking: true, BreakingNote: "set FOO before upgrading"},
			{Type: "chore", Subject: "bump x", PR: 9, Author: "dependabot[bot]"},
		},
		Contributors:    []string{"alice", "bob"},
		NewContributors: []Contributor{{Login: "bob", PR: 8}},
	}
}

func TestRenderSections(t *testing.T) {
	out := Render(sampleRelease())
	wantInOrder := []string{
		startMarker, "## Highlights", "setup wizard ([#7](https://github.com/o/acme/pull/7)) by @alice",
		"## Upgrade notes", "set FOO before upgrading", "## What's changed", "### Features", "### Bug fixes",
		"<summary>Maintenance, CI, and dependency updates (1)</summary>", "## Install",
		"sudo env ACME_VERSION=v1.2.0-beta.3 sh", "sh -s upgrade", "image: ghcr.io/o/acme:v1.2.0-beta.3",
		"## Container images", "| `ghcr.io/o/acme` | `v1.2.0-beta.3` | `sha256:abc` |",
		"sha256sum --ignore-missing -c checksums.txt", "cosign verify ghcr.io/o/acme@sha256:abc",
		`^https://github\.com/o/acme/\.github/workflows/release\.yml@refs/(heads/main|tags/v.+)$`,
		"@bob made their first contribution", "Thanks to @alice, @bob.",
		"compare/v1.2.0-beta.2...v1.2.0-beta.3", "https://docs.example.com/changelog/v1-2-0-beta-3", endMarker,
	}
	pos := 0
	for _, s := range wantInOrder {
		i := strings.Index(out[pos:], s)
		if i < 0 {
			t.Fatalf("missing (or out of order) %q in:\n%s", s, out)
		}
		pos += i + len(s)
	}
	if strings.Contains(out, "@dependabot") {
		t.Fatal("bot author should not be credited")
	}
}

func TestRenderSkipsRedundantHighlights(t *testing.T) {
	r := sampleRelease()
	r.Changes[0].Labels = nil
	if out := Render(r); strings.Contains(out, "## Highlights") {
		t.Fatalf("a single feature should not be repeated as a highlight:\n%s", out)
	}
}

func TestRenderNoAssets(t *testing.T) {
	r := sampleRelease()
	r.Assets, r.Images, r.NextTag = nil, nil, "v1.2.0-beta.4"
	out := Render(r)
	if !strings.Contains(out, "shipped no binaries") || !strings.Contains(out, "releases/tag/v1.2.0-beta.4") {
		t.Fatalf("expected no-assets caution pointing at next release:\n%s", out)
	}
	for _, s := range []string{"## Install", "## Verify", "## Container images"} {
		if strings.Contains(out, s) {
			t.Fatalf("unexpected %q for a release without assets", s)
		}
	}
}

func TestImageTags(t *testing.T) {
	tests := []struct {
		tag  string
		pre  bool
		want []string
	}{
		{"v0.2.0-beta.7", true, []string{"v0.2.0-beta.7", "beta"}},
		{"v1.4.2", false, []string{"v1.4.2", "v1.4", "latest"}},
		{"v1.4.2-rc.1", false, []string{"v1.4.2-rc.1", "beta"}},
	}
	for _, tt := range tests {
		if got := ImageTags(tt.tag, tt.pre); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ImageTags(%q): got %v, want %v", tt.tag, got, tt.want)
		}
	}
}

func TestSplice(t *testing.T) {
	gen1 := startMarker + "\none\n" + endMarker + "\n"
	gen2 := startMarker + "\ntwo\n" + endMarker + "\n"

	if got := Splice("## [1.0](x)\n* release-please body", gen1); got != gen1 {
		t.Fatalf("no markers should replace whole body, got %q", got)
	}
	edited := "Manual intro.\n\n" + gen1 + "Manual outro.\n"
	want := "Manual intro.\n\n" + gen2 + "Manual outro.\n"
	got := Splice(edited, gen2)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if again := Splice(got, gen2); again != got {
		t.Fatalf("not idempotent: %q", again)
	}
}

func TestSlug(t *testing.T) {
	if got := Slug("v0.2.0-beta.7"); got != "v0-2-0-beta-7" {
		t.Fatalf("got %q", got)
	}
}

func TestParseNewContributors(t *testing.T) {
	body := "## New Contributors\n* @carol made their first contribution in https://github.com/o/r/pull/42\n"
	got := ParseNewContributors(body)
	if len(got) != 1 || got[0] != (Contributor{Login: "carol", PR: 42}) {
		t.Fatalf("got %+v", got)
	}
}

func TestDigestAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_, _ = w.Write([]byte(`{"token":"t"}`))
		case r.Header.Get("Authorization") != "Bearer t":
			w.WriteHeader(http.StatusUnauthorized)
		case strings.HasSuffix(r.URL.Path, "/manifests/v1"):
			w.Header().Set("Docker-Content-Digest", "sha256:ff")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if d, err := digestAt(ctx, srv.Client(), srv.URL, "o/acme", "v1"); err != nil || d != "sha256:ff" {
		t.Fatalf("got %q, %v", d, err)
	}
	if d, err := digestAt(ctx, srv.Client(), srv.URL, "o/acme", "v2"); err != nil || d != "" {
		t.Fatalf("missing tag: got %q, %v", d, err)
	}
}
