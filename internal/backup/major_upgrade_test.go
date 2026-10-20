package backup

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type upgradeStoreFake struct {
	mu        sync.Mutex
	db        store.DesiredDatabase
	suspended bool
	upgrades  map[string]*store.MajorUpgrade
}

func newUpgradeStoreFake(version string) *upgradeStoreFake {
	return &upgradeStoreFake{db: store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: version}, upgrades: map[string]*store.MajorUpgrade{}}
}

func (s *upgradeStoreFake) GetDesiredDatabase(context.Context, string) (*store.DesiredDatabase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.db
	return &d, nil
}

func (s *upgradeStoreFake) SaveDesiredDatabase(_ context.Context, d store.DesiredDatabase) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = d
	return nil
}

func (s *upgradeStoreFake) UpdateDatabaseSuspended(_ context.Context, _ string, v bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suspended = v
	return nil
}

func (s *upgradeStoreFake) StartMajorUpgrade(_ context.Context, u store.MajorUpgrade) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u.Status = store.BackupStatusRunning
	s.upgrades[u.ID] = &u
	return nil
}

func (s *upgradeStoreFake) UpdateMajorUpgradePhase(_ context.Context, id, phase, snap string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.upgrades[id]
	u.Phase = phase
	if snap != "" {
		u.SnapshotVolume = snap
	}
	return nil
}

func (s *upgradeStoreFake) FinishMajorUpgrade(_ context.Context, id, status, msg, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.upgrades[id]
	u.Status, u.Error = status, msg
	return nil
}

func (s *upgradeStoreFake) GetMajorUpgrade(_ context.Context, id string) (store.MajorUpgrade, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.upgrades[id]
	if !ok {
		return store.MajorUpgrade{}, store.ErrMajorUpgradeNotFound
	}
	return *u, nil
}

func (s *upgradeStoreFake) ListRunningMajorUpgrades(context.Context) ([]store.MajorUpgrade, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.MajorUpgrade
	for _, u := range s.upgrades {
		if u.Status == store.BackupStatusRunning {
			out = append(out, *u)
		}
	}
	return out, nil
}

func (s *upgradeStoreFake) ClearMajorUpgradeSnapshot(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upgrades[id].SnapshotVolume = ""
	return nil
}

// upgradeRuntimeFake plays the reconciler too: db-main is gone while the
// database is suspended and runs the desired version otherwise.
type upgradeRuntimeFake struct {
	docker.Runtime
	st           *upgradeStoreFake
	mu           sync.Mutex
	ops          []string
	failExec     map[string]error
	oldCounts    string
	newCounts    string
	newVersion   string
	removedVols  []string
	missingImage string
}

func (r *upgradeRuntimeFake) record(op string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops = append(r.ops, op)
}

func (r *upgradeRuntimeFake) opsJoined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.ops, "\n")
}

func (r *upgradeRuntimeFake) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	if name != "db-main" {
		return nil, nil
	}
	r.st.mu.Lock()
	defer r.st.mu.Unlock()
	if r.st.suspended {
		return nil, nil
	}
	return &docker.ContainerState{Running: true, Image: "postgres:" + r.st.db.Version}, nil
}

func (r *upgradeRuntimeFake) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	r.record("create " + spec.Name + " " + spec.Image)
	if r.missingImage != "" && spec.Image == r.missingImage {
		return "", errors.New("pull access denied: manifest unknown")
	}
	return spec.Name, nil
}
func (r *upgradeRuntimeFake) Start(context.Context, string) error { return nil }
func (r *upgradeRuntimeFake) Stop(_ context.Context, id string, _ time.Duration) error {
	r.record("stop " + id)
	return nil
}
func (r *upgradeRuntimeFake) Remove(_ context.Context, id string, _ bool) error {
	r.record("remove " + id)
	return nil
}
func (r *upgradeRuntimeFake) EnsureVolume(_ context.Context, name string) error {
	r.record("ensure-volume " + name)
	return nil
}
func (r *upgradeRuntimeFake) RemoveVolume(_ context.Context, name string) error {
	r.mu.Lock()
	r.removedVols = append(r.removedVols, name)
	r.mu.Unlock()
	return nil
}

