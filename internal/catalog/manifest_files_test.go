package catalog

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
)

const fileManifestHead = `id: demo
name: Demo
description: A demo stack.
category: Testing
documentation_url: https://example.com
recommended_memory_bytes: 1024
`

const fileCompose = "services:\n  web:\n    image: nginx:alpine\n    ports: [\"80\"]\n"

func fileManifest(t *testing.T, env string) Manifest {
	t.Helper()
	ms, err := LoadManifests(demoFS(fileManifestHead+env, fileCompose), "m")
	if err != nil {
		t.Fatalf("LoadManifests() error = %v", err)
	}
	return ms[0]
}

func desiredFiles(t *testing.T, body string, generate func(kind, key string, length int) (string, error)) []store.ServiceFile {
	t.Helper()
	f, err := compose.Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, unresolved, err := compose.ResolveMagicVars(f, generate, func(string, string, string) error { return nil }); err != nil || len(unresolved) > 0 {
		t.Fatalf("resolve: %v %v", err, unresolved)
	}
	d, _, err := compose.ToDesiredServices("demo", f)
	if err != nil {
		t.Fatalf("ToDesiredServices: %v", err)
	}
	return d[0].Files
}

func TestFilePrompt_DefaultExpandsSettingsAndSuppliedIsLiteral(t *testing.T) {
	m := fileManifest(t, `env:
  - key: GREETING
    default: hello
  - key: APP_CONF
    type: file
    service: web
    path: /etc/app/app.conf
    default: "say ${GREETING}\n"
`)
	body, err := m.Render(InstallInput{}, false)
	if err != nil {
		t.Fatal(err)
	}
	stub := func(string, string, int) (string, error) { return "x", nil }
	files := desiredFiles(t, body, stub)
	if len(files) != 1 || files[0].Path != "/etc/app/app.conf" || files[0].Content != "say hello\n" || files[0].Mode != "0444" || files[0].Secret {
		t.Fatalf("default render: %+v", files)
	}
	body, err = m.Render(InstallInput{Env: map[string]string{"APP_CONF": "raw ${GREETING} $HOME ${SERVICE_PASSWORD_X}\n"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := desiredFiles(t, body, stub)[0].Content; got != "raw ${GREETING} $HOME ${SERVICE_PASSWORD_X}\n" {
		t.Fatalf("supplied body must stay literal, got %q", got)
	}
}

func TestFilePrompt_SecretFileHoldsGeneratedValueAndIsRedacted(t *testing.T) {
	m := fileManifest(t, `env:
  - key: TOKEN
    generate: password
    secret: true
  - key: APP_SECRET_CONF
    type: file
    secret: true
    service: web
    path: /run/app/token.conf
    default: "token=${TOKEN}\n"
`)
	body, err := m.Render(InstallInput{}, false)
	if err != nil {
		t.Fatal(err)
	}
	files := desiredFiles(t, body, func(string, string, int) (string, error) { return "g3n3r4ted", nil })
	if len(files) != 1 || !files[0].Secret || files[0].Mode != "0400" || files[0].Content != "token=g3n3r4ted\n" {
		t.Fatalf("secret file: %+v", files)
	}
	redacted, changed, err := compose.RedactSecretContent([]byte(body))
	if err != nil || !changed || strings.Contains(string(redacted), "token=") {
		t.Fatalf("stored revision must not keep the body: changed=%v err=%v\n%s", changed, err, redacted)
	}
}

func TestFilePrompt_RequiredWithoutBodyNeedsConfiguration(t *testing.T) {
	m := fileManifest(t, `env:
  - key: APP_CONF
    type: file
    required: true
    service: web
    path: /etc/app.conf
`)
	if _, err := m.Render(InstallInput{}, false); err == nil || !strings.Contains(err.Error(), "APP_CONF") {
		t.Fatalf("want a missing-value error, got %v", err)
	}
	body, err := m.Render(InstallInput{}, true)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := compose.Parse([]byte(body))
	_, unresolved, _ := compose.ResolveMagicVars(f, nil, func(string, string, string) error { return nil })
	if len(unresolved) != 1 {
		t.Fatalf("default render must flag the file as unresolved, got %v", unresolved)
	}
}

func TestFilePrompt_PathsAndShapeValidated(t *testing.T) {
	for name, env := range map[string]string{
		"traversal": "    path: /etc/../root/x\n    service: web\n",
		"relative":  "    path: conf/app.conf\n    service: web\n",
		"proc":      "    path: /proc/sys/x\n    service: web\n",
		"service":   "    path: /etc/x.conf\n    service: nope\n",
		"nosvc":     "    path: /etc/x.conf\n",
		"mode":      "    path: /etc/x.conf\n    service: web\n    mode: \"0999\"\n",
		"generate":  "    path: /etc/x.conf\n    service: web\n    generate: password\n",
	} {
		manifest := fileManifestHead + "env:\n  - key: APP_CONF\n    type: file\n    default: x\n" + env
		if _, err := LoadManifests(demoFS(manifest, fileCompose), "m"); err == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
}

func TestCheckFileInputs_CapsSuppliedBody(t *testing.T) {
	m := fileManifest(t, "env:\n  - key: APP_CONF\n    type: file\n    service: web\n    path: /etc/x.conf\n    default: x\n")
	if err := m.CheckFileInputs(map[string]string{"APP_CONF": strings.Repeat("a", 11)}, 10); err == nil || strings.Contains(err.Error(), "aaaa") {
		t.Fatalf("want a size error without the body, got %v", err)
	}
	if err := m.CheckFileInputs(map[string]string{"APP_CONF": "aaaa"}, 10); err != nil {
		t.Fatal(err)
	}
	if keys := m.FileInputKeys(); len(keys) != 1 || keys[0] != "APP_CONF" {
		t.Fatalf("FileInputKeys = %v", keys)
	}
}
