package orphans

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const testInstance = "inst-1"

type fakeRuntime struct {
	docker.Runtime
	containers []docker.ContainerState
	removeErr  error
	removed    []string
}

func (f *fakeRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return f.containers, nil
}

func (f *fakeRuntime) Remove(_ context.Context, id string, _ bool) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, id)
	return nil
}

type fakeVolumes struct {
	vols    []docker.NamedVolume
	removed []string
}

func (f *fakeVolumes) ListNamedVolumes(context.Context) ([]docker.NamedVolume, error) {
	return f.vols, nil
}
func (f *fakeVolumes) RemoveVolume(_ context.Context, n string) error {
	f.removed = append(f.removed, n)
	return nil
}

func ours(name string) docker.ContainerState {
	return docker.ContainerState{ID: "id-" + name, Name: name, Labels: map[string]string{spec.InstanceLabelKey: testInstance}}
}

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedService(t *testing.T, db *store.DB, name string) store.DesiredService {
	t.Helper()
	svc := store.DesiredService{Name: name, Image: "img:v1", Port: 80}
	if err := db.SaveDesiredService(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestClassifyContainer(t *testing.T) {
	live := store.DesiredService{Name: "web", Image: "img:v1"}
	desired := Desired{
		Services:        map[string]bool{"web": true},
		ContainerNames:  map[string]bool{application.ContainerName("web", application.NameImage(live), ""): true},
		Databases:       map[string]bool{"pg": true},
		PendingTeardown: map[string]bool{"dying": true},
	}
	desired.ContainerNames["db-pg"] = true
	liveName := application.ContainerName("web", application.NameImage(live), "")
	oldGen := application.ContainerName("web", "img:v0", "")
	gone := application.ContainerName("gone", "img:v1", "")
	dying := application.ContainerName("dying", "img:v1", "")

	foreign := ours(gone)
	foreign.Labels[spec.InstanceLabelKey] = "other"
	unlabeled := docker.ContainerState{ID: "x", Name: gone}

	tests := []struct {
		name       string
		c          docker.ContainerState
		wantOrphan bool
		wantReason Reason
		wantSkip   bool
	}{
		{"live app", ours(liveName), false, "", false},
		{"live egress sidecar", ours(liveName + "-egress"), false, "", false},
		{"live database", ours("db-pg"), false, "", false},
		{"deleted app", ours(gone), true, ReasonServiceDeleted, false},
		{"deleted app egress sidecar", ours(gone + "-egress"), true, ReasonServiceDeleted, false},
		{"old generation of live app", ours(oldGen), true, ReasonStaleGeneration, true},
		{"teardown pending", ours(dying), true, ReasonServiceDeleted, true},
		{"deleted database", ours("db-old"), true, ReasonDatabaseDeleted, false},
		{"foreign instance", foreign, true, ReasonForeignInstance, true},
		{"unlabeled", unlabeled, true, ReasonUnmanaged, true},
		{"unknown shape", ours("backup-helper"), true, ReasonUnrecognized, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, isOrphan := ClassifyContainer(tc.c, desired, testInstance, "")
			if isOrphan != tc.wantOrphan {
				t.Fatalf("orphan = %v, want %v", isOrphan, tc.wantOrphan)
			}
			if !isOrphan {
				return
			}
			if o.Reason != tc.wantReason || (o.Skip != "") != tc.wantSkip {
				t.Fatalf("got reason=%q skip=%q, want reason=%q skip=%v", o.Reason, o.Skip, tc.wantReason, tc.wantSkip)
			}
		})
	}
}

func TestClassifyVolume(t *testing.T) {
	d := Desired{VolumeNames: map[string]bool{"app-keep": true}}
	tests := []struct {
		name       string
		v          docker.NamedVolume
		reap       bool
		wantOrphan bool
		wantSkip   bool
	}{
		{"referenced", docker.NamedVolume{Name: "app-keep"}, true, false, false},
		{"orphan opted in", docker.NamedVolume{Name: "app-old"}, true, true, false},
		{"orphan opt-in off", docker.NamedVolume{Name: "app-old"}, false, true, true},
		{"mounted", docker.NamedVolume{Name: "app-old", Mounted: true}, true, true, true},
		{"database data volume", docker.NamedVolume{Name: "db-gone-data"}, true, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, isOrphan := ClassifyVolume(tc.v, d, "", tc.reap)
			if isOrphan != tc.wantOrphan || (o.Skip != "") != tc.wantSkip {
				t.Fatalf("orphan=%v skip=%q, want orphan=%v skip=%v", isOrphan, o.Skip, tc.wantOrphan, tc.wantSkip)
			}
		})
	}
}

