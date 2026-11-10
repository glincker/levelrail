package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuilderNotes(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		link  string
		want  string // substring of the second line, "" means no second line
	}{
		{name: "empty"},
		{name: "procfile only", files: []string{"Procfile"}, want: "found Procfile;"},
		{name: "buildpacks", files: []string{".buildpacks"}, want: "found .buildpacks;"},
		{name: "project toml and procfile", files: []string{"project.toml", "Procfile"}, want: "found project.toml, Procfile;"},
		{name: "symlink ignored", link: "project.toml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.link != "" {
				if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, tc.link)); err != nil {
					t.Skip("symlinks unavailable")
				}
			}
			got := BuilderNotes(dir)
			if !strings.HasPrefix(got[0], "builder: Railpack") {
				t.Fatalf("first line %q", got[0])
			}
			if tc.want == "" && len(got) != 1 {
				t.Fatalf("want one line, got %v", got)
			}
			if tc.want != "" && (len(got) != 2 || !strings.Contains(got[1], tc.want)) {
				t.Fatalf("got %v, want %q", got, tc.want)
			}
		})
	}
}

func TestDockerfileNote(t *testing.T) {
	if got := DockerfileNote("/ctx", ""); got != "builder: Dockerfile (Dockerfile)" {
		t.Fatal(got)
	}
	if got := DockerfileNote("/ctx", "/ctx/deploy/Dockerfile.prod"); got != "builder: Dockerfile (deploy/Dockerfile.prod)" {
		t.Fatal(got)
	}
}
