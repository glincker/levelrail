package sourceupload

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanSkipsEditorAndAgentConfig(t *testing.T) {
	cases := []struct {
		path    string
		include []string
		want    bool
	}{
		{".claude/settings.local.json", nil, false},
		{".claude/skills/a/SKILL.md", nil, false},
		{"pkg/.claude/x.md", nil, false},
		{".claude/skills/a/SKILL.md", []string{".claude/skills"}, true},
		{".cursor/rules.md", nil, false},
		{".idea/workspace.xml", nil, false},
		{".aider.chat.history.md", nil, false},
		{".vscode/settings.json", nil, false},
		{".vscode/launch.json", nil, true},
		{"app.code-workspace", nil, true},
		{".vscode/app.code-workspace", nil, false},
		{"Thumbs.db", nil, false},
		{"deep/dir/settings.local.json", nil, false},
		{"config.local.yaml", nil, false},
		{"src/claude.ts", nil, true},
		{"src/local.json", nil, true},
		{"src/index.ts", nil, true},
	}
	root := t.TempDir()
	for _, c := range cases {
		p := filepath.Join(root, filepath.FromSlash(c.path))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range cases {
		if c.include != nil {
			plan, err := Scan(context.Background(), root, Options{Include: c.include})
			if err != nil {
				t.Fatal(err)
			}
			if got := hasFile(plan, c.path); got != c.want {
				t.Errorf("%s with include %v: included=%v want %v", c.path, c.include, got, c.want)
			}
		}
	}
	plan, err := Scan(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if c.include != nil {
			continue
		}
		if got := hasFile(plan, c.path); got != c.want {
			t.Errorf("%s: included=%v want %v", c.path, got, c.want)
		}
	}
	n, roots := plan.EditorSkippedSummary(3)
	if n == 0 || len(roots) != 3 {
		t.Errorf("summary n=%d roots=%v", n, roots)
	}
	if n3, names := plan.EditorSkippedSummary(100); n3 != len(names) || n3 != n {
		t.Errorf("count %d must match listed files %v", n3, names)
	}
}

func hasFile(p Plan, path string) bool {
	for _, f := range p.Files {
		if f.Path == path {
			return true
		}
	}
	return false
}