func newReaper(t *testing.T, db *store.DB, rt *fakeRuntime, vols VolumeBackend, now *time.Time, cfg Config) *Reaper {
	t.Helper()
	return New(Deps{
		Store:      db,
		NodeIDs:    func(context.Context) ([]string, error) { return []string{""}, nil },
		Resolve:    func(string) (docker.Runtime, error) { return rt, nil },
		Volumes:    vols,
		InstanceID: testInstance,
		Config:     cfg,
		Logger:     slog.New(slog.DiscardHandler),
		Now:        func() time.Time { return *now },
	})
}

func TestReapGraceDryRunAndIdempotence(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	kept := seedService(t, db, "web")
	rt := &fakeRuntime{containers: []docker.ContainerState{
		ours(application.ContainerName("web", application.NameImage(kept), "")),
		ours(application.ContainerName("gone", "img:v1", "")),
	}}
	now := time.Now()
	cfg := DefaultConfig()
	r := newReaper(t, db, rt, nil, &now, cfg)

	rep, err := r.Reap(ctx, false)
	if err != nil || len(rep.Removed) != 0 || len(rep.Findings) != 1 {
		t.Fatalf("first pass: err=%v removed=%d findings=%d, want no removal and 1 finding", err, len(rep.Removed), len(rep.Findings))
	}

	now = now.Add(cfg.Grace - time.Minute)
	if rep, _ = r.Reap(ctx, false); len(rep.Removed) != 0 {
		t.Fatal("removed inside the grace period")
	}

	now = now.Add(2 * time.Minute)
	if rep, _ = r.Reap(ctx, true); len(rep.Removed) != 0 || !rep.Findings[0].Due {
		t.Fatalf("dry run removed=%d due=%v, want 0 and true", len(rep.Removed), rep.Findings[0].Due)
	}
	if len(rt.removed) != 0 {
		t.Fatal("dry run removed a container")
	}

	if rep, _ = r.Reap(ctx, false); len(rep.Removed) != 1 || len(rt.removed) != 1 {
		t.Fatalf("real pass removed=%d, want 1", len(rep.Removed))
	}
	entries, _ := db.ListAuditEntries(ctx, 10, nil, store.AuditEntryFilter{Method: "REAP"})
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	rt.containers = rt.containers[:1]
	if rep, _ = r.Reap(ctx, false); len(rep.Findings) != 0 || len(rep.Removed) != 0 {
		t.Fatal("second run after convergence is not a no-op")
	}
	if s, _ := db.ListOrphanSightings(ctx); len(s) != 0 {
		t.Fatalf("sightings left behind: %v", s)
	}
}

func TestReapRemoveFailureRetriesAndKeepsSighting(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	seedService(t, db, "web")
	rt := &fakeRuntime{containers: []docker.ContainerState{ours(application.ContainerName("gone", "i", ""))}, removeErr: errors.New("busy")}
	now := time.Now()
	cfg := DefaultConfig()
	cfg.Grace = 0
	r := newReaper(t, db, rt, nil, &now, cfg)

	_, _ = r.Reap(ctx, false)
	now = now.Add(time.Second)
	rep, _ := r.Reap(ctx, false)
	if len(rep.Failed) != 1 || len(rep.Removed) != 0 {
		t.Fatalf("failed=%d removed=%d, want 1 and 0", len(rep.Failed), len(rep.Removed))
	}
	res, err := NewController(r).Reconcile(ctx)
	if err == nil || res.Conditions[0].Reason != "ReapFailed" {
		t.Fatalf("controller err=%v reason=%v, want ReapFailed surfaced", err, res.Conditions)
	}
	rt.removeErr = nil
	rep, _ = r.Reap(ctx, false)
	if len(rep.Removed) != 1 {
		t.Fatal("did not retry after the failure cleared")
	}
}

