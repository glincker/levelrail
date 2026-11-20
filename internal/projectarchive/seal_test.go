package projectarchive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testPass = "correct horse battery"

func sealedArchive(t *testing.T, spec string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.AddBytes(SpecPrefix+"a.yaml", []byte(spec)); err != nil {
		t.Fatal(err)
	}
	w.SetSecrets(testPass, []byte(`{"secrets":{"web/K":"v"}}`))
	if err := w.Close(Manifest{Project: "shop"}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSealedSecretsOpenAndBindToManifest(t *testing.T) {
	a, err := Read(bytes.NewReader(sealedArchive(t, "kind: Project\n")))
	if err != nil || !a.Manifest.SecretsIncluded {
		t.Fatalf("read: %v", err)
	}
	if plain, err := a.OpenSecrets(testPass); err != nil || !strings.Contains(string(plain), "web/K") {
		t.Fatalf("open: %v", err)
	}
	if _, err := a.OpenSecrets("another passphrase!!"); err == nil {
		t.Fatal("wrong passphrase opened")
	}
	// The same blob under a different manifest must not open.
	a.Manifest.Project = "other"
	if _, err := a.OpenSecrets(testPass); !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("changed manifest err = %v", err)
	}
}

func TestSealedBlobFromAnotherArchiveIsRefused(t *testing.T) {
	a, _ := Read(bytes.NewReader(sealedArchive(t, "kind: Project\n")))
	b, _ := Read(bytes.NewReader(sealedArchive(t, "kind: Project\nmetadata: {name: x}\n")))
	a.Secrets = b.Secrets
	if _, err := a.OpenSecrets(testPass); !errors.Is(err, ErrManifestMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestShortPassphraseRefusedAndFloorHolds(t *testing.T) {
	if PassphraseOK("short", 3) || !PassphraseOK("exactly12char", 1) {
		t.Fatal("the floor of 12 must hold whatever the setting says")
	}
	if PassphraseOK("elevenchars", 5) || !PassphraseOK("ééééééééééééé", 12) {
		t.Fatal("runes must count, not bytes")
	}
}

func rawTar(t *testing.T, entries [][2]string, trailer []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		_ = tw.WriteHeader(&tar.Header{Name: e[0], Mode: 0o600, Size: int64(len(e[1]))})
		_, _ = tw.Write([]byte(e[1]))
	}
	_ = tw.Close()
	_, _ = gz.Write(trailer)
	_ = gz.Close()
	return buf.Bytes()
}

const okManifest = `{"format":2,"kind":"levelrail.project-archive","files":[]}`

func TestReadRejectsHostileShapes(t *testing.T) {
	cases := map[string][]byte{
		"duplicate manifest": rawTar(t, [][2]string{{ManifestName, okManifest}, {ManifestName, okManifest}}, nil),
		"dotdot name":        rawTar(t, [][2]string{{"spec/../x", "a"}, {ManifestName, okManifest}}, nil),
		"long name":          rawTar(t, [][2]string{{SpecPrefix + strings.Repeat("a", 300), "a"}, {ManifestName, okManifest}}, nil),
		"unknown top level":  rawTar(t, [][2]string{{"etc/x", "a"}, {ManifestName, okManifest}}, nil),
		"bytes after tar":    rawTar(t, [][2]string{{ManifestName, okManifest}}, []byte("junk")),
	}
	for name, raw := range cases {
		if _, err := Read(bytes.NewReader(raw)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	good := rawTar(t, [][2]string{{ManifestName, okManifest}}, nil)
	if _, err := Read(bytes.NewReader(good)); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := Read(bytes.NewReader(append(append([]byte{}, good...), good...))); err == nil {
		t.Error("concatenated gzip stream accepted")
	}
}

func TestReadEnforcesLimits(t *testing.T) {
	big := rawTar(t, [][2]string{{DataPrefix + "a/b.bak", strings.Repeat("x", 1<<20)}, {ManifestName, okManifest}}, nil)
	if _, err := ReadWithLimits(bytes.NewReader(big), Limits{MaxTotalBytes: 20000}); !errors.Is(err, errLimit) {
		t.Fatalf("size err = %v", err)
	}
	many := rawTar(t, [][2]string{{SpecPrefix + "a", "1"}, {SpecPrefix + "b", "1"}, {ManifestName, okManifest}}, nil)
	if _, err := ReadWithLimits(bytes.NewReader(many), Limits{MaxEntries: 2}); err == nil {
		t.Fatal("entry cap not enforced")
	}
}

func TestSecretsEntryMustMatchManifestFlag(t *testing.T) {
	raw := rawTar(t, [][2]string{{SecretsName, "blob"}, {ManifestName, okManifest}}, nil)
	if _, err := Read(bytes.NewReader(raw)); err == nil {
		t.Fatal("secrets.enc with secrets_included false accepted")
	}
}

func TestOldFormatVersionFailsClearly(t *testing.T) {
	raw := rawTar(t, [][2]string{{ManifestName, `{"format":1,"kind":"levelrail.project-archive","files":[]}`}}, nil)
	if _, err := Read(bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "format version 1") {
		t.Fatalf("err = %v", err)
	}
}

func TestFilterForProjectAllowlist(t *testing.T) {
	spec := "kind: Project\nmetadata: {name: shop}\n---\nkind: App\nmetadata: {name: web}\nspec: {project: shop}\n---\nkind: Domain\nmetadata: {name: a.example.com}\nspec: {app: web}\n---\nkind: Domain\nmetadata: {name: b.example.com}\nspec: {app: other}\n---\nkind: Setting\nmetadata: {name: x}\n---\nkind: Project\nmetadata: {name: other}\n"
	out, skipped, err := FilterForProject(map[string][]byte{"a.yaml": []byte(spec)}, "shop")
	if err != nil {
		t.Fatal(err)
	}
	got := string(out["a.yaml"])
	if !strings.Contains(got, "a.example.com") || strings.Contains(got, "b.example.com") || strings.Contains(got, "Setting") || strings.Contains(got, "name: other") {
		t.Fatalf("filtered:\n%s", got)
	}
	if len(skipped) != 3 {
		t.Fatalf("skipped = %v", skipped)
	}
}

func TestOpenRejectsCraftedKDFCostBeforeScrypt(t *testing.T) {
	blob, err := seal(testPass, []byte("x"), []byte("m"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		idx  int
		val  byte
	}{{"large r", 6, 255}, {"large p", 7, 255}, {"large logN", 5, 20}} {
		bad := append([]byte{}, blob...)
		bad[c.idx] = c.val
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		_, err := open(testPass, bad, []byte("m"))
		runtime.ReadMemStats(&after)
		if !errors.Is(err, ErrFormat) {
			t.Fatalf("%s: err = %v, want ErrFormat", c.name, err)
		}
		if time.Since(start) > time.Second || after.TotalAlloc-before.TotalAlloc > 1<<20 {
			t.Fatalf("%s: header was not rejected cheaply", c.name)
		}
	}
}
