package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefinitionDockerfile(t *testing.T) {
	write := func(root, rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	if got := definitionDockerfile(root, root); got != "" {
		t.Fatalf("no file: got %q", got)
	}
	write(root, "docker/App.Dockerfile", "FROM scratch\n")
	write(root, "levelrail-definition.json", `{"schemaVersion":2,"dockerfilePath":"docker/App.Dockerfile"}`)
	if got, want := definitionDockerfile(root, root), filepath.Join(root, "docker", "App.Dockerfile"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	write(root, "levelrail-definition.json", `{"schemaVersion":2,"dockerfilePath":"missing"}`)
	if got := definitionDockerfile(root, root); got != "" {
		t.Fatalf("missing target: got %q", got)
	}
	write(root, "levelrail-definition.json", `{"schemaVersion":2,"dockerfilePath":"../x"}`)
	if got := definitionDockerfile(root, root); got != "" {
		t.Fatalf("escape: got %q", got)
	}
	write(root, "levelrail-definition.json", `{"schemaVersion":2,"imageName":"nginx"}`)
	if got := definitionDockerfile(root, root); got != "" {
		t.Fatalf("imageName: got %q", got)
	}
}
