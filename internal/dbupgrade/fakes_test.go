package dbupgrade

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStore struct {
	mu       sync.Mutex
	dbs      map[string]store.DesiredDatabase
	runs     map[string]store.DBUpgradeRun
	policies map[string]store.DBUpgradePolicy
}

func newFakeStore(dbs ...store.DesiredDatabase) *fakeStore {
	s := &fakeStore{dbs: map[string]store.DesiredDatabase{}, runs: map[string]store.DBUpgradeRun{}, policies: map[string]store.DBUpgradePolicy{}}
	for _, d := range dbs {
		s.dbs[d.Name] = d
	}
	return s
}

func (s *fakeStore) GetDesiredDatabase(_ context.Context, name string) (*store.DesiredDatabase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.dbs[name]
	if !ok {
		return nil, store.ErrDatabaseNotFound
	}
	return &d, nil
}

func (s *fakeStore) ListDesiredDatabases(context.Context) ([]store.DesiredDatabase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.DesiredDatabase
	for _, d := range s.dbs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *fakeStore) setVersion(name, v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.dbs[name]
	d.Version = v
	s.dbs[name] = d
}

func (s *fakeStore) version(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dbs[name].Version
}

func (s *fakeStore) CreateDBUpgradeRun(ctx context.Context, r store.DBUpgradeRun) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[r.ID] = cloneRun(r)
	return nil
}

func (s *fakeStore) UpdateDBUpgradeRun(ctx context.Context, r store.DBUpgradeRun) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[r.ID]; !ok {
		return store.ErrDBUpgradeRunNotFound
	}
	s.runs[r.ID] = cloneRun(r)
	return nil
}

func cloneRun(r store.DBUpgradeRun) store.DBUpgradeRun {
	tm := map[string]string{}
	for k, v := range r.Timings {
		tm[k] = v
	}
	r.Timings = tm
	r.Notify = append([]string(nil), r.Notify...)
	return r
}

func (s *fakeStore) GetDBUpgradeRun(_ context.Context, id string) (store.DBUpgradeRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return store.DBUpgradeRun{}, store.ErrDBUpgradeRunNotFound
	}
	return cloneRun(r), nil
}

func (s *fakeStore) ListDBUpgradeRuns(_ context.Context, name string, limit int) ([]store.DBUpgradeRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.DBUpgradeRun
	for _, r := range s.runs {
		if r.DatabaseName == name {
			out = append(out, cloneRun(r))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *fakeStore) ListActiveDBUpgradeRuns(context.Context) ([]store.DBUpgradeRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.DBUpgradeRun
	for _, r := range s.runs {
		if !store.DBUpgradeTerminal(r.State) {
			out = append(out, cloneRun(r))
		}
	}
	return out, nil
}

func (s *fakeStore) GetDBUpgradePolicy(_ context.Context, name string) (store.DBUpgradePolicy, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.policies[name]
	return p, ok, nil
}

func (s *fakeStore) SaveDBUpgradePolicy(_ context.Context, p store.DBUpgradePolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[p.DatabaseName] = p
	return nil
}

func (s *fakeStore) DeleteDBUpgradePolicy(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.policies, name)
	return nil
}

var errInjected = errors.New("injected failure")

// fakeRuntime models one database container. newFormat is true once the
// target version touched the data and the old version can no longer read it.
type fakeRuntime struct {
	mu    sync.Mutex
	store *fakeStore
	name  string
	calls []string

	busy             string
	fail             map[string]bool
	backupStatus     string
	verifyStatus     string
	unhealthy        map[string]bool
	writesNewFormat  bool
	newFormat        bool
	suspended        bool
	removedVolumes   []string
	crashAt          string
	cancel           context.CancelFunc
	backups, verifys map[string]string
}

func newFakeRuntime(s *fakeStore, name string) *fakeRuntime {
	return &fakeRuntime{
		store: s, name: name, fail: map[string]bool{}, unhealthy: map[string]bool{},
		backupStatus: store.BackupStatusSucceeded, verifyStatus: store.BackupVerificationStatusPassed,
		backups: map[string]string{}, verifys: map[string]string{},
	}
}

// hit records a call and reports the configured failure or simulated crash.
func (f *fakeRuntime) hit(ctx context.Context, name string) error {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	crash := f.crashAt == name
	if crash {
		f.crashAt = ""
	}
	fail := f.fail[name]
	f.mu.Unlock()
	if crash && f.cancel != nil {
		f.cancel()
		return ctx.Err()
	}
	if fail {
		return errInjected
	}
	return nil
}

func (f *fakeRuntime) called(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == name {
			n++
		}
	}
	return n
}