func (r *upgradeRuntimeFake) Exec(_ context.Context, id string, cmd []string) (io.ReadCloser, error) {
	joined := strings.Join(cmd, " ")
	r.record("exec " + id + " " + joined)
	for sub, err := range r.failExec {
		if strings.Contains(joined, sub) {
			return nil, err
		}
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (r *upgradeRuntimeFake) ExecWithInput(_ context.Context, id string, _ []string, in io.Reader) (io.ReadCloser, error) {
	b, _ := io.ReadAll(in)
	sql := string(b)
	out := ""
	switch {
	case strings.Contains(sql, "server_version_num"):
		out = r.newVersion
	case strings.Contains(sql, "string_agg"):
		out = r.oldCounts
		if id == "db-main" {
			out = r.newCounts
		}
	}
	return io.NopCloser(strings.NewReader(out + "\n")), nil
}

type upgradeDumper struct{ body string }

func (d upgradeDumper) Dump(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(d.body)), nil
}

type upgradeRestorer struct {
	err   error
	calls int
	got   string
}

func (r *upgradeRestorer) Restore(_ context.Context, _, _ string, dump io.Reader) error {
	r.calls++
	b, _ := io.ReadAll(dump)
	r.got = string(b)
	return r.err
}

const goodDump = "CREATE TABLE t (i int);\n-- PostgreSQL database dump complete\n"

func newUpgradeRunner(t *testing.T, st *upgradeStoreFake, rt *upgradeRuntimeFake, d Dumper, rs Restorer) *MajorUpgradeRunner {
	t.Helper()
	rt.st = st
	return &MajorUpgradeRunner{
		Store: st, Runtime: rt, Dumper: d, Restorer: rs,
		WorkDir: t.TempDir(), Wait: 2 * time.Second, Poll: time.Millisecond,
		Now: func() time.Time { return time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) },
	}
}

func TestMajorUpgradeRunner_Success(t *testing.T) {
	st := newUpgradeStoreFake("16")
	rt := &upgradeRuntimeFake{oldCounts: "t=3", newCounts: "t=3", newVersion: "170002"}
	rs := &upgradeRestorer{}
	r := newUpgradeRunner(t, st, rt, upgradeDumper{goodDump}, rs)

	if err := r.RunMajorUpgrade(context.Background(), "mu_abc1", "main", "17"); err != nil {
		t.Fatalf("RunMajorUpgrade() error = %v", err)
	}
	u := st.upgrades["mu_abc1"]
	if u.Status != store.BackupStatusSucceeded || u.Phase != phaseVerify {
		t.Errorf("upgrade = %+v, want succeeded after verify", u)
	}
	if st.db.Version != "17" || st.suspended {
		t.Errorf("database version %q suspended %v, want 17 and running", st.db.Version, st.suspended)
	}
	if u.SnapshotVolume != "db-main-data-pre16-muabc1" {
		t.Errorf("snapshot volume = %q", u.SnapshotVolume)
	}
	if rs.got != goodDump {
		t.Errorf("restored dump = %q", rs.got)
	}
	ops := rt.opsJoined()
	for _, want := range []string{"create db-main-upgrade-src-muabc1 postgres:16", "cp -a /src/. /dst/", "rm -rf --"} {
		if !strings.Contains(ops, want) {
			t.Errorf("missing op %q in:\n%s", want, ops)
		}
	}
	if strings.Index(ops, "cp -a /src/. /dst/") > strings.Index(ops, "rm -rf --") {
		t.Error("the snapshot copy must happen before the live volume is wiped")
	}
}

func TestMajorUpgradeRunner_FailuresRollBack(t *testing.T) {
	tests := []struct {
		name       string
		newCounts  string
		newVersion string
		restoreErr error
	}{
		{"restore fails", "t=3", "170002", errors.New("psql: syntax error")},
		{"row counts differ", "t=2", "170002", nil},
		{"wrong server version", "t=3", "160004", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newUpgradeStoreFake("16")
			rt := &upgradeRuntimeFake{oldCounts: "t=3", newCounts: tt.newCounts, newVersion: tt.newVersion}
			r := newUpgradeRunner(t, st, rt, upgradeDumper{goodDump}, &upgradeRestorer{err: tt.restoreErr})

			err := r.RunMajorUpgrade(context.Background(), "mu_x1", "main", "17")
			if err == nil || !strings.Contains(err.Error(), "rolled back to version 16") {
				t.Fatalf("RunMajorUpgrade() error = %v, want a rolled back failure", err)
			}
			u := st.upgrades["mu_x1"]
			if u.Status != store.MajorUpgradeStatusRolledBack {
				t.Errorf("status = %q, want rolled_back", u.Status)
			}
			if st.db.Version != "16" || st.suspended {
				t.Errorf("version %q suspended %v, want 16 and running", st.db.Version, st.suspended)
			}
			if got := strings.Count(rt.opsJoined(), "cp -a /src/. /dst/"); got != 2 {
				t.Errorf("volume copies = %d, want 2 (snapshot, then snapshot back)", got)
			}
			if len(rt.removedVols) != 0 {
				t.Errorf("snapshot must be kept after a rollback, removed %v", rt.removedVols)
			}
		})
	}
}

