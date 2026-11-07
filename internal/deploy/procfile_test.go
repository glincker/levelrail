package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestProcfileServicesFanOut(t *testing.T) {
	dir := writeRepo(t, map[string]string{"Procfile": "web: node server.js\nworker: node jobs.js\n"})
	svcs, findings, ok, err := ProcfileServices(dir)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(svcs) != 2 {
		t.Fatalf("services = %d, want 2", len(svcs))
	}
	if svcs["web"].Port == 0 || svcs["web"].Build.Type != spec.BuildRailpack {
		t.Errorf("web = %+v", svcs["web"])
	}
	if got := svcs["worker"].Command; len(got) != 3 || got[2] != "node jobs.js" {
		t.Errorf("worker command = %v", got)
	}
	if len(FindingLines(findings)) != len(findings) {
		t.Error("every finding must render a line")
	}
}

func TestProcfileServicesSkippedWithAppSpec(t *testing.T) {
	dir := writeRepo(t, map[string]string{"Procfile": "web: x\n", "app.yaml": "version: 1\n"})
	if _, _, ok, err := ProcfileServices(dir); ok || err != nil {
		t.Fatalf("ok=%v err=%v, want skipped", ok, err)
	}
}

func TestProcfileServicesSkippedWithoutProcfile(t *testing.T) {
	dir := writeRepo(t, map[string]string{"app.json": "{}"})
	if _, _, ok, err := ProcfileServices(dir); ok || err != nil {
		t.Fatalf("ok=%v err=%v, want skipped", ok, err)
	}
}

func TestProcfileServicesBadAppJSON(t *testing.T) {
	dir := writeRepo(t, map[string]string{"Procfile": "web: x\n", "app.json": "{nope"})
	_, _, ok, err := ProcfileServices(dir)
	if ok || err == nil || !strings.Contains(err.Error(), "app.json") {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestProcfileServicesSkipsDatabases(t *testing.T) {
	dir := writeRepo(t, map[string]string{
		"Procfile": "web: x\n",
		"app.json": `{"addons":["heroku-postgresql:essential-0"]}`,
	})
	svcs, findings, ok, err := ProcfileServices(dir)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if _, has := svcs["web"].Env["DATABASE_URL"]; has {
		t.Error("DATABASE_URL must not reference a database that is not created")
	}
	found := false
	for _, f := range findings {
		found = found || f.Code == "heroku.addon.deploy-manual"
	}
	if !found {
		t.Errorf("missing deploy-manual finding: %v", findings)
	}
}
