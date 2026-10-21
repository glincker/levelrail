package staticroutes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileErrors(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		l    Limits
		want string
	}{
		{"spec rule invalid", Input{Routes: []Rule{{From: "/a", To: "//evil.example.com"}}}, DefaultLimits(), "routes[0]"},
		{"spec header invalid", Input{HeaderBlocks: []HeaderBlock{{Path: "/*", Set: []HeaderKV{{Name: "Host", Value: "x"}}}}}, DefaultLimits(), "headers[0]"},
		{"bad slash option", Input{Options: Options{TrailingSlash: "sideways"}}, DefaultLimits(), "trailing_slash"},
		{"escaping not found page", Input{Options: Options{NotFound: "../x.html"}}, DefaultLimits(), "not_found"},
		{"too many rules", Input{Redirects: "/a /b\n/c /d\n/e /f"}, Limits{MaxRules: 2, MaxHeaderBlocks: 10, MaxFileBytes: 1000, MaxValueLen: 100}, "over the cap"},
		{"too many header blocks", Input{Headers: "/a\n  X-A: b\n/c\n  X-A: b\n"}, Limits{MaxRules: 10, MaxHeaderBlocks: 1, MaxFileBytes: 1000, MaxValueLen: 100}, "header blocks"},
		{"file too big", Input{Redirects: strings.Repeat("/a /b\n", 100)}, Limits{MaxRules: 1000, MaxHeaderBlocks: 10, MaxFileBytes: 50, MaxValueLen: 100}, "byte cap"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Compile(tc.in, tc.l)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCompileKeepsBadLinesAsProblems(t *testing.T) {
	rs, err := Compile(Input{Redirects: "/a /b\nnonsense\n", Headers: "/*\n  Host: x\n"}, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.Rules) != 1 || len(rs.Problems) != 2 {
		t.Fatalf("rules=%d problems=%v", len(rs.Rules), rs.Problems)
	}
}

func TestLimitsFromEnv(t *testing.T) {
	env := map[string]string{EnvMaxRules: "5", EnvMaxFileBytes: "bogus", EnvMaxValueLen: "-3"}
	l := LimitsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	d := DefaultLimits()
	if l.MaxRules != 5 || l.MaxFileBytes != d.MaxFileBytes || l.MaxValueLen != d.MaxValueLen {
		t.Fatalf("limits = %+v", l)
	}
}

func TestSidecarRoundTripAndMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "web", "abc123")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if rs, err := LoadSidecar(root); rs != nil || err != nil {
		t.Fatalf("a tree with no rules should load as nil: %v %v", rs, err)
	}
	in, err := Compile(Input{Redirects: "/old /new 301!", Headers: "/*\n  X-A: b\n", Options: Options{SPA: true}}, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteSidecar(root, in); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(SidecarPath(root)) != filepath.Dir(root) {
		t.Fatal("the sidecar must sit beside the tree, outside what is served")
	}
	out, err := LoadSidecar(root)
	if err != nil || out == nil {
		t.Fatalf("load: %v %v", out, err)
	}
	if len(out.Rules) != 1 || !out.Rules[0].Force || !out.Options.SPA || out.Rules[0].pat == nil || out.Headers[0].pat == nil {
		t.Fatalf("round trip lost data: %+v", out)
	}
}

func TestLoadSidecarRejectsCorruptAndWrongVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "v")
	for name, body := range map[string]string{
		"corrupt":   "{not json",
		"version":   `{"version":99}`,
		"bad rule":  `{"version":1,"rules":[{"source":"x","from":"/a","to":"//evil.example.com","status":301}]}`,
		"bad route": `{"version":1,"rules":[{"source":"x","from":"nope","to":"/b","status":301}]}`,
	} {
		if err := os.WriteFile(SidecarPath(root), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if rs, err := LoadSidecar(root); err == nil || rs != nil {
			t.Fatalf("%s: want an error, got %v %v", name, rs, err)
		}
	}
}
