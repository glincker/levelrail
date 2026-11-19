package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const goodPage = `<html><head><title>Hi</title><meta name="description" content="d"><meta name="viewport" content="width=device-width"></head><body><a href="/about.html">a</a><img src="logo.png"></body></html>`

func TestRunStaticSiteChecks(t *testing.T) {
	big := strings.Repeat("x", 3<<10)
	cases := []struct {
		name  string
		files map[string]string
		want  map[string]string
		minSc int
		maxSc int
	}{
		{
			name:  "healthy",
			files: map[string]string{"index.html": goodPage, "about.html": goodPage, "404.html": goodPage, "logo.png": "p", "_headers": "/*\n  X-Content-Type-Options: nosniff\n  X-Frame-Options: DENY\n"},
			want:  map[string]string{"index_html": "ok", "custom_404": "ok", "broken_links": "ok", "meta_title": "ok", "header_nosniff": "ok", "header_frame": "ok"},
			minSc: 100, maxSc: 100,
		},
		{
			name:  "bare site",
			files: map[string]string{"page.html": `<html><body><a href="/gone">x</a><script src="missing.js"></script><a href="https://x.dev/y">e</a></body></html>`},
			want:  map[string]string{"index_html": "fail", "custom_404": "warn", "meta_title": "warn", "meta_description": "warn", "meta_viewport": "warn", "broken_link": "fail", "header_nosniff": "warn", "header_frame": "warn"},
			minSc: 0, maxSc: 40,
		},
		{
			name:  "large files",
			files: map[string]string{"index.html": goodPage, "photo.png": big, "bundle.js": big},
			want:  map[string]string{"large_file": "warn", "large_image": "warn"},
			minSc: 0, maxSc: 100,
		},
		{
			name:  "redirect rule heals a link",
			files: map[string]string{"index.html": strings.Replace(goodPage, `href="/about.html"`, `href="/old"`, 1), "logo.png": "p", "_redirects": "/old /index.html 301\n"},
			want:  map[string]string{"broken_links": "ok"},
			minSc: 0, maxSc: 100,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeTree(t, tc.files)
			in := staticroutes.Input{Redirects: tc.files["_redirects"], Headers: tc.files["_headers"]}
			rs, err := staticroutes.Compile(in, staticroutes.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			res, err := runStaticSiteChecks(dir, rs, staticCheckLimits{LargeFileBytes: 2 << 10, MaxFiles: 100})
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, c := range res.Checks {
				got[c.ID] = c.Status
			}
			for id, st := range tc.want {
				if got[id] != st {
					t.Errorf("check %s = %q, want %q (all: %+v)", id, got[id], st, res.Checks)
				}
			}
			if res.Score < tc.minSc || res.Score > tc.maxSc {
				t.Errorf("score %d outside %d..%d", res.Score, tc.minSc, tc.maxSc)
			}
		})
	}
}

func TestRunStaticSiteChecks_FileCap(t *testing.T) {
	files := map[string]string{"index.html": goodPage, "a.txt": "a", "b.txt": "b", "c.txt": "c"}
	rs, _ := staticroutes.Compile(staticroutes.Input{}, staticroutes.DefaultLimits())
	res, err := runStaticSiteChecks(writeTree(t, files), rs, staticCheckLimits{LargeFileBytes: 1 << 20, MaxFiles: 2})
	if err != nil || !res.Truncated || res.FilesScanned != 2 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
