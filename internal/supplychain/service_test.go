package supplychain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeRunner struct {
	mu         sync.Mutex
	stdout     []byte
	exit       int64
	runErr     error
	ensureErr  error
	specs      []docker.OneShotSpec
	containers []docker.LabeledContainer
	removed    []string
}

func (f *fakeRunner) EnsureImageID(context.Context, string) (string, bool, error) {
	return "sha256:img", true, f.ensureErr
}

func (f *fakeRunner) RunOneShot(_ context.Context, spec docker.OneShotSpec) (docker.OneShotResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.specs = append(f.specs, spec)
	if f.runErr != nil {
		return docker.OneShotResult{}, f.runErr
	}
	return docker.OneShotResult{ExitCode: f.exit, Stdout: f.stdout, Stderr: []byte("boom")}, nil
}

func (f *fakeRunner) ListContainersByLabel(context.Context, string) ([]docker.LabeledContainer, error) {
	return f.containers, nil
}

func (f *fakeRunner) Remove(_ context.Context, id string, _ bool) error {
	f.removed = append(f.removed, id)
	return nil
}

type fakeResolver struct {
	apps     map[string]bool
	attempts map[string]bool
}

func (r fakeResolver) AppExists(_ context.Context, app string) (bool, error) { return r.apps[app], nil }
func (r fakeResolver) AttemptExists(_ context.Context, id string) (bool, error) {
	return r.attempts[id], nil
}

type harness struct {
	svc      *Service
	sql      *SQLStore
	runner   *fakeRunner
	resolver fakeResolver
	dir      string
	now      time.Time
}

func newHarness(t *testing.T, mutate func(*Config)) *harness {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	h := &harness{
		sql: NewSQLStore(db), runner: &fakeRunner{stdout: fixture(t, "trivy.json")}, dir: t.TempDir(),
		resolver: fakeResolver{apps: map[string]bool{"web": true, "api": true}, attempts: map[string]bool{}},
		now:      time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC),
	}
	cfg := ConfigFromEnv(func(string) (string, bool) { return "", false }, nil)
	cfg.KeepPerApp = 2
	if mutate != nil {
		mutate(&cfg)
	}
	h.svc = New(cfg, Deps{Store: h.sql, Resolver: h.resolver, Runner: h.runner, DataDir: h.dir, Namespace: "lr"}, WithClock(func() time.Time { return h.now }))
	return h
}

func (h *harness) attempt(id string) { h.resolver.attempts[id] = true }

