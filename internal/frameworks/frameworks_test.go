package frameworks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixturesRoot = "../../test/fixtures/frameworks"

func dirReader(dir string) Reader {
	return func(rel string) (string, bool) {
		raw, err := os.ReadFile(filepath.Join(dir, rel)) //nolint:gosec // test fixture path
		if err != nil {
			return "", false
		}
		return string(raw), true
	}
}

func TestDetectFixtures(t *testing.T) {
	for _, p := range List() {
		t.Run(p.ID, func(t *testing.T) {
			m, ok := Detect(dirReader(filepath.Join(fixturesRoot, p.ID)))
			if !ok {
				t.Fatalf("no preset detected for fixture %s", p.ID)
			}
			if m.Preset.ID != p.ID {
				t.Fatalf("detected %s, want %s (%s)", m.Preset.ID, p.ID, m.Evidence)
			}
		})
	}
}

func TestDetectNothing(t *testing.T) {
	if _, ok := Detect(func(string) (string, bool) { return "", false }); ok {
		t.Fatal("expected no match for an empty tree")
	}
	if _, ok := Detect(func(rel string) (string, bool) { return "{bad", rel == "package.json" }); ok {
		t.Fatal("expected no match for malformed package.json")
	}
}

func TestPresetInvariants(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range List() {
		if seen[p.ID] {
			t.Fatalf("duplicate preset id %s", p.ID)
		}
		seen[p.ID] = true
		if p.BuildType != BuildStatic && p.Port <= 0 {
			t.Errorf("%s: server preset needs a port", p.ID)
		}
		if p.HealthPath == "" || !strings.HasPrefix(p.HealthPath, "/") {
			t.Errorf("%s: bad health path %q", p.ID, p.HealthPath)
		}
		if p.Icon == "" || p.DocsAnchor == "" {
			t.Errorf("%s: missing icon or docs anchor", p.ID)
		}
		if p.Dockerfile {
			if _, ok := DockerfileTemplate(p.ID); !ok {
				t.Errorf("%s: dockerfile_template set but no template embedded", p.ID)
			}
		}
	}
}

func TestDockerfileTemplate(t *testing.T) {
	if _, ok := DockerfileTemplate("django"); ok {
		t.Fatal("django ships no template")
	}
	text, ok := DockerfileTemplate("go")
	if !ok || !strings.Contains(text, "distroless") {
		t.Fatalf("go template missing distroless: %v", ok)
	}
}

func TestDetectRemixBeatsVite(t *testing.T) {
	for _, pkg := range []string{
		`{"dependencies":{"@remix-run/react":"2"},"devDependencies":{"vite":"5"}}`,
		`{"devDependencies":{"@remix-run/dev":"2","vite":"5"}}`,
	} {
		m, ok := Detect(func(rel string) (string, bool) { return pkg, rel == "package.json" })
		if !ok || m.Preset.ID != "remix" {
			t.Fatalf("pkg %s: got %q ok=%v, want remix", pkg, m.Preset.ID, ok)
		}
	}
}
