package main

import (
	"github.com/GLINCKER/levelrail/internal/defparse"
	"strings"
	"testing"
)

func TestInferUpDefinitionFile(t *testing.T) {
	const remote = "https://example.com/a/b.git"
	def := `{"schemaVersion":2,"dockerfilePath":"docker/App.Dockerfile"}`
	tests := []struct {
		name       string
		files      map[string]string
		flags      upFlags
		wantDocker string
		wantSrc    string
		wantErr    string
	}{
		{name: "path wins over detection", files: map[string]string{defparse.FileName: def, "docker/App.Dockerfile": "FROM scratch\n"}, flags: upFlags{port: 8080}, wantDocker: "docker/App.Dockerfile", wantSrc: srcDefFile},
		{name: "explicit dockerfile wins", files: map[string]string{defparse.FileName: def, "go.mod": "module x\n"}, flags: upFlags{dockerfile: "Other"}, wantDocker: "Other", wantSrc: srcFlag},
		{name: "missing file", files: map[string]string{defparse.FileName: def}, flags: upFlags{port: 8080}, wantErr: "is not a file"},
		{name: "lines refused", files: map[string]string{defparse.FileName: `{"schemaVersion":2,"dockerfileLines":["FROM alpine"]}`}, wantErr: "never writes a Dockerfile"},
		{name: "traversal", files: map[string]string{defparse.FileName: `{"schemaVersion":2,"dockerfilePath":"../x"}`}, wantErr: "inside the project"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			initRepo(t, root, remote)
			writeTree(t, root, tc.files)
			inf, err := inferUp(root, tc.flags, detectGitAt(root))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("inferUp: %v", err)
			}
			if inf.Flags.dockerfile != tc.wantDocker || inf.Sources["dockerfile"] != tc.wantSrc {
				t.Errorf("dockerfile = %q (%s), want %q (%s)", inf.Flags.dockerfile, inf.Sources["dockerfile"], tc.wantDocker, tc.wantSrc)
			}
		})
	}
}