func (h *harness) settings(t *testing.T, enabled bool, gate string) {
	t.Helper()
	if _, err := h.svc.SaveSettings(context.Background(), "web", SettingsPatch{Enabled: &enabled, Gate: &gate}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
}

func (h *harness) sbomFiles(t *testing.T) int {
	t.Helper()
	files, err := h.svc.fs.List()
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

func attest(t *testing.T) build.Attestations {
	t.Helper()
	return build.Attestations{SBOM: fixture(t, "spdx.json"), Provenance: []byte(`{"buildType":"x"}`)}
}

func TestAfterBuild_StoresSBOMWithoutScanning(t *testing.T) {
	h := newHarness(t, nil)
	if err := h.svc.AfterBuild(context.Background(), "web", "da_1", attest(t)); err != nil {
		t.Fatal(err)
	}
	rec, err := h.svc.Get(context.Background(), "web", "da_1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Summary.PackageCount != 4 || !rec.HasProvenance || !rec.HasSBOM() || rec.ScanStatus != ScanNone {
		t.Errorf("record = %+v", rec)
	}
	if len(h.runner.specs) != 0 {
		t.Error("scanning is off by default: no scanner container may run")
	}
	data, _, err := h.svc.OpenSBOM(context.Background(), "web", "da_1")
	if err != nil || len(data) == 0 {
		t.Errorf("open sbom: %v", err)
	}
	if _, err := h.svc.Get(context.Background(), "api", "da_1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("another app must not read this record, got %v", err)
	}
}

func TestAfterBuild_NoSBOMIsNotAnError(t *testing.T) {
	h := newHarness(t, nil)
	h.settings(t, true, "block_on_critical")
	if err := h.svc.AfterBuild(context.Background(), "web", "da_1", build.Attestations{}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Get(context.Background(), "web", "da_1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected no record, got %v", err)
	}
	if err := h.svc.AfterBuild(context.Background(), "web", "", attest(t)); err != nil {
		t.Fatalf("no attempt id: %v", err)
	}
}

func TestAfterBuild_GateTable(t *testing.T) {
	tests := []struct {
		name       string
		gate       string
		enabled    bool
		serverOff  bool
		runErr     error
		stdout     func(*testing.T) []byte
		wantBlock  bool
		wantScans  int
		wantAction string
		wantStatus string
	}{
		{name: "gate off scans but never blocks", gate: "off", enabled: true, wantScans: 1, wantAction: ActionAllow, wantStatus: ScanOK},
		{name: "warn records the warning", gate: "warn", enabled: true, wantScans: 1, wantAction: ActionWarn, wantStatus: ScanOK},
		{name: "block on critical blocks", gate: "block_on_critical", enabled: true, wantBlock: true, wantScans: 1, wantAction: ActionBlock, wantStatus: ScanOK},
		{name: "scan disabled for the app", gate: "off", enabled: false, wantScans: 0},
		{name: "server kill switch", gate: "block_on_critical", enabled: true, serverOff: true, wantScans: 0},
		{name: "scanner failure fails open", gate: "block_on_critical", enabled: true, runErr: errors.New("daemon down"), wantScans: 1, wantAction: ActionAllow, wantStatus: ScanFailed},
		{name: "clean image passes the gate", gate: "block_on_critical", enabled: true, stdout: func(*testing.T) []byte { return []byte(`{"Results":[]}`) }, wantScans: 1, wantAction: ActionAllow, wantStatus: ScanOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, func(c *Config) { c.Enabled = !tc.serverOff })
			if tc.serverOff {
				h.svc.cfg.Enabled = true
				h.settings(t, tc.enabled, tc.gate)
				h.svc.cfg.Enabled = false
			} else {
				h.settings(t, tc.enabled, tc.gate)
			}
			h.runner.runErr = tc.runErr
			if tc.stdout != nil {
				h.runner.stdout = tc.stdout(t)
			}
			err := h.svc.AfterBuild(context.Background(), "web", "da_1", attest(t))
			var blocked *BlockedError
			if got := errors.As(err, &blocked); got != tc.wantBlock {
				t.Fatalf("blocked = %v (%v), want %v", got, err, tc.wantBlock)
			}
			if len(h.runner.specs) != tc.wantScans {
				t.Errorf("scans = %d, want %d", len(h.runner.specs), tc.wantScans)
			}
			rec, gerr := h.svc.Get(context.Background(), "web", "da_1")
			if gerr != nil {
				t.Fatalf("the SBOM is recorded regardless of the gate: %v", gerr)
			}
			if rec.GateAction != tc.wantAction || rec.ScanStatus != tc.wantStatus {
				t.Errorf("gate %q status %q, want %q %q", rec.GateAction, rec.ScanStatus, tc.wantAction, tc.wantStatus)
			}
		})
	}
}

func TestAfterBuild_ScannerContainerIsCappedAndLabeled(t *testing.T) {
	h := newHarness(t, nil)
	h.settings(t, true, "warn")
	if err := h.svc.AfterBuild(context.Background(), "web", "da_1", attest(t)); err != nil {
		t.Fatal(err)
	}
	spec := h.runner.specs[0]
	if spec.Labels["lr.role"] != roleValue || spec.MemoryBytes == 0 || spec.NanoCPUs == 0 || spec.PidsLimit == 0 {
		t.Errorf("spec = %+v", spec)
	}
	if len(spec.Files[sbomPathInContainer]) == 0 || spec.Volumes["lr-scanner-cache"] != cachePath {
		t.Errorf("sbom must be copied in and the db cache mounted: %+v", spec)
	}
}

func TestAfterBuild_ScannerExitCodeFailsOpen(t *testing.T) {
	h := newHarness(t, nil)
	h.settings(t, true, "block_on_critical")
	h.runner.exit = 2
	if err := h.svc.AfterBuild(context.Background(), "web", "da_1", attest(t)); err != nil {
		t.Fatalf("scanner failure must not block: %v", err)
	}
	rec, _ := h.svc.Get(context.Background(), "web", "da_1")
	if rec.ScanStatus != ScanFailed || rec.ScanError == "" {
		t.Errorf("record = %+v", rec)
	}
}

func TestOverride_IsOneShotAndNeedsReason(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	h.settings(t, true, "block_on_critical")
	if _, err := h.svc.ArmOverride(ctx, "web", "  "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty reason: %v", err)
	}
	if _, err := h.svc.ArmOverride(ctx, "web", "customer outage, fix ships next week"); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); err != nil {
		t.Fatalf("armed override lets the release through: %v", err)
	}
	rec, _ := h.svc.Get(ctx, "web", "da_1")
	if rec.GateAction != ActionOverride {
		t.Errorf("action = %q", rec.GateAction)
	}
	var blocked *BlockedError
	if err := h.svc.AfterBuild(ctx, "web", "da_2", attest(t)); !errors.As(err, &blocked) {
		t.Fatalf("the override is consumed by one release, got %v", err)
	}
}

