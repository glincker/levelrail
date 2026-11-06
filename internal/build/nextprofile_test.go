package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectNext(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		dirs  []string
		want  NextProfile
	}{
		{"not next", map[string]string{"package.json": `{"dependencies":{"react":"18"}}`}, nil, NextProfile{}},
		{"no package", nil, nil, NextProfile{}},
		{"bad json", map[string]string{"package.json": `{`}, nil, NextProfile{}},
		{"range only", map[string]string{"package.json": `{"dependencies":{"next":"^14.2.3"}}`}, []string{"pages"}, NextProfile{Detected: true, Major: 14}},
		{"lock wins", map[string]string{
			"package.json":      `{"dependencies":{"next":"latest"}}`,
			"package-lock.json": `{"packages":{"node_modules/next": {"version": "15.1.0"}}}`,
		}, []string{"src/app"}, NextProfile{Detected: true, Major: 15, UsesAppRouter: true}},
		{"standalone", map[string]string{
			"package.json":    `{"devDependencies":{"next":"~13.5.0"}}`,
			"next.config.mjs": `export default { output: 'standalone' }`,
		}, []string{"app"}, NextProfile{Detected: true, Major: 13, Standalone: true, UsesAppRouter: true}},
		{"config without standalone", map[string]string{
			"package.json":   `{"dependencies":{"next":"14.0.0"}}`,
			"next.config.js": `module.exports = {}`,
		}, nil, NextProfile{Detected: true, Major: 14}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for n, body := range c.files {
				if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, d := range c.dirs {
				if err := os.MkdirAll(filepath.Join(dir, d), 0o750); err != nil {
					t.Fatal(err)
				}
			}
			if got := DetectNext(dir); got != c.want {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}
