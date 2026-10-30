package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

func TestResolveBuildRoot(t *testing.T) {
	tests := []struct {
		name          string
		sourceDir     string
		baseDirectory string
		want          string
		wantErr       bool
	}{
		{
			name:      "empty base directory returns source dir unchanged",
			sourceDir: "/tmp/checkout",
			want:      "/tmp/checkout",
		},
		{
			name:          "simple subdirectory",
			sourceDir:     "/tmp/checkout",
			baseDirectory: "apps/web",
			want:          "/tmp/checkout/apps/web",
		},
		{
			name:          "leading dot slash",
			sourceDir:     "/tmp/checkout",
			baseDirectory: "./apps/web",
			want:          "/tmp/checkout/apps/web",
		},
		{
			name:          "trailing slash",
			sourceDir:     "/tmp/checkout",
			baseDirectory: "apps/web/",
			want:          "/tmp/checkout/apps/web",
		},
		{
			name:          "single dot means source dir itself",
			sourceDir:     "/tmp/checkout",
			baseDirectory: ".",
			want:          "/tmp/checkout",
		},
		{
			name:          "simple traversal rejected",
			sourceDir:     "/tmp/checkout",
			baseDirectory: "../escape",
			wantErr:       true,
		},
		{
			name:          "traversal past root rejected",
			sourceDir:     "/tmp/checkout",
			baseDirectory: "..",
			wantErr:       true,
		},
		{
			name:          "traversal that cleans back inside is still rejected at the string level but resolves outside",
			sourceDir:     "/tmp/checkout",
			baseDirectory: "apps/../../escape",
			wantErr:       true,
		},
		{
			name:          "nested traversal that cancels out stays inside",
			sourceDir:     "/tmp/checkout",
			baseDirectory: "apps/../web",
			want:          "/tmp/checkout/web",
		},
		{
			name:          "absolute path rejected",
			sourceDir:     "/tmp/checkout",
			baseDirectory: "/etc/passwd",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveBuildRoot(tt.sourceDir, tt.baseDirectory)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveBuildRoot(%q, %q) = %q, nil; want error", tt.sourceDir, tt.baseDirectory, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveBuildRoot(%q, %q) unexpected error: %v", tt.sourceDir, tt.baseDirectory, err)
			}
			if got != tt.want {
				t.Errorf("resolveBuildRoot(%q, %q) = %q, want %q", tt.sourceDir, tt.baseDirectory, got, tt.want)
			}
		})
	}
}

func TestBuildRootSymlinkContainment(t *testing.T) {
	outside := t.TempDir()
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "apps", "web"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "leak")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "apps"), filepath.Join(src, "alias")); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveBuildRoot(src, "leak"); err == nil {
		t.Fatal("symlink leaving the checkout should be refused")
	}
	for _, ok := range []string{"apps/web", "alias/web"} {
		if _, err := resolveBuildRoot(src, ok); err != nil {
			t.Fatalf("%s: unexpected error: %v", ok, err)
		}
	}
	if err := build.EnsureWithin(src, filepath.Join(src, "leak", "Dockerfile")); err == nil {
		t.Fatal("dockerfile path through an outside symlink should be refused")
	}
}