func TestOverride_ExpiresAndOnlyForBlockMode(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	h.settings(t, true, "warn")
	if _, err := h.svc.ArmOverride(ctx, "web", "why"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("override needs block mode: %v", err)
	}
	h.settings(t, true, "block_on_critical")
	if _, err := h.svc.ArmOverride(ctx, "web", "why"); err != nil {
		t.Fatal(err)
	}
	h.now = h.now.Add(2 * time.Hour)
	var blocked *BlockedError
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); !errors.As(err, &blocked) {
		t.Fatalf("an expired override must not apply, got %v", err)
	}
}

func TestSaveSettings_GateNeedsScanning(t *testing.T) {
	h := newHarness(t, nil)
	on, gate := false, "warn"
	if _, err := h.svc.SaveSettings(context.Background(), "web", SettingsPatch{Enabled: &on, Gate: &gate}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("gate without scanning: %v", err)
	}
	bad := "sometimes"
	if _, err := h.svc.SaveSettings(context.Background(), "web", SettingsPatch{Gate: &bad}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad gate: %v", err)
	}
}

func TestScanNow(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	if _, err := h.svc.ScanNow(ctx, "web", "da_1"); !errors.Is(err, ErrNotEnabled) {
		t.Fatalf("not enabled: %v", err)
	}
	h.settings(t, true, "off")
	if _, err := h.svc.ScanNow(ctx, "web", "da_1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no record: %v", err)
	}
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); err != nil {
		t.Fatal(err)
	}
	h.runner.stdout = fixture(t, "grype.json")
	h.svc.cfg.Scanner = ScannerGrype
	rec, err := h.svc.ScanNow(ctx, "web", "da_1")
	if err != nil || rec.Scan == nil || rec.Scan.Counts.Critical != 1 || rec.Scan.Scanner != ScannerGrype {
		t.Fatalf("rescan = %+v, %v", rec, err)
	}
	h.svc.cfg.Enabled = false
	if _, err := h.svc.ScanNow(ctx, "web", "da_1"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("kill switch: %v", err)
	}
}

func TestScanNow_RetentionRemovedTheDocument(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	h.settings(t, true, "off")
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.fs.Remove("web", "da_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.ScanNow(ctx, "web", "da_1"); !errors.Is(err, ErrNoSBOM) {
		t.Fatalf("expected ErrNoSBOM, got %v", err)
	}
}

func TestScanNow_FailureKeepsThePreviousRecordAndReportsIt(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	h.settings(t, true, "off")
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); err != nil {
		t.Fatal(err)
	}
	h.runner.runErr = errors.New("cannot reach docker")
	rec, err := h.svc.ScanNow(ctx, "web", "da_1")
	if err == nil || rec.AttemptID != "da_1" || rec.ScanStatus != ScanFailed {
		t.Fatalf("rec %+v err %v", rec, err)
	}
}

