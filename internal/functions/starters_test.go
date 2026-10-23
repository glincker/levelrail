package functions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartersShipEveryFile(t *testing.T) {
	want := []string{"README.md", "handler.js", "handler.test.js", "index.js", "kv.js", "package.json"}
	got := Starters()
	if len(got) != 5 {
		t.Fatalf("starters = %d, want 5", len(got))
	}
	for _, s := range got {
		have := map[string]bool{}
		for _, f := range s.Files {
			have[f.Path] = true
		}
		for _, w := range want {
			if !have[w] {
				t.Errorf("%s is missing %s", s.ID, w)
			}
		}
		if s.Preset == "" || len(s.Env) == 0 {
			t.Errorf("%s needs a preset and its env vars", s.ID)
		}
	}
}

func TestStarterSharedFilesMatchSource(t *testing.T) {
	for _, s := range Starters() {
		for _, f := range s.Files {
			switch f.Path {
			case "kv.js", "index.js", "package.json":
				b, err := os.ReadFile(filepath.Join("starters", "_shared", f.Path))
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != f.Content {
					t.Errorf("%s/%s drifted from starters/_shared, copy it again", s.ID, f.Path)
				}
			}
		}
	}
}

func TestLookupStarter(t *testing.T) {
	if _, ok := LookupStarter("chat-bot"); !ok {
		t.Fatal("chat-bot not found")
	}
	if _, ok := LookupStarter("missing"); ok {
		t.Fatal("unexpected starter")
	}
}

func TestKVConfigFromEnv(t *testing.T) {
	env := map[string]string{EnvKVMaxKeys: "5", EnvKVMaxValueBytes: "bad", EnvKVMaxTotalBytes: "-1"}
	c := KVConfigFromEnv(func(k string) string { return env[k] })
	if c.MaxKeys != 5 || c.MaxValueBytes != defaultKVMaxValueBytes || c.MaxTotalBytes != defaultKVMaxTotalBytes {
		t.Fatalf("config = %+v", c)
	}
}