func (f *fakeRuntime) Busy(ctx context.Context, _ string) (string, error) {
	return f.busy, f.hit(ctx, "Busy")
}

func (f *fakeRuntime) RunBackup(ctx context.Context, id string, _ store.DesiredDatabase) error {
	f.mu.Lock()
	f.backups[id] = store.BackupStatusRunning
	f.mu.Unlock()
	if err := f.hit(ctx, "RunBackup"); err != nil {
		if ctx.Err() == nil {
			f.mu.Lock()
			f.backups[id] = store.BackupStatusFailed
			f.mu.Unlock()
		}
		return err
	}
	f.mu.Lock()
	f.backups[id] = f.backupStatus
	f.mu.Unlock()
	return nil
}

func (f *fakeRuntime) BackupStatus(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.backups[id], nil
}

func (f *fakeRuntime) RunVerify(ctx context.Context, vid, _, _ string) error {
	if err := f.hit(ctx, "RunVerify"); err != nil {
		return err
	}
	f.mu.Lock()
	f.verifys[vid] = f.verifyStatus
	f.mu.Unlock()
	return nil
}

func (f *fakeRuntime) VerifyStatus(_ context.Context, _, vid string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.verifys[vid], nil
}

func (f *fakeRuntime) CheckImage(ctx context.Context, _, _ string) error {
	return f.hit(ctx, "CheckImage")
}

func (f *fakeRuntime) ImageDigest(ctx context.Context, _ string) (string, error) {
	return "sha256:old", f.hit(ctx, "ImageDigest")
}

func (f *fakeRuntime) SnapshotData(ctx context.Context, _, _ string) error {
	f.mu.Lock()
	f.suspended = true
	f.mu.Unlock()
	return f.hit(ctx, "SnapshotData")
}

func (f *fakeRuntime) SetVersion(ctx context.Context, name, v string) error {
	key := "SetVersion:" + v
	if err := f.hit(ctx, key); err != nil && ctx.Err() == nil {
		return err
	}
	f.store.setVersion(name, v)
	f.mu.Lock()
	f.suspended = false
	if f.writesNewFormat && v != "" && f.isTarget(v) {
		f.newFormat = true
	}
	f.mu.Unlock()
	return ctx.Err()
}

// isTarget is true for the version the run upgrades to: anything not marked
// as the original stays readable on old data.
func (f *fakeRuntime) isTarget(v string) bool { return v == fakeTarget }

func (f *fakeRuntime) Resume(ctx context.Context, _ string) error {
	f.mu.Lock()
	f.suspended = false
	f.mu.Unlock()
	return f.hit(ctx, "Resume")
}

func (f *fakeRuntime) Health(ctx context.Context, db store.DesiredDatabase, v string, _ bool) (bool, string, error) {
	if err := f.hit(ctx, "Health"); err != nil {
		return false, "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.suspended || db.Version != v {
		return false, "", nil
	}
	if f.unhealthy[v] || (f.newFormat && !f.isTarget(v)) {
		return false, "", errors.New("connection refused")
	}
	return true, v, nil
}

func (f *fakeRuntime) RestoreSnapshot(ctx context.Context, name, _, v string) error {
	if err := f.hit(ctx, "RestoreSnapshot"); err != nil {
		return err
	}
	f.mu.Lock()
	f.newFormat = false
	f.mu.Unlock()
	return f.SetVersion(ctx, name, v)
}

func (f *fakeRuntime) ResetData(ctx context.Context, name, v string) error {
	if err := f.hit(ctx, "ResetData"); err != nil {
		return err
	}
	f.mu.Lock()
	f.newFormat = false
	f.mu.Unlock()
	return f.SetVersion(ctx, name, v)
}

func (f *fakeRuntime) RestoreBackup(ctx context.Context, _, _ string, _ store.DesiredDatabase) error {
	return f.hit(ctx, "RestoreBackup")
}

func (f *fakeRuntime) RemoveVolume(ctx context.Context, v string) error {
	f.mu.Lock()
	f.removedVolumes = append(f.removedVolumes, v)
	f.mu.Unlock()
	return f.hit(ctx, "RemoveVolume")
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []string
}

func (n *fakeNotifier) Notify(_ context.Context, _ []string, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, text)
	return nil
}
