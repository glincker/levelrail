package proxyroutes

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func mustRender(t *testing.T, domain, upstream string) File {
	t.Helper()
	name, err := testNS.FileName(domain)
	if err != nil {
		t.Fatal(err)
	}
	body, err := testNS.Render(Route{Domain: domain, EntrypointHTTP: "http", EntrypointHTTPS: "https", CertResolver: "letsencrypt", Upstream: upstream})
	if err != nil {
		t.Fatal(err)
	}
	return File{Domain: domain, Name: name, Content: body}
}

func TestOpenDir_PathSafety(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "proxy", "dynamic")
	sibling := filepath.Join(root, "proxy", "dynamic-v2")
	outside := t.TempDir()
	for _, d := range []string{inside, sibling} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	linkSibling := filepath.Join(root, "proxy", "current")
	linkOutside := filepath.Join(root, "proxy", "escape")
	if err := os.Symlink(sibling, linkSibling); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, linkOutside); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(inside, "plain")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"real dir", inside, false},
		{"trailing slash", inside + "/", false},
		{"symlink under same parent", linkSibling, false},
		{"symlink elsewhere", linkOutside, true},
		{"relative", "proxy/dynamic", true},
		{"traversal into missing", filepath.Join(inside, "..", "missing"), true},
		{"root", "/", true},
		{"a file", file, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := OpenDir(testNS, tc.path)
			if (err != nil) != tc.wantErr {
				t.Errorf("OpenDir(%q) error = %v, wantErr %v", tc.path, err, tc.wantErr)
			}
		})
	}
}

func TestDir_NeverTouchesForeignFiles(t *testing.T) {
	dir := t.TempDir()
	d, err := OpenDir(testNS, dir)
	if err != nil {
		t.Fatal(err)
	}
	f := mustRender(t, "a.example.com", "127.0.0.1:8088")
	foreign := []byte("http:\n  routers: {}\n")
	foreignPath := filepath.Join(dir, f.Name)
	if err := os.WriteFile(foreignPath, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.Write(f.Name, f.Content); err == nil {
		t.Error("Write over a file without the header: expected an error")
	}
	if err := d.Remove(f.Name); err == nil {
		t.Error("Remove of a file without the header: expected an error")
	}
	got, _ := os.ReadFile(foreignPath) //nolint:gosec // test temp dir
	if !bytes.Equal(got, foreign) {
		t.Error("foreign file was modified")
	}

	// A symlink with a managed name, pointing at a file with the header, is
	// still never followed.
	target := filepath.Join(t.TempDir(), "victim.yaml")
	if err := os.WriteFile(target, f.Content, 0o600); err != nil {
		t.Fatal(err)
	}
	linkName := testNS.FilePrefix() + "b.example.com.yaml"
	if err := os.Symlink(target, filepath.Join(dir, linkName)); err != nil {
		t.Fatal(err)
	}
	if err := d.Remove(linkName); err == nil {
		t.Error("Remove of a symlink: expected an error")
	}
	if _, err := os.Stat(target); err != nil {
		t.Error("symlink target was removed")
	}

	for _, bad := range []string{"../x.yaml", "other.yaml", testNS.FilePrefix() + "../../x.yaml", testNS.FilePrefix() + "a.yaml/x"} {
		if err := d.Write(bad, f.Content); err == nil {
			t.Errorf("Write(%q): expected an error", bad)
		}
	}
	if err := d.Write(testNS.FilePrefix()+"c.example.com.yaml", []byte("no header\n")); err == nil {
		t.Error("Write without header: expected an error")
	}
}

func TestDir_WriteIsAtomicAndWorldReadable(t *testing.T) {
	dir := t.TempDir()
	d, err := OpenDir(testNS, dir)
	if err != nil {
		t.Fatal(err)
	}
	f := mustRender(t, "a.example.com", "127.0.0.1:8088")
	if err := d.Write(f.Name, f.Content); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, f.Name))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

func TestDir_RefusesWhenDirectorySwappedForSymlink(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dynamic")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	d, err := OpenDir(testNS, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), dir); err != nil {
		t.Fatal(err)
	}
	f := mustRender(t, "a.example.com", "127.0.0.1:8088")
	if err := d.Write(f.Name, f.Content); err == nil {
		t.Error("expected a refusal after the directory became a symlink elsewhere")
	}
}
