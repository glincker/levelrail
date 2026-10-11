package proxyroutes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApply_AddUpdateRemoveIdempotent(t *testing.T) {
	dir := t.TempDir()
	d, err := OpenDir(testNS, dir)
	if err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(dir, "coolify.yaml")
	if err := os.WriteFile(unrelated, []byte("http: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := mustRender(t, "a.example.com", "127.0.0.1:8088")
	b := mustRender(t, "b.example.com", "127.0.0.1:8088")

	steps := []struct {
		name        string
		want        []File
		wantChanged map[string]bool
		wantRemoved []string
	}{
		{"add two", []File{a, b}, map[string]bool{"a.example.com": true, "b.example.com": true}, nil},
		{"repeat is a no-op", []File{a, b}, map[string]bool{"a.example.com": false, "b.example.com": false}, nil},
		{"update one", []File{a, mustRender(t, "b.example.com", "127.0.0.1:9000")}, map[string]bool{"a.example.com": false, "b.example.com": true}, nil},
		{"remove one", []File{a}, map[string]bool{"a.example.com": false}, []string{"b.example.com"}},
		{"remove is idempotent", []File{a}, map[string]bool{"a.example.com": false}, nil},
		{"remove all", nil, map[string]bool{}, []string{"a.example.com"}},
	}
	for _, s := range steps {
		res, err := Apply(d, s.want, nil)
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		for _, o := range res.Files {
			if o.Err != nil || o.Changed != s.wantChanged[o.Domain] {
				t.Errorf("%s: %s changed=%v err=%v", s.name, o.Domain, o.Changed, o.Err)
			}
		}
		if len(res.Removed) != len(s.wantRemoved) {
			t.Fatalf("%s: removed %v, want %v", s.name, res.Removed, s.wantRemoved)
		}
		for i, r := range res.Removed {
			if r.Domain != s.wantRemoved[i] || r.Err != nil {
				t.Errorf("%s: removed %+v", s.name, r)
			}
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Error("unrelated file was touched")
	}
}

// A previous pass interrupted between temp file creation and rename leaves a
// dot temp file; the next pass still converges and ignores it.
func TestApply_HalfSucceededPassConverges(t *testing.T) {
	dir := t.TempDir()
	d, err := OpenDir(testNS, dir)
	if err != nil {
		t.Fatal(err)
	}
	a := mustRender(t, "a.example.com", "127.0.0.1:8088")
	if err := os.WriteFile(filepath.Join(dir, "."+testNS.FilePrefix()+"123.tmp"), a.Content[:10], 0o600); err != nil {
		t.Fatal(err)
	}
	foreignName := testNS.FilePrefix() + "z.example.com.yaml"
	if err := os.WriteFile(filepath.Join(dir, foreignName), []byte("hand written\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Apply(d, []File{a}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 1 || !res.Files[0].Changed {
		t.Errorf("files = %+v", res.Files)
	}
	if len(res.Foreign) != 1 || res.Foreign[0] != foreignName || len(res.Removed) != 0 {
		t.Errorf("foreign = %v removed = %v", res.Foreign, res.Removed)
	}
	got, _ := os.ReadFile(filepath.Join(dir, a.Name)) //nolint:gosec // test temp dir
	if string(got) != string(a.Content) {
		t.Error("content mismatch")
	}
}
