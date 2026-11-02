package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var edgeScanDirs = []string{"app", "pages", "src/app", "src/pages"}
var edgeScanFiles = []string{"middleware.ts", "middleware.js", "src/middleware.ts", "src/middleware.js"}
var edgeScanExt = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true}

// addEdgeSources adds the route and middleware sources that mention the edge
// runtime, so the Vercel translation can warn about them. Unreadable files are
// skipped: the scan only adds a warning.
func addEdgeSources(dir string, files map[string][]byte) {
	keep := func(rel string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || info.Size() > maxConfigFileBytes {
			return
		}
		b, err := os.ReadFile(p) //nolint:gosec // the operator names the directory to read
		if err == nil && bytes.Contains(b, []byte("edge")) {
			files[rel] = b
		}
	}
	for _, rel := range edgeScanFiles {
		keep(rel)
	}
	for _, d := range edgeScanDirs {
		root := filepath.Join(dir, filepath.FromSlash(d))
		_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if e.IsDir() {
				if e.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if edgeScanExt[filepath.Ext(p)] {
				if rel, err := filepath.Rel(dir, p); err == nil {
					keep(strings.ReplaceAll(rel, string(filepath.Separator), "/"))
				}
			}
			return nil
		})
	}
}
