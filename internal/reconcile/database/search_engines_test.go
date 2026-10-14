package database

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestController_Reconcile_SearchEngines(t *testing.T) {
	tests := []struct {
		engine, version, image, volumePath, keyEnv string
		port                                       int
		wantEnv                                    map[string]string
		entrypoint                                 bool
	}{
		{
			engine: store.EngineMeilisearch, version: "v1.55", image: "getmeili/meilisearch:v1.55",
			volumePath: "/meili_data", keyEnv: "MEILI_MASTER_KEY", port: 7700,
			wantEnv: map[string]string{"MEILI_ENV": "production", "MEILI_NO_ANALYTICS": "true", "MEILI_DB_PATH": "/meili_data/data.ms", "MEILI_DUMP_DIR": "/meili_data/dumps"},
		},
		{
			engine: store.EngineQdrant, version: "v1.19", image: "qdrant/qdrant:v1.19",
			volumePath: "/qdrant/storage", keyEnv: "QDRANT__SERVICE__API_KEY", port: 6333,
			wantEnv: map[string]string{"QDRANT__STORAGE__STORAGE_PATH": "/qdrant/storage/data", "QDRANT__STORAGE__SNAPSHOTS_PATH": "/qdrant/storage/snapshots", "QDRANT__TELEMETRY_DISABLED": "true"},
		},
		{
			engine: store.EngineTypesense, version: "30.2", image: "typesense/typesense:30.2",
			volumePath: "/data", keyEnv: "TYPESENSE_API_KEY", port: 8108, entrypoint: true,
			wantEnv: map[string]string{"TYPESENSE_DATA_DIR": "/data/db"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.engine, func(t *testing.T) {
			rt := newFakeRuntime()
			desired := &store.DesiredDatabase{Name: "search", Engine: tc.engine, Version: tc.version, PubliclyAccessible: true, PublicPort: 17700}
			c := New("search", &fakeStore{db: desired}, rt, WithAPIKeyCredentials(&APIKeyCredentials{Key: "k3y"}))
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition = %+v", cond)
			}
			spec := rt.lastCreateSpec
			if spec.Image != tc.image {
				t.Errorf("image = %q, want %q", spec.Image, tc.image)
			}
			if len(spec.Volumes) != 1 || spec.Volumes[0].ContainerPath != tc.volumePath {
				t.Errorf("volumes = %+v, want one at %s", spec.Volumes, tc.volumePath)
			}
			if spec.Env[tc.keyEnv] != "k3y" {
				t.Errorf("%s not set from the generated key", tc.keyEnv)
			}
			for k, v := range tc.wantEnv {
				if spec.Env[k] != v {
					t.Errorf("env %s = %q, want %q", k, spec.Env[k], v)
				}
			}
			script := spec.Command
			if tc.entrypoint {
				script = spec.Entrypoint
				if spec.Command != nil {
					t.Errorf("typesense command = %q, want nil under the replaced entrypoint", spec.Command)
				}
			}
			if len(script) != 3 || script[0] != "/bin/sh" || !strings.Contains(script[2], "exec ") {
				t.Errorf("start command = %q, want the boot script", script)
			}
			if strings.Contains(strings.Join(script, " "), "k3y") || spec.Labels[engineConfigLabelKey] == "" {
				t.Error("the key must never be in the command, and the config label must be set")
			}
			if len(spec.Ports) != 1 || spec.Ports[0].ContainerPort != tc.port || spec.Ports[0].HostIP != "127.0.0.1" {
				t.Errorf("ports = %+v, want the API port bound to loopback only", spec.Ports)
			}
		})
	}
}

func TestController_Reconcile_SearchEngine_NoKeyIsBlocked(t *testing.T) {
	rt := newFakeRuntime()
	c := New("search", &fakeStore{db: &store.DesiredDatabase{Name: "search", Engine: store.EngineQdrant, Version: "v1.19"}}, rt)
	res, err := c.Reconcile(context.Background())
	if err != nil || conditionOf(t, res).Reason != "CredentialsNotConfigured" || rt.createCalls != 0 {
		t.Fatalf("res = %+v err = %v creates = %d", res, err, rt.createCalls)
	}
}

func TestController_Reconcile_SearchEngine_ConfigLabelDrivesReplace(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "search", Engine: store.EngineMeilisearch, Version: "v1.55"}
	c := New("search", &fakeStore{db: desired}, rt, WithAPIKeyCredentials(&APIKeyCredentials{Key: "k"}))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	label := rt.lastCreateSpec.Labels[engineConfigLabelKey]
	rt.containers[containerName("search")].Labels = map[string]string{engineConfigLabelKey: label}
	if _, err := c.Reconcile(context.Background()); err != nil || rt.createCalls != 1 {
		t.Fatalf("an unchanged config must be a no-op: err = %v creates = %d", err, rt.createCalls)
	}
	rt.containers[containerName("search")].Labels = map[string]string{engineConfigLabelKey: "stale"}
	if _, err := c.Reconcile(context.Background()); err != nil || rt.createCalls != 2 {
		t.Fatalf("a changed config must replace: err = %v creates = %d", err, rt.createCalls)
	}
}

