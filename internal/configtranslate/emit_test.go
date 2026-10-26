package configtranslate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

var emitOpts = platformimport.EmitOptions{Stack: "shop", Project: "shop", Environment: "production", ImageRepo: "registry.example.com/acme"}

func emitCase(t *testing.T, name string, eo EmitOptions) []platformimport.EmitFile {
	t.Helper()
	in, err := Translate(loadCase(t, name)).EmitInput(eo)
	if err != nil {
		t.Fatal(err)
	}
	files, err := platformimport.EmitStack(in, emitOpts)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func validateFiles(t *testing.T, files []platformimport.EmitFile) {
	t.Helper()
	var srcs []iac.Source
	for _, f := range files {
		if strings.HasSuffix(f.Name, ".yaml") && f.Name != platformimport.EmitStackFile {
			srcs = append(srcs, iac.Source{Name: f.Name, Data: f.Content})
		}
	}
	docs, issues := iac.ParseDocuments(srcs)
	if len(issues) == 0 {
		_, issues = iac.Build(docs, iac.Options{})
	}
	if len(issues) > 0 {
		t.Fatalf("emitted documents do not validate: %v", issues)
	}
}

func TestEmitGolden(t *testing.T) {
	dbv := EmitOptions{DBVersions: map[string]string{"postgres": "16", "redis": "7.4"}}
	for _, name := range []string{"heroku-full", "heroku-minimal", "vercel-next", "vercel-static", "merge-clash"} {
		t.Run(name, func(t *testing.T) {
			files := emitCase(t, name, dbv)
			validateFiles(t, files)
			dir := filepath.Join("testdata", name, "emit")
			if update {
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range files {
				p := filepath.Join(dir, f.Name)
				if update {
					if err := os.WriteFile(p, f.Content, 0o600); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(p) //nolint:gosec // fixture path from testdata
				if err != nil {
					t.Fatalf("%v (set CONFIGTRANSLATE_UPDATE=1 to write goldens)", err)
				}
				if !bytes.Equal(want, f.Content) {
					t.Errorf("%s differs from golden:\n%s", f.Name, f.Content)
				}
			}
			again := emitCase(t, name, dbv)
			for i := range files {
				if !bytes.Equal(files[i].Content, again[i].Content) {
					t.Errorf("%s is not deterministic", files[i].Name)
				}
			}
		})
	}
}

func TestEmitKeepsBothClashingServices(t *testing.T) {
	files := emitCase(t, "merge-clash", EmitOptions{})
	var apps string
	for _, f := range files {
		if f.Name == platformimport.EmitAppsFile {
			apps = string(f.Content)
		}
	}
	for _, want := range []string{"name: web\n", "name: web-vercel\n", "name: worker\n"} {
		if !strings.Contains(apps, want) {
			t.Errorf("apps.yaml misses %q:\n%s", want, apps)
		}
	}
}

func TestEmitServiceFilterAndSecrets(t *testing.T) {
	files := emitCase(t, "heroku-full", EmitOptions{Services: []string{"web"}})
	for _, f := range files {
		if f.Name == platformimport.EmitAppsFile && strings.Contains(string(f.Content), "name: worker") {
			t.Error("--service web must emit only web")
		}
		if f.Name == platformimport.EmitSecretsFile {
			for _, want := range []string{"`STRIPE_KEY`", "`SECRET_KEY_BASE`", "`DATABASE_URL`"} {
				if !strings.Contains(string(f.Content), want) {
					t.Errorf("SECRETS.md misses %q", want)
				}
			}
		}
	}
	if _, err := Translate(loadCase(t, "heroku-full")).EmitInput(EmitOptions{Services: []string{"nope"}}); err == nil {
		t.Error("an unknown --service must be an error")
	}
}
