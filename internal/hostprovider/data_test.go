package hostprovider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultsKeepGuidance(t *testing.T) {
	t.Setenv(EnvDataFile, "")
	g := For(Oracle, "dmi")
	if g.Name != "Oracle Cloud" || len(g.Steps) != 2 || !strings.HasPrefix(g.DocsURL, "https://") || g.Note == "" {
		t.Fatalf("unexpected oracle guidance: %+v", g)
	}
	if For(AWS, "x").Note == "" || For(GCP, "x").Note != "" {
		t.Fatal("note mismatch")
	}
}

func TestOverrideMergesAndBadFileIsIgnored(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "o.json")
	if err := os.WriteFile(good, []byte(`{"aws":{"docs_url":"https://example.test/x"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvDataFile, good)
	g := For(AWS, "x")
	if g.DocsURL != "https://example.test/x" || len(g.Steps) != 3 {
		t.Fatalf("merge failed: %+v", g)
	}
	bad := filepath.Join(dir, "b.json")
	if err := os.WriteFile(bad, []byte(`{nope`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvDataFile, bad)
	d, err := LoadData()
	if err == nil || d["aws"].Name != "Amazon EC2" {
		t.Fatalf("expected warning with defaults, got %v", err)
	}
}