func TestLookupAndList(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	for _, id := range []string{"da_1", "da_2"} {
		if err := h.svc.AfterBuild(ctx, "web", id, attest(t)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := h.svc.Lookup(ctx, []string{"da_1", "da_2", "da_missing"})
	if err != nil || len(got) != 2 {
		t.Fatalf("lookup = %v, %v", got, err)
	}
	if recs, _ := h.svc.List(ctx, "web"); len(recs) != 2 {
		t.Errorf("list = %d", len(recs))
	}
}

func TestSweep_RetentionKeepsNewestAndTheMetadata(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	for i, id := range []string{"da_1", "da_2", "da_3"} {
		h.attempt(id)
		h.now = time.Date(2026, 9, 26, 12, i, 0, 0, time.UTC)
		if err := h.svc.AfterBuild(ctx, "web", id, attest(t)); err != nil {
			t.Fatal(err)
		}
	}
	h.svc.Sweep(ctx)
	if n := h.sbomFiles(t); n != 2 {
		t.Fatalf("files = %d, want the newest 2", n)
	}
	old, err := h.svc.Get(ctx, "web", "da_1")
	if err != nil || old.HasSBOM() || old.Summary.PackageCount != 4 {
		t.Fatalf("oldest keeps its counts but loses the document: %+v, %v", old, err)
	}
	if _, _, err := h.svc.OpenSBOM(ctx, "web", "da_1"); !errors.Is(err, ErrNoSBOM) {
		t.Errorf("open pruned sbom: %v", err)
	}
	if kept, _ := h.svc.Get(ctx, "web", "da_3"); !kept.HasSBOM() {
		t.Error("the newest document must stay")
	}
}

func TestSweep_RemovesRecordsOfDeletedAppsAndDeploys(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	h.attempt("da_web")
	h.attempt("da_gone_app")
	for app, id := range map[string]string{"web": "da_web", "ghost": "da_gone_app", "api": "da_no_attempt"} {
		if err := h.svc.AfterBuild(ctx, app, id, attest(t)); err != nil {
			t.Fatal(err)
		}
	}
	h.svc.Sweep(ctx)
	if _, err := h.svc.Get(ctx, "web", "da_web"); err != nil {
		t.Errorf("live deploy must survive: %v", err)
	}
	for app, id := range map[string]string{"ghost": "da_gone_app", "api": "da_no_attempt"} {
		if _, err := h.svc.Get(ctx, app, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s/%s should be gone: %v", app, id, err)
		}
	}
	if n := h.sbomFiles(t); n != 1 {
		t.Errorf("files = %d, want 1", n)
	}
}

func TestSweep_OrphanFilesAndDanglingRecords(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	h.attempt("da_1")
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.fs.Write("web", "da_stale", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.fs.Write("web", "da_fresh", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	stale, _ := h.svc.fs.Path("web", "da_stale")
	old := h.now.Add(-time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	fresh, _ := h.svc.fs.Path("web", "da_fresh")
	if err := os.Chtimes(fresh, h.now, h.now); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.fs.Remove("web", "da_1"); err != nil {
		t.Fatal(err)
	}
	h.svc.Sweep(ctx)
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Error("an old orphan file must be removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("a file younger than the grace period may still be waiting for its record")
	}
	if rec, _ := h.svc.Get(ctx, "web", "da_1"); rec.HasSBOM() {
		t.Error("a record whose file vanished must be demoted")
	}
}

func TestSweep_OrphanContainersOnlyWhenOld(t *testing.T) {
	h := newHarness(t, nil)
	h.runner.containers = []docker.LabeledContainer{
		{ID: "old", Name: "lr-scan-old", Created: h.now.Add(-time.Hour)},
		{ID: "young", Name: "lr-scan-young", Created: h.now.Add(-time.Minute)},
	}
	h.svc.Sweep(context.Background())
	if len(h.runner.removed) != 1 || h.runner.removed[0] != "old" {
		t.Errorf("removed = %v", h.runner.removed)
	}
}

func TestDeleteApp_RemovesEverything(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	h.settings(t, true, "warn")
	if err := h.svc.AfterBuild(ctx, "web", "da_1", attest(t)); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeleteApp(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if h.sbomFiles(t) != 0 {
		t.Error("files must be removed")
	}
	if _, err := h.svc.Get(ctx, "web", "da_1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("record: %v", err)
	}
	if st, _ := h.svc.Settings(ctx, "web"); st.Enabled || st.Gate != GateOff {
		t.Errorf("settings must reset: %+v", st)
	}
}

func TestFileStore_RejectsUnsafeNames(t *testing.T) {
	fs := NewFileStore(t.TempDir())
	for _, c := range [][2]string{{"../x", "a"}, {"a", "../x"}, {"a/b", "c"}, {"", "c"}, {"a", ".hidden"}} {
		if _, err := fs.Path(c[0], c[1]); err == nil {
			t.Errorf("Path(%q, %q) must fail", c[0], c[1])
		}
	}
	if err := fs.RemoveApp("../etc"); err == nil {
		t.Error("RemoveApp must reject traversal")
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{
		EnvScanner: "grype", EnvScanTimeout: "90s", EnvScanMemoryMB: "junk", EnvScanEnabled: "false", build.EnvAttest: "true", EnvKeepPerApp: "7",
	}
	cfg := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }, nil)
	if cfg.Scanner != ScannerGrype || cfg.Image != DefaultGrypeImage || cfg.Timeout != 90*time.Second || cfg.MemoryMB != 1024 || cfg.Enabled || !cfg.BuildAttest || cfg.KeepPerApp != 7 {
		t.Errorf("cfg = %+v", cfg)
	}
	def := ConfigFromEnv(func(string) (string, bool) { return "", false }, nil)
	if def.BuildAttest || !def.Enabled || def.Scanner != ScannerTrivy {
		t.Errorf("defaults = %+v", def)
	}
	bad := ConfigFromEnv(func(k string) (string, bool) { return "clair", k == EnvScanner }, nil)
	if bad.Scanner != ScannerTrivy {
		t.Errorf("unknown scanner falls back to trivy, got %q", bad.Scanner)
	}
}
