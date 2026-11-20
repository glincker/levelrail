package projectarchive

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
)

func build(t *testing.T, spec string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.AddBytes(SpecPrefix+"a.yaml", []byte(spec)); err != nil {
		t.Fatal(err)
	}
	if err := w.AddStream(DataPrefix+"web/data.bak", strings.NewReader("dump")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(Manifest{Project: "shop", SourceVersion: "v1"}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadVerifiesAndKeepsSpec(t *testing.T) {
	a, err := Read(bytes.NewReader(build(t, "kind: Project\n")))
	if err != nil || string(a.Spec["a.yaml"]) != "kind: Project\n" || len(a.Manifest.Files) != 2 {
		t.Fatalf("read = %+v %v", a, err)
	}
}

func TestTamperedSpecFailsChecksum(t *testing.T) {
	raw := build(t, "kind: Project\n")
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(gz)
	plain = bytes.Replace(plain, []byte("kind: Project"), []byte("kind: Projecx"), 1)
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	_, _ = zw.Write(plain)
	_ = zw.Close()
	if _, err := Read(&out); !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v", err)
	}
}

func TestDamagedStreamRejected(t *testing.T) {
	raw := build(t, "kind: Project\n")
	if _, err := Read(bytes.NewReader(raw[:len(raw)/2])); err == nil {
		t.Fatal("truncated archive accepted")
	}
}

func TestSealRoundTripAndWrongPassphrase(t *testing.T) {
	blob, err := Seal("a long enough pass", []byte(`{"secrets":{"web/K":"v"}}`))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Open("a long enough pass", blob)
	if err != nil || !strings.Contains(string(plain), `"v"`) {
		t.Fatalf("open = %q %v", plain, err)
	}
	if _, err := Open("another long pass!", blob); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("wrong passphrase err = %v", err)
	}
	if _, err := Seal("short", nil); err == nil {
		t.Fatal("short passphrase accepted")
	}
}

func TestRenameProject(t *testing.T) {
	in := map[string][]byte{"x.yaml": []byte("kind: Project\nmetadata: {name: shop}\n---\nkind: App\nmetadata: {name: web}\nspec: {project: shop}\n")}
	out, err := RenameProject(in, "shop", "shop2")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out["x.yaml"])
	if strings.Count(s, "shop2") != 2 || strings.Contains(strings.ReplaceAll(s, "shop2", ""), "shop") {
		t.Fatalf("rename output:\n%s", s)
	}
}
