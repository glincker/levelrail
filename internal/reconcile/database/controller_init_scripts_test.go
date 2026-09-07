package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestController_Reconcile_InitScripts_MountedForSupportedEngine(t *testing.T) {
	rt := newFakeRuntime()
	scriptsDir := t.TempDir()
	fs := &fakeStore{
		db: &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"},
		initScripts: []store.DatabaseInitScript{
			{DatabaseName: "main", Filename: "01-init.sql", Content: "CREATE EXTENSION IF NOT EXISTS vector;"},
		},
	}
	c := New("main", fs, rt, WithInitScriptsDir(scriptsDir), WithPostgresCredentials(&PostgresCredentials{Username: "levelrail", Password: "s3cret"}))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(rt.lastCreateSpec.BindMounts) != 1 {
		t.Fatalf("BindMounts = %+v, want exactly 1", rt.lastCreateSpec.BindMounts)
	}
	bind := rt.lastCreateSpec.BindMounts[0]
	if bind.ContainerPath != "/docker-entrypoint-initdb.d" || !bind.ReadOnly {
		t.Errorf("bind mount = %+v, want ContainerPath=/docker-entrypoint-initdb.d ReadOnly=true", bind)
	}

	written, err := os.ReadFile(filepath.Join(bind.HostPath, "01-init.sql"))
	if err != nil {
		t.Fatalf("expected the init script to be written to disk at the bind mount's own host path: %v", err)
	}
	if string(written) != "CREATE EXTENSION IF NOT EXISTS vector;" {
		t.Errorf("written content = %q, want the stored script content verbatim", written)
	}
}

func TestController_Reconcile_InitScripts_NoneAttached_NoBindMount(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}
	c := New("main", &fakeStore{db: desired}, rt, WithInitScriptsDir(t.TempDir()), WithPostgresCredentials(&PostgresCredentials{Username: "levelrail", Password: "s3cret"}))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(rt.lastCreateSpec.BindMounts) != 0 {
		t.Errorf("BindMounts = %+v, want none when no init scripts are attached", rt.lastCreateSpec.BindMounts)
	}
}

func TestController_Reconcile_InitScripts_NotConfigured_NoBindMountEvenIfScriptsExist(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}
	fs := &fakeStore{db: desired, initScripts: []store.DatabaseInitScript{{DatabaseName: "main", Filename: "01-init.sql", Content: "x"}}}
	// WithInitScriptsDir deliberately not called: an operator has some
	// stored, but this control plane never opted into materializing them.
	c := New("main", fs, rt, WithPostgresCredentials(&PostgresCredentials{Username: "levelrail", Password: "s3cret"}))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(rt.lastCreateSpec.BindMounts) != 0 {
		t.Errorf("BindMounts = %+v, want none when WithInitScriptsDir was never set", rt.lastCreateSpec.BindMounts)
	}
}

func TestController_Reconcile_InitScripts_UnsupportedEngine_NeverMounted(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	fs := &fakeStore{db: desired, initScripts: []store.DatabaseInitScript{{DatabaseName: "main", Filename: "01-init.sh", Content: "x"}}}
	c := New("main", fs, rt, WithInitScriptsDir(t.TempDir()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(rt.lastCreateSpec.BindMounts) != 0 {
		t.Errorf("BindMounts = %+v, want none: redis's own official image has no /docker-entrypoint-initdb.d convention", rt.lastCreateSpec.BindMounts)
	}
}

func TestController_Reconcile_InitScripts_RemoteNode_SkippedNotMounted(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16", NodeID: "node-2"}
	fs := &fakeStore{db: desired, initScripts: []store.DatabaseInitScript{{DatabaseName: "main", Filename: "01-init.sql", Content: "x"}}}
	c := New("main", fs, rt, WithInitScriptsDir(t.TempDir()), WithPostgresCredentials(&PostgresCredentials{Username: "levelrail", Password: "s3cret"}))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(rt.lastCreateSpec.BindMounts) != 0 {
		t.Errorf("BindMounts = %+v, want none: a host path on the control plane's own disk is meaningless on a remote node's Docker daemon", rt.lastCreateSpec.BindMounts)
	}
}

func TestController_Reconcile_InitScripts_StoreErrorFailsReconcile(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}
	fs := &fakeStore{db: desired, initScriptsErr: context.DeadlineExceeded}
	c := New("main", fs, rt, WithInitScriptsDir(t.TempDir()), WithPostgresCredentials(&PostgresCredentials{Username: "levelrail", Password: "s3cret"}))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the list-init-scripts failure to surface")
	}
	cond := conditionOf(t, result)
	if cond.Reason != "InitScriptsFailed" {
		t.Errorf("Reason = %q, want InitScriptsFailed", cond.Reason)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: a failure resolving init scripts must not proceed to create the container", rt.createCalls)
	}
}

func TestMaterializeInitScripts_WritesMultipleScriptsSortedByFilename(t *testing.T) {
	dir := t.TempDir()
	fs := &fakeStore{
		db: &store.DesiredDatabase{Name: "main"},
		initScripts: []store.DatabaseInitScript{
			{DatabaseName: "main", Filename: "01-extensions.sql", Content: "a"},
			{DatabaseName: "main", Filename: "02-roles.sql", Content: "b"},
		},
	}
	c := New("main", fs, newFakeRuntime(), WithInitScriptsDir(dir))

	got, err := c.materializeInitScripts(context.Background())
	if err != nil {
		t.Fatalf("materializeInitScripts() error = %v", err)
	}
	if got != filepath.Join(dir, "main") {
		t.Errorf("dir = %q, want %q", got, filepath.Join(dir, "main"))
	}
	for filename, want := range map[string]string{"01-extensions.sql": "a", "02-roles.sql": "b"} {
		content, err := os.ReadFile(filepath.Join(got, filename)) //nolint:gosec // reading back this test's own t.TempDir() output, not caller input
		if err != nil {
			t.Fatalf("read %q: %v", filename, err)
		}
		if string(content) != want {
			t.Errorf("%s content = %q, want %q", filename, content, want)
		}
	}
}

func TestMaterializeInitScripts_NotConfigured_ReturnsEmptyNoError(t *testing.T) {
	fs := &fakeStore{db: &store.DesiredDatabase{Name: "main"}, initScripts: []store.DatabaseInitScript{{DatabaseName: "main", Filename: "x.sql", Content: "y"}}}
	c := New("main", fs, newFakeRuntime()) // no WithInitScriptsDir

	got, err := c.materializeInitScripts(context.Background())
	if err != nil {
		t.Fatalf("materializeInitScripts() error = %v", err)
	}
	if got != "" {
		t.Errorf("dir = %q, want empty", got)
	}
}
