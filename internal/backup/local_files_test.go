package backup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalTargetRoundTripAndConfinement(t *testing.T) {
	root := t.TempDir()
	r := Routed{LocalRoot: root}
	d := Destination{Endpoint: LocalEndpointScheme, Bucket: "dumps"}
	ctx := context.Background()

	if err := r.Test(ctx, d); err != nil {
		t.Fatalf("test: %v", err)
	}
	if err := r.Upload(ctx, d, "db/a.dump", strings.NewReader("data"), 4); err != nil {
		t.Fatalf("upload: %v", err)
	}
	rc, err := r.Download(ctx, d, "db/a.dump")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "data" {
		t.Fatalf("got %q", b)
	}
	objs, err := r.List(ctx, d, "db/")
	if err != nil || len(objs) != 1 || objs[0].Key != "db/a.dump" {
		t.Fatalf("list: %v %v", objs, err)
	}
	if err := r.Delete(ctx, d, "db/a.dump"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := r.Upload(ctx, d, "../../escape", strings.NewReader("x"), 1); err == nil {
		t.Fatal("key traversal must be refused")
	}
	if err := r.Upload(ctx, Destination{Endpoint: LocalEndpointScheme, Bucket: "../out"}, "k", strings.NewReader("x"), 1); err == nil {
		t.Fatal("bucket traversal must be refused")
	}
	if err := (Routed{}).Upload(ctx, d, "k", strings.NewReader("x"), 1); !errors.Is(err, ErrLocalRootUnset) {
		t.Fatalf("want ErrLocalRootUnset, got %v", err)
	}
}

func TestLocalRootDefaultsUnderDataDir(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	if got := LocalRootFromEnv(env(map[string]string{"APP_DATA_DIR": "/srv/lr"})); got != "/srv/lr/backups" {
		t.Fatalf("default root = %q", got)
	}
	if got := LocalRootFromEnv(env(map[string]string{"APP_DATA_DIR": "/srv/lr", EnvLocalRoot: "/mnt/b"})); got != "/mnt/b" {
		t.Fatalf("explicit root = %q", got)
	}
	if got := LocalRootFromEnv(env(map[string]string{EnvLocalRoot: "rel"})); got != "" {
		t.Fatalf("relative root must be refused, got %q", got)
	}
}

func TestLocalPathValidationRejects(t *testing.T) {
	for _, p := range []string{"", ".", "/etc", "../x", "a/../../x"} {
		if _, err := CleanLocalPath(p); err == nil {
			t.Fatalf("%q must be rejected", p)
		}
	}
}

func TestLocalUploadPrivateDirsAndDiskRefusal(t *testing.T) {
	root := t.TempDir()
	r := Routed{LocalRoot: root}
	d := Destination{Endpoint: LocalEndpointScheme, Bucket: "dumps"}
	ctx := context.Background()
	if err := r.Upload(ctx, d, "db/a.dump", strings.NewReader("x"), 1); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "dumps", "db"))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v err %v", info, err)
	}
	orig := diskFreeFunc
	t.Cleanup(func() { diskFreeFunc = orig })
	diskFreeFunc = func(string) (int64, error) { return 1, nil }
	if err := r.Upload(ctx, d, "db/b.dump", strings.NewReader("x"), 1); err == nil || !strings.Contains(err.Error(), "insufficient disk space") {
		t.Fatalf("want disk refusal, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dumps", "db", "b.dump")); err == nil {
		t.Fatal("refused upload must not leave an object")
	}
}
