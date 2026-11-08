package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func expandWith(t *testing.T, root, composeYAML string) (map[string]spec.Service, error) {
	t.Helper()
	writeComposeFile(t, root, "docker-compose.yml", composeYAML)
	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yml"}}
	out, _, err := ExpandBuildService(svc, root)
	return out, err
}

func TestEnvFile_MergeOrderAndForms(t *testing.T) {
	root := t.TempDir()
	writeComposeFile(t, root, "a.env", "A=file-a\nB=file-a\n# comment\nexport C=\"quoted # kept\"\nD='single $X'\nE=bare # trailing\n")
	writeComposeFile(t, root, "b.env", "B=file-b\n")
	out, err := expandWith(t, root, `
services:
  web:
    image: nginx
    env_file:
      - ./a.env
      - path: ./b.env
      - path: ./missing.env
        required: false
    environment:
      A: inline
`)
	if err != nil {
		t.Fatalf("ExpandBuildService() error = %v", err)
	}
	env := out["web"].Env
	want := map[string]string{"A": "inline", "B": "file-b", "C": "quoted # kept", "D": "single $X", "E": "bare"}
	for k, v := range want {
		if env[k].Value != v {
			t.Errorf("env[%s] = %q, want %q", k, env[k].Value, v)
		}
	}
}

func TestEnvFile_StringForm(t *testing.T) {
	root := t.TempDir()
	writeComposeFile(t, root, "x.env", "K=v\n")
	out, err := expandWith(t, root, "services:\n  web:\n    image: nginx\n    env_file: x.env\n")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if out["web"].Env["K"].Value != "v" {
		t.Errorf("K = %q", out["web"].Env["K"].Value)
	}
}

func TestEnvFile_Rejections(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.env"), []byte("S=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		path    string
		setup   func(t *testing.T, root string)
		wantErr string
	}{
		{name: "traversal", path: "../../etc/passwd", wantErr: "inside the repository"},
		{name: "absolute", path: "/etc/passwd", wantErr: "inside the repository"},
		{name: "missing", path: "nope.env", wantErr: "file not found"},
		{name: "symlink escape", path: "link.env", wantErr: "outside the repository", setup: func(t *testing.T, root string) {
			if err := os.Symlink(filepath.Join(outside, "secret.env"), filepath.Join(root, "link.env")); err != nil {
				t.Skipf("symlink: %v", err)
			}
		}},
		{name: "bad line names line not value", path: "bad.env", wantErr: "line 1", setup: func(t *testing.T, root string) {
			writeComposeFile(t, root, "bad.env", "hunter2-not-a-pair\n")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, root)
			}
			_, err := expandWith(t, root, "services:\n  web:\n    image: nginx\n    env_file: "+tt.path+"\n")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), "env_file") || !strings.Contains(err.Error(), tt.path) {
				t.Errorf("error %q must name the key and path", err)
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Errorf("error echoed a value: %v", err)
			}
		})
	}
}

func TestEnvFile_SizeCap(t *testing.T) {
	t.Setenv(envMaxEnvFileBytes, "16")
	root := t.TempDir()
	writeComposeFile(t, root, "big.env", "KEY="+strings.Repeat("x", 64)+"\n")
	_, err := expandWith(t, root, "services:\n  web:\n    image: nginx\n    env_file: big.env\n")
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("error = %v, want size limit", err)
	}
}

func TestEnvFile_NestedComposeDirAndMagicVarRejected(t *testing.T) {
	root := t.TempDir()
	writeComposeFile(t, root, "deploy/app.env", "TOKEN=${SERVICE_PASSWORD_DB}\n")
	writeComposeFile(t, root, "deploy/compose.yml", "services:\n  web:\n    image: nginx\n    env_file: app.env\n")
	svc := spec.Service{Build: spec.Build{Type: spec.BuildCompose, Path: "deploy/compose.yml"}}
	_, _, err := ExpandBuildService(svc, root)
	if err == nil || !strings.Contains(err.Error(), "magic var") {
		t.Fatalf("error = %v, want magic var rejection from the shared env path", err)
	}
	if strings.Contains(err.Error(), "hunter") {
		t.Errorf("leaked value: %v", err)
	}
}

func TestEnvFile_PastedFileKeepsWarning(t *testing.T) {
	f, err := Parse([]byte("services:\n  web:\n    image: nginx\n    env_file: a.env\n"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range f.Unsupported {
		if u.Service == "web" && u.Key == "env_file" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected env_file warning without a checkout")
	}
}

func TestExtends_SameFile(t *testing.T) {
	f, err := Parse([]byte(`
services:
  base:
    image: app:1
    command: ["run"]
    environment:
      - A=base
      - B=base
    labels: {team: x}
    ports: ["80"]
    depends_on: [db]
  db:
    image: pg
  child:
    extends:
      service: base
    command: ["other"]
    environment:
      B: child
    ports: ["81"]
`))
	if err != nil {
		t.Fatal(err)
	}
	c := f.Services["child"]
	if c.Image != "app:1" {
		t.Errorf("image = %q", c.Image)
	}
	if got := strings.Join(c.Command, " "); got != "other" {
		t.Errorf("command = %q, want replaced", got)
	}
	if c.Environment["A"] != "base" || c.Environment["B"] != "child" {
		t.Errorf("env = %v", c.Environment)
	}
	if len(c.Ports) != 2 {
		t.Errorf("ports = %v, want concatenated", c.Ports)
	}
	if len(c.DependsOn) != 0 {
		t.Errorf("depends_on must not be inherited: %v", c.DependsOn)
	}
	for _, u := range f.Unsupported {
		if u.Key == "extends" {
			t.Errorf("resolved extends still warned: %v", u)
		}
	}
}

func TestExtends_Errors(t *testing.T) {
	tests := []struct{ name, doc, want string }{
		{"missing parent", "services:\n  a:\n    image: x\n    extends: {service: nope}\n", "not defined"},
		{"cycle", "services:\n  a:\n    extends: {service: b}\n  b:\n    extends: {service: a}\n", "cycle"},
		{"no service", "services:\n  a:\n    image: x\n    extends: {}\n", "needs a service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.doc))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestExtends_OtherFileKeepsWarning(t *testing.T) {
	f, err := Parse([]byte("services:\n  a:\n    image: x\n    extends: {file: other.yml, service: b}\n"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range f.Unsupported {
		if u.Service == "a" && u.Key == "extends" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected extends warning for file: form")
	}
}

func TestIgnoredKeyAlternativesNameTheReplacement(t *testing.T) {
	if !strings.Contains(alternatives["container_name"], "service key") {
		t.Errorf("container_name alternative = %q", alternatives["container_name"])
	}
}
