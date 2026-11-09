package deploy

import (
	"strings"

	"github.com/GLINCKER/levelrail/internal/defparse"
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

func TestWebhookDefinitionIgnoredNote(t *testing.T) {
	root := t.TempDir()
	if got := definitionIgnoredNote(root, root); got != "" {
		t.Fatalf("no file: want no note, got %q", got)
	}
	file := filepath.Join(root, defparse.FileName)
	if err := os.WriteFile(file, []byte(`{"schemaVersion":2,"dockerfilePath":"a.Dockerfile"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := definitionIgnoredNote(root, root); got != "" {
		t.Fatalf("plain file: want no note, got %q", got)
	}
	if err := os.WriteFile(file, []byte(`{"schemaVersion":2,"imageName":"nginx:1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := definitionIgnoredNote(root, root); !strings.Contains(got, "imageName") {
		t.Fatalf("imageName: want note, got %q", got)
	}
	if err := os.WriteFile(file, []byte(`{"schemaVersion":2,"dockerfileLines":["FROM scratch"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := definitionIgnoredNote(root, root); !strings.Contains(got, "dockerfileLines") {
		t.Fatalf("dockerfileLines: want note, got %q", got)
	}
}