func TestMajorUpgradeRunner_FailureBeforeWipeLeavesDataUntouched(t *testing.T) {
	st := newUpgradeStoreFake("16")
	rt := &upgradeRuntimeFake{oldCounts: "t=3", newCounts: "t=3", newVersion: "170002"}
	rs := &upgradeRestorer{}
	r := newUpgradeRunner(t, st, rt, upgradeDumper{"CREATE TABLE t (i int);\n"}, rs)

	err := r.RunMajorUpgrade(context.Background(), "mu_x2", "main", "17")
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error = %v, want a truncated dump failure", err)
	}
	u := st.upgrades["mu_x2"]
	if u.Status != store.BackupStatusFailed {
		t.Errorf("status = %q, want failed", u.Status)
	}
	if st.db.Version != "16" || st.suspended {
		t.Errorf("version %q suspended %v, want 16 and running", st.db.Version, st.suspended)
	}
	if strings.Contains(rt.opsJoined(), "rm -rf --") || rs.calls != 0 {
		t.Error("the live volume must not be wiped nor a restore attempted")
	}
	if len(rt.removedVols) != 1 || rt.removedVols[0] != "db-main-data-pre16-mux2" || u.SnapshotVolume != "" {
		t.Errorf("snapshot cleanup: removed %v, recorded %q", rt.removedVols, u.SnapshotVolume)
	}
}

func TestMajorUpgradeRunner_RollbackFailureNamesTheSnapshot(t *testing.T) {
	st := newUpgradeStoreFake("16")
	rt := &upgradeRuntimeFake{oldCounts: "t=3", newCounts: "t=3", newVersion: "170002"}
	r := newUpgradeRunner(t, st, rt, upgradeDumper{goodDump}, &upgradeRestorer{err: errors.New("boom")})
	// The first copy (snapshot) succeeds; the second (copy back) fails.
	calls := 0
	rt.failExec = nil
	failing := &failSecondCopyRuntime{upgradeRuntimeFake: rt, calls: &calls}
	r.Runtime = failing

	err := r.RunMajorUpgrade(context.Background(), "mu_x3", "main", "17")
	if err == nil || !strings.Contains(err.Error(), "ROLLBACK FAILED") || !strings.Contains(err.Error(), "db-main-data-pre16-mux3") {
		t.Fatalf("error = %v, want ROLLBACK FAILED naming the snapshot volume", err)
	}
	if st.upgrades["mu_x3"].Status != store.BackupStatusFailed {
		t.Errorf("status = %q, want failed", st.upgrades["mu_x3"].Status)
	}
}

type failSecondCopyRuntime struct {
	*upgradeRuntimeFake
	calls *int
}

func (f *failSecondCopyRuntime) Exec(ctx context.Context, id string, cmd []string) (io.ReadCloser, error) {
	if strings.Contains(strings.Join(cmd, " "), "cp -a") {
		*f.calls++
		if *f.calls == 2 {
			return nil, errors.New("disk full")
		}
	}
	return f.upgradeRuntimeFake.Exec(ctx, id, cmd)
}

