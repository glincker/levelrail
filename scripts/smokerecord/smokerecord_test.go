package smokerecord

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func pinned() catalog.Template {
	return catalog.Template{ID: "demo", Compose: "services:\n  web:\n    image: example/web:1.2.3\n"}
}

func TestApplyIsDeterministicAndMergesArches(t *testing.T) {
	dir := t.TempDir()
	v, l := filepath.Join(dir, "verified.json"), filepath.Join(dir, "images.lock.json")
	run := Run{Arch: "arm64", HTTPStatus: 200, Digests: map[string]string{"example/web:1.2.3": "sha256:aa"}}
	if err := Apply(v, l, pinned(), run); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(v) //nolint:gosec // test temp path
	if err := Apply(v, l, pinned(), run); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(v) //nolint:gosec // test temp path
	if string(again) != string(first) {
		t.Error("second identical apply changed the file")
	}
	run.Arch = "amd64"
	if err := Apply(v, l, pinned(), run); err != nil {
		t.Fatal(err)
	}
	var m map[string]catalog.VerifiedRecord
	raw, _ := os.ReadFile(v) //nolint:gosec // test temp path
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	rec := m["demo"]
	if len(rec.Arches) != 2 || rec.Arches[0] != "amd64" || rec.Digest != "sha256:aa" || rec.Tag != "1.2.3" {
		t.Errorf("record = %+v", rec)
	}
	var lock catalog.LockFile
	raw, _ = os.ReadFile(l) //nolint:gosec // test temp path
	if err := json.Unmarshal(raw, &lock); err != nil || lock.Images["example/web:1.2.3"].Digest != "sha256:aa" {
		t.Errorf("lock = %+v, %v", lock, err)
	}
}

func TestApplyRejectsUnpinned(t *testing.T) {
	bad := catalog.Template{ID: "x", Compose: "services:\n  web:\n    image: example/web:latest\n"}
	if err := Apply(filepath.Join(t.TempDir(), "v.json"), filepath.Join(t.TempDir(), "l.json"), bad, Run{Arch: "amd64"}); err == nil {
		t.Error("unpinned template was recorded")
	}
}

func TestParseRefs(t *testing.T) {
	got := ParseRefs([]byte("a/b:1 sha256:ff\nbroken\nc:2 notadigest\n"))
	if len(got) != 1 || got["a/b:1"] != "sha256:ff" {
		t.Errorf("ParseRefs = %v", got)
	}
}