// runBoot runs the real boot script under /bin/sh against a temp dir, with
// a fake engine binary that records its arguments.
func runBoot(t *testing.T, l SearchLayout) string {
	t.Helper()
	argsFile := filepath.Join(l.VolumePath, "engine-args")
	bin := filepath.Join(t.TempDir(), "engine")
	if err := os.WriteFile(bin, []byte("echo \"$*\" > "+shQuote(argsFile)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l.binary = "/bin/sh " + shQuote(bin)
	out, err := exec.Command("/bin/sh", "-c", l.BootScript()).CombinedOutput() //nolint:gosec // the script under test, built from a temp dir layout
	if err != nil {
		t.Fatalf("boot script failed: %v\n%s", err, out)
	}
	args, _ := os.ReadFile(filepath.Clean(argsFile))
	return strings.TrimSpace(string(args))
}

func testLayout(t *testing.T, engine string) SearchLayout {
	l := searchLayouts[engine]
	root := t.TempDir()
	l.DataDir = filepath.Join(root, filepath.Base(l.DataDir))
	l.VolumePath = root
	return l
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(path string) string {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestBootScript_ImportThenRollback(t *testing.T) {
	l := testLayout(t, store.EngineMeilisearch)
	write(t, filepath.Join(l.DataDir, "old"), "old-data")
	write(t, l.PendingPath(), "dump")

	if args := runBoot(t, l); args != "--import-dump "+l.ImportingPath() {
		t.Fatalf("import boot args = %q", args)
	}
	if exists(l.PendingPath()) || !exists(l.ImportingPath()) || read(filepath.Join(l.PreviousPath(), "old")) != "old-data" || exists(l.DataDir) {
		t.Fatal("import boot must stash the old data and mark the dump importing")
	}
	if strings.Count(read(l.BootsPath()), "boot") != 1 {
		t.Fatalf("boots after import = %q", read(l.BootsPath()))
	}

	write(t, filepath.Join(l.DataDir, "partial"), "x")
	if args := runBoot(t, l); args != "" {
		t.Fatalf("a reboot must not import again, args = %q", args)
	}
	if strings.Count(read(l.BootsPath()), "boot") != 2 {
		t.Fatalf("a reboot of an unconfirmed restore must be counted, boots = %q", read(l.BootsPath()))
	}
	if !exists(filepath.Join(l.DataDir, "partial")) {
		t.Fatal("a reboot without a rollback request must not touch the data")
	}

	write(t, l.RollbackPath(), "")
	runBoot(t, l)
	if read(filepath.Join(l.DataDir, "old")) != "old-data" || exists(filepath.Join(l.DataDir, "partial")) {
		t.Fatal("rollback must put the previous data back")
	}
	for _, p := range []string{l.ImportingPath(), l.PreviousPath(), l.RollbackPath(), l.BootsPath()} {
		if exists(p) {
			t.Fatalf("%s left behind after rollback", p)
		}
	}
}

func TestBootScript_RollbackOfRestoreIntoEmptyLeavesNoData(t *testing.T) {
	l := testLayout(t, store.EngineQdrant)
	write(t, l.PendingPath(), "snap")
	if args := runBoot(t, l); args != "--storage-snapshot "+l.ImportingPath() {
		t.Fatalf("args = %q", args)
	}
	write(t, filepath.Join(l.DataDir, "imported"), "x")
	write(t, l.RollbackPath(), "")
	runBoot(t, l)
	if exists(filepath.Join(l.DataDir, "imported")) || exists(l.PreviousPath()) {
		t.Fatal("rolling back a restore into an empty database must leave it empty")
	}
}

func TestBootScript_CrashAfterStashKeepsPreviousData(t *testing.T) {
	l := testLayout(t, store.EngineMeilisearch)
	write(t, filepath.Join(l.PreviousPath(), "old"), "old-data")
	write(t, l.PendingPath(), "dump")
	runBoot(t, l)
	if read(filepath.Join(l.PreviousPath(), "old")) != "old-data" {
		t.Fatal("a rerun after a crash between stash and import must not delete the stashed data")
	}
}

func TestBootScript_TypesenseSwapsStagedSnapshot(t *testing.T) {
	l := testLayout(t, store.EngineTypesense)
	write(t, filepath.Join(l.DataDir, "old"), "old-data")
	write(t, filepath.Join(l.StagingPath(), "state", "snap"), "new-data")
	write(t, l.PendingPath(), "")
	if args := runBoot(t, l); args != "" {
		t.Fatalf("typesense loads its data dir, args = %q", args)
	}
	if read(filepath.Join(l.DataDir, "state", "snap")) != "new-data" || read(filepath.Join(l.PreviousPath(), "old")) != "old-data" || exists(l.StagingPath()) {
		t.Fatal("staged snapshot must become the data dir, previous kept aside")
	}

	confirm := exec.Command("/bin/sh", "-c", l.ConfirmRestoreScript()) //nolint:gosec // the script under test
	if out, err := confirm.CombinedOutput(); err != nil {
		t.Fatalf("confirm: %v %s", err, out)
	}
	if exists(l.ImportingPath()) || exists(l.PreviousPath()) || exists(l.BootsPath()) {
		t.Fatal("confirm must drop the markers and the previous data")
	}
	runBoot(t, l)
	if read(filepath.Join(l.DataDir, "state", "snap")) != "new-data" || exists(l.BootsPath()) {
		t.Fatal("a confirmed restore must survive a reboot untouched")
	}
}

func TestConventionalEnvVar(t *testing.T) {
	tests := []struct{ engine, field, want string }{
		{store.EngineMeilisearch, "url", "MEILISEARCH_URL"},
		{store.EngineMeilisearch, "password", "MEILISEARCH_API_KEY"},
		{store.EngineQdrant, "url", "QDRANT_URL"},
		{store.EngineQdrant, "password", "QDRANT_API_KEY"},
		{store.EngineTypesense, "host", "TYPESENSE_HOST"},
		{store.EngineTypesense, "port", "TYPESENSE_PORT"},
		{store.EngineTypesense, "password", "TYPESENSE_API_KEY"},
		{store.EngineTypesense, "username", ""},
		{store.EnginePostgres, "url", ""},
	}
	for _, tc := range tests {
		if got := ConventionalEnvVar(tc.engine, tc.field); got != tc.want {
			t.Errorf("ConventionalEnvVar(%s, %s) = %q, want %q", tc.engine, tc.field, got, tc.want)
		}
	}
}

func TestSearchEngineVersions(t *testing.T) {
	valid := []struct{ engine, version string }{
		{store.EngineMeilisearch, "v1.55"}, {store.EngineMeilisearch, "v1.55.0"},
		{store.EngineQdrant, "v1.19.2"}, {store.EngineTypesense, "30.2"},
	}
	for _, tc := range valid {
		if err := ValidateEngineVersion(tc.engine, tc.version); err != nil {
			t.Errorf("ValidateEngineVersion(%s, %s) = %v", tc.engine, tc.version, err)
		}
	}
	invalid := []struct{ engine, version string }{
		{store.EngineMeilisearch, "latest"}, {store.EngineMeilisearch, "1.55"},
		{store.EngineQdrant, "latest"}, {store.EngineTypesense, "latest"}, {store.EngineTypesense, "v30.2"},
	}
	for _, tc := range invalid {
		if err := ValidateEngineVersion(tc.engine, tc.version); err == nil {
			t.Errorf("ValidateEngineVersion(%s, %s) = nil, want refused", tc.engine, tc.version)
		}
	}

	changes := []struct {
		engine, from, to string
		ok               bool
	}{
		{store.EngineMeilisearch, "v1.55", "v1.55.0", true},
		{store.EngineMeilisearch, "v1.54.3", "v1.55.0", false},
		{store.EngineQdrant, "v1.18.3", "v1.19.2", true},
		{store.EngineQdrant, "v1.17.1", "v1.19.2", false},
		{store.EngineQdrant, "v1.19.2", "v1.18.3", false},
		{store.EngineTypesense, "29.1", "30.2", true},
		{store.EngineTypesense, "30.2", "29.1", false},
	}
	for _, tc := range changes {
		if err := CheckInPlaceVersionChange(tc.engine, tc.from, tc.to); (err == nil) != tc.ok {
			t.Errorf("CheckInPlaceVersionChange(%s, %s, %s) = %v, want ok=%v", tc.engine, tc.from, tc.to, err, tc.ok)
		}
	}
}

func TestSearchEngineAddressing(t *testing.T) {
	tests := []struct {
		engine string
		port   int
		key    string
	}{
		{store.EngineMeilisearch, 7700, MeilisearchAPIKeyEnvKey},
		{store.EngineQdrant, 6333, QdrantAPIKeyEnvKey},
		{store.EngineTypesense, 8108, TypesenseAPIKeyEnvKey},
	}
	for _, tc := range tests {
		if p, ok := ContainerPort(tc.engine); !ok || p != tc.port {
			t.Errorf("ContainerPort(%s) = %d, %v", tc.engine, p, ok)
		}
		if k, ok := PasswordSecretKey(tc.engine); !ok || k != tc.key {
			t.Errorf("PasswordSecretKey(%s) = %q, %v", tc.engine, k, ok)
		}
		if SupportsField(tc.engine, "username") || SupportsField(tc.engine, "database") || !SupportsField(tc.engine, "password") || SupportsTLS(tc.engine) {
			t.Errorf("field support for %s is wrong", tc.engine)
		}
	}
}