func TestReapRefusesWhenNothingDesiredAndCapsPerPass(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	rt := &fakeRuntime{containers: []docker.ContainerState{ours(application.ContainerName("a", "i", ""))}}
	now := time.Now()
	cfg := DefaultConfig()
	cfg.Grace = 0
	r := newReaper(t, db, rt, nil, &now, cfg)
	_, _ = r.Reap(ctx, false)
	now = now.Add(time.Second)
	if rep, _ := r.Reap(ctx, false); len(rep.Removed) != 0 || rep.Halted == "" {
		t.Fatalf("removed with empty desired state: %+v", rep)
	}

	seedService(t, db, "keep")
	cfg.MaxPerPass = 1
	rt.containers = []docker.ContainerState{ours(application.ContainerName("a", "i", "")), ours(application.ContainerName("b", "i", ""))}
	r = newReaper(t, db, rt, nil, &now, cfg)
	_, _ = r.Reap(ctx, true)
	now = now.Add(time.Second)
	if rep, _ := r.Reap(ctx, false); len(rep.Removed) != 1 || rep.Halted == "" {
		t.Fatalf("removed=%d halted=%q, want exactly 1 and a limit note", len(rep.Removed), rep.Halted)
	}
}

func TestReapVolumesOptInAndNeverDatabaseVolumes(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	seedService(t, db, "web")
	rt := &fakeRuntime{}
	vols := &fakeVolumes{vols: []docker.NamedVolume{{Name: "app-old"}, {Name: "db-gone-data"}, {Name: "app-used", Mounted: true}}}
	now := time.Now()
	cfg := DefaultConfig()
	cfg.Grace, cfg.VolumeGrace = 0, 0

	r := newReaper(t, db, rt, vols, &now, cfg)
	_, _ = r.Reap(ctx, false)
	now = now.Add(time.Second)
	_, _ = r.Reap(ctx, false)
	if len(vols.removed) != 0 {
		t.Fatalf("removed %v with volume reaping off", vols.removed)
	}

	cfg.ReapVolumes = true
	r = newReaper(t, db, rt, vols, &now, cfg)
	_, _ = r.Reap(ctx, false)
	now = now.Add(time.Second)
	_, _ = r.Reap(ctx, false)
	if len(vols.removed) != 1 || vols.removed[0] != "app-old" {
		t.Fatalf("removed %v, want only app-old", vols.removed)
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{EnvMode: "dry-run", EnvGrace: "2m", EnvReapVolumes: "true", EnvMaxRemovals: "bad"}
	cfg := ConfigFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if cfg.Mode != ModeDryRun || cfg.Grace != 2*time.Minute || !cfg.ReapVolumes || cfg.MaxPerPass != DefaultConfig().MaxPerPass {
		t.Fatalf("cfg = %+v", cfg)
	}
}

type multiNodeRuntimes map[string]*fakeRuntime

func TestReapMisplacedContainerOnlyWhenTargetNodeRuns(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	svc := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, NodeID: "n2"}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "n2"); err != nil {
		t.Fatal(err)
	}
	name := application.ContainerName("web", application.NameImage(svc), "")
	old := ours(name)
	old.Running = true
	fresh := ours(name)
	fresh.Running = true
	rts := multiNodeRuntimes{"": {containers: []docker.ContainerState{old}}, "n2": {}}

	now := time.Now()
	cfg := DefaultConfig()
	cfg.Grace = 0
	r := New(Deps{
		Store:      db,
		NodeIDs:    func(context.Context) ([]string, error) { return []string{"", "n2"}, nil },
		Resolve:    func(id string) (docker.Runtime, error) { return rts[id], nil },
		InstanceID: testInstance,
		Config:     cfg,
		Logger:     slog.New(slog.DiscardHandler),
		Now:        func() time.Time { return now },
	})

	_, _ = r.Reap(ctx, false)
	now = now.Add(time.Second)
	if rep, _ := r.Reap(ctx, false); len(rep.Removed) != 0 || len(rts[""].removed) != 0 {
		t.Fatal("removed the only running copy while the target node had none")
	}

	rts["n2"].containers = []docker.ContainerState{fresh}
	_, _ = r.Reap(ctx, false)
	now = now.Add(time.Second)
	rep, _ := r.Reap(ctx, false)
	if len(rep.Removed) != 1 || rep.Removed[0].Reason != ReasonMisplaced || len(rts["n2"].removed) != 0 {
		t.Fatalf("removed=%+v, want the stale copy on the old node only", rep.Removed)
	}
}

