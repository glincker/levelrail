package changelog

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Entry
	}{
		{
			name: "linked heading with sections and reference links",
			in: `# Changelog

## [0.2.0-beta.14](https://github.com/glincker/levelrail/compare/v0.2.0-beta.13...v0.2.0-beta.14) (2026-09-23)


### Bug Fixes

* resolve migration collision ([#659](https://github.com/glincker/levelrail/issues/659)) ([25e9ea5](https://github.com/glincker/levelrail/commit/25e9ea5))

## [0.2.0-beta.13](https://github.com/glincker/levelrail/compare/v0.2.0-beta.12...v0.2.0-beta.13) (2026-09-23)


### Features

* branch-scoped env var overrides ([#632](https://github.com/glincker/levelrail/issues/632)) ([2c9930b](https://github.com/glincker/levelrail/commit/2c9930b))
* per-app integrations catalog ([#647](https://github.com/glincker/levelrail/issues/647)) ([ae6ffdd](https://github.com/glincker/levelrail/commit/ae6ffdd))
`,
			want: []Entry{
				{Version: "0.2.0-beta.14", Date: "2026-09-23", Bullets: []string{"resolve migration collision"}},
				{Version: "0.2.0-beta.13", Date: "2026-09-23", Bullets: []string{"branch-scoped env var overrides", "per-app integrations catalog"}},
			},
		},
		{
			name: "plain heading with no link",
			in: `## 0.1.0 (2026-09-09)

### Features

* initial release
`,
			want: []Entry{
				{Version: "0.1.0", Date: "2026-09-09", Bullets: []string{"initial release"}},
			},
		},
		{
			name: "stale unreleased preamble at the tail is not attributed to the prior release",
			in: `## 0.1.0 (2026-09-09)

### Features

* initial release

## Changelog

All notable changes are documented here.

## Unreleased

### Added

- this should never land on 0.1.0's bullets
`,
			want: []Entry{
				{Version: "0.1.0", Date: "2026-09-09", Bullets: []string{"initial release"}},
			},
		},
		{
			name: "empty input",
			in:   "",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestLatest(t *testing.T) {
	entries := []Entry{{Version: "3"}, {Version: "2"}, {Version: "1"}}

	tests := []struct {
		name string
		n    int
		want int
	}{
		{"zero means all", 0, 3},
		{"negative means all", -1, 3},
		{"cap below length", 2, 2},
		{"cap at length", 3, 3},
		{"cap above length", 10, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Latest(entries, tt.n)
			if len(got) != tt.want {
				t.Errorf("Latest(n=%d) returned %d entries, want %d", tt.n, len(got), tt.want)
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(realPath, []byte("## 0.1.0 (2026-09-09)\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("skips empty and missing candidates", func(t *testing.T) {
		content, found, err := ReadFile("", filepath.Join(dir, "missing.md"), realPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found {
			t.Fatal("want found=true")
		}
		if content != "## 0.1.0 (2026-09-09)\n" {
			t.Errorf("unexpected content: %q", content)
		}
	})

	t.Run("none exist", func(t *testing.T) {
		_, found, err := ReadFile(filepath.Join(dir, "missing.md"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("want found=false")
		}
	})

	t.Run("real error is not swallowed", func(t *testing.T) {
		// A path under a non-existent parent directory fails with
		// ENOTDIR/ENOENT in a way os.IsNotExist still reports true for,
		// so use a directory itself to force a non-not-exist read error.
		_, _, err := ReadFile(dir)
		if err == nil {
			t.Fatal("want an error reading a directory as a file")
		}
	})
}