func TestMajorUpgradeRunner_ManualRollbackAndDiscard(t *testing.T) {
	st := newUpgradeStoreFake("17")
	st.upgrades["mu_ok"] = &store.MajorUpgrade{ID: "mu_ok", DatabaseName: "main", FromVersion: "16", ToVersion: "17", Status: store.BackupStatusSucceeded, SnapshotVolume: "db-main-data-pre16-muok"}
	st.upgrades["mu_run"] = &store.MajorUpgrade{ID: "mu_run", Status: store.BackupStatusRunning, SnapshotVolume: "v"}
	st.upgrades["mu_none"] = &store.MajorUpgrade{ID: "mu_none", Status: store.BackupStatusSucceeded}
	rt := &upgradeRuntimeFake{}
	r := newUpgradeRunner(t, st, rt, upgradeDumper{}, &upgradeRestorer{})

	if err := r.Rollback(context.Background(), "mu_run"); err == nil {
		t.Error("rolling back a running upgrade must be refused")
	}
	if err := r.Rollback(context.Background(), "mu_none"); err == nil {
		t.Error("rolling back an upgrade without a snapshot must be refused")
	}
	if err := r.Rollback(context.Background(), "mu_ok"); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if st.db.Version != "16" || st.upgrades["mu_ok"].Status != store.MajorUpgradeStatusRolledBack {
		t.Errorf("version %q status %q, want 16 and rolled_back", st.db.Version, st.upgrades["mu_ok"].Status)
	}

	if err := r.DiscardSnapshot(context.Background(), "mu_run"); err == nil {
		t.Error("discarding a running upgrade's snapshot must be refused")
	}
	if err := r.DiscardSnapshot(context.Background(), "mu_ok"); err != nil {
		t.Fatalf("DiscardSnapshot() error = %v", err)
	}
	if len(rt.removedVols) != 1 || st.upgrades["mu_ok"].SnapshotVolume != "" {
		t.Errorf("removed %v, snapshot %q", rt.removedVols, st.upgrades["mu_ok"].SnapshotVolume)
	}
}

func TestMajorUpgradeRunner_RecoverInterrupted(t *testing.T) {
	t.Run("after the wipe rolls back", func(t *testing.T) {
		st := newUpgradeStoreFake("17")
		u := &store.MajorUpgrade{ID: "mu_r1", DatabaseName: "main", FromVersion: "16", ToVersion: "17", Status: store.BackupStatusRunning, Phase: phaseRestore, SnapshotVolume: "snapvol"}
		st.upgrades[u.ID] = u
		rt := &upgradeRuntimeFake{}
		r := newUpgradeRunner(t, st, rt, upgradeDumper{}, &upgradeRestorer{})
		r.RecoverInterrupted(context.Background())
		if st.db.Version != "16" || st.upgrades["mu_r1"].Status != store.MajorUpgradeStatusRolledBack {
			t.Errorf("version %q status %q", st.db.Version, st.upgrades["mu_r1"].Status)
		}
	})
	t.Run("before the wipe unsuspends", func(t *testing.T) {
		st := newUpgradeStoreFake("16")
		st.suspended = true
		u := &store.MajorUpgrade{ID: "mu_r2", DatabaseName: "main", FromVersion: "16", ToVersion: "17", Status: store.BackupStatusRunning, Phase: phaseDump, SnapshotVolume: "snapvol"}
		st.upgrades[u.ID] = u
		rt := &upgradeRuntimeFake{}
		r := newUpgradeRunner(t, st, rt, upgradeDumper{}, &upgradeRestorer{})
		r.RecoverInterrupted(context.Background())
		if st.suspended || st.upgrades["mu_r2"].Status != store.BackupStatusFailed || len(rt.removedVols) != 1 {
			t.Errorf("suspended %v status %q removed %v", st.suspended, st.upgrades["mu_r2"].Status, rt.removedVols)
		}
	})
}

func TestDumpIsComplete(t *testing.T) {
	tests := []struct {
		body    string
		wantErr bool
	}{
		{"x\n-- PostgreSQL database dump complete\n\n", false},
		{"", true},
		{"CREATE TABLE t(i int);\nCOPY t FROM stdin;\n", true},
	}
	for _, tt := range tests {
		f, err := os.CreateTemp(t.TempDir(), "d")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString(tt.body)
		if err := dumpIsComplete(f); (err != nil) != tt.wantErr {
			t.Errorf("dumpIsComplete(%q) error = %v, wantErr %v", tt.body, err, tt.wantErr)
		}
		_ = f.Close()
	}
}

func TestMajorUpgradeRunner_MissingTargetImageFailsBeforeDowntime(t *testing.T) {
	st := newUpgradeStoreFake("16")
	rt := &upgradeRuntimeFake{missingImage: "postgres:99"}
	r := newUpgradeRunner(t, st, rt, upgradeDumper{goodDump}, &upgradeRestorer{})

	err := r.RunMajorUpgrade(context.Background(), "mu_img", "main", "99")
	if err == nil || !strings.Contains(err.Error(), "postgres:99 is not available") {
		t.Fatalf("error = %v, want the missing image failure", err)
	}
	if st.upgrades["mu_img"].Status != store.BackupStatusFailed || st.suspended {
		t.Errorf("status %q suspended %v, want failed and never suspended", st.upgrades["mu_img"].Status, st.suspended)
	}
	if strings.Contains(rt.opsJoined(), "ensure-volume") {
		t.Error("no snapshot may be taken before the image check passes")
	}
}