type fakeCerts struct{ keys map[string]bool }

func (f *fakeCerts) ListCertStorageKeys(_ context.Context, prefix string, _ bool) ([]string, error) {
	var out []string
	for k := range f.keys {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

func (f *fakeCerts) DeleteCertStorageValue(_ context.Context, k string) error {
	delete(f.keys, k)
	return nil
}

func TestServedSet(t *testing.T) {
	set := ServedSet([]string{"A.Example.com", "*.wild.dev"})
	for _, want := range []string{"a.example.com", "wildcard_.example.com", "wildcard_.wild.dev"} {
		if !set[want] {
			t.Errorf("ServedSet missing %q: %v", want, set)
		}
	}
}

func TestReapStaleCertificates(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	seedService(t, db, "web")
	certs := &fakeCerts{keys: map[string]bool{}}
	for _, d := range []string{"live.example.com", "old.example.com", "wildcard_.apps.dev"} {
		for _, ext := range []string{".crt", ".key", ".json"} {
			certs.keys["certificates/local/"+d+"/"+d+ext] = true
		}
	}
	served := []string{"live.example.com", "*.apps.dev"}
	known := false
	now := time.Now()
	cfg := DefaultConfig()
	cfg.Grace, cfg.CertGrace = 0, time.Minute
	r := New(Deps{
		Store: db, NodeIDs: func(context.Context) ([]string, error) { return nil, nil },
		Resolve:     func(string) (docker.Runtime, error) { return &fakeRuntime{}, nil },
		Certs:       certs,
		ServedHosts: func(context.Context) ([]string, bool, error) { return served, known, nil },
		InstanceID:  testInstance, Config: cfg, Logger: slog.New(slog.DiscardHandler),
		Now: func() time.Time { return now },
	})

	now = now.Add(time.Hour)
	_, _ = r.Reap(ctx, false)
	now = now.Add(time.Hour)
	if rep, _ := r.Reap(ctx, false); len(rep.Findings) != 0 || len(certs.keys) != 9 {
		t.Fatalf("scanned certificates before ingress ever applied a config: %+v", rep)
	}

	known = true
	_, _ = r.Reap(ctx, false)
	now = now.Add(30 * time.Second)
	if rep, _ := r.Reap(ctx, false); len(rep.Removed) != 0 || len(certs.keys) != 9 {
		t.Fatal("removed a certificate inside its grace period")
	}
	now = now.Add(time.Minute)
	rep, _ := r.Reap(ctx, false)
	if len(rep.Removed) != 1 || rep.Removed[0].Name != "local/old.example.com" {
		t.Fatalf("removed = %+v, want only the unserved certificate", rep.Removed)
	}
	if len(certs.keys) != 6 {
		t.Fatalf("keys left = %d, want the live and wildcard certificates untouched", len(certs.keys))
	}
}
