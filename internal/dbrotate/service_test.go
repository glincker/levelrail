package dbrotate

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const dbName = "main"

var alterRe = regexp.MustCompile(`IDENTIFIED BY '([^']*)'`)

// fakeEngine emulates a MySQL container's exec surface: the admin session
// must present the password in force, ALTER USER changes it, and a login check
// passes only for the current password.
type fakeEngine struct {
	mu        sync.Mutex
	password  string
	refuse    string
	failSetAt int
	setCalls  int
	seen      []string
}

func (f *fakeEngine) ExecWithInput(_ context.Context, _ string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, _ := io.ReadAll(stdin)
	first, rest, _ := strings.Cut(string(raw), "\n")
	f.seen = append(f.seen, strings.Join(cmd, " ")+"\n"+string(raw))
	exitErr := &docker.ExecExitError{ExitCode: 1, Stderr: "boom " + first}
	if strings.Contains(rest, "ALTER USER") {
		f.setCalls++
		if first != f.password {
			return nil, exitErr
		}
		m := alterRe.FindStringSubmatch(rest)
		if f.failSetAt == f.setCalls {
			f.password = m[1]
			return nil, exitErr
		}
		f.password = m[1]
		return io.NopCloser(strings.NewReader("")), nil
	}
	if first != f.password || first == f.refuse {
		return nil, exitErr
	}
	return io.NopCloser(strings.NewReader("1\n")), nil
}

type fakeStore struct {
	mu   sync.Mutex
	recs map[string]store.DatabasePasswordRotation
}

func (s *fakeStore) GetDatabasePasswordRotation(_ context.Context, n string) (store.DatabasePasswordRotation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.recs[n]; ok {
		return r, nil
	}
	return store.DatabasePasswordRotation{Database: n, State: store.DBPasswordRotationIdle}, nil
}

func (s *fakeStore) ListDatabasePasswordRotations(context.Context) ([]store.DatabasePasswordRotation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.DatabasePasswordRotation
	for _, r := range s.recs {
		out = append(out, r)
	}
	return out, nil
}

func (s *fakeStore) SetDatabasePasswordRotationInterval(_ context.Context, n string, d int, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.recs[n]
	r.Database, r.IntervalDays = n, d
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	s.recs[n] = r
	return nil
}

func (s *fakeStore) SaveDatabasePasswordRotationState(_ context.Context, n, state, reason string, last, pending, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.recs[n]
	r.Database, r.State, r.LastError, r.PendingSince = n, state, reason, pending
	if !last.IsZero() {
		r.LastRotatedAt = last
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	s.recs[n] = r
	return nil
}

func (s *fakeStore) DeleteDatabasePasswordRotation(_ context.Context, n string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.recs, n)
	return nil
}

type fakeSecrets struct {
	vals    map[string]string
	failSet map[string]int
}

func (f *fakeSecrets) k(s, e string) string { return s + "/" + e }
func (f *fakeSecrets) Resolve(_ context.Context, s, e string) (string, error) {
	return f.vals[f.k(s, e)], nil
}
func (f *fakeSecrets) Exists(_ context.Context, s, e string) (bool, error) {
	_, ok := f.vals[f.k(s, e)]
	return ok, nil
}
func (f *fakeSecrets) SetValueGuarded(_ context.Context, s, e, v string, _ bool) error {
	if f.failSet[e] > 0 {
		f.failSet[e]--
		return errors.New("secret store unavailable")
	}
	f.vals[f.k(s, e)] = v
	return nil
}
func (f *fakeSecrets) ListKeys(context.Context, string) ([]store.SecretKeyInfo, error) {
	return nil, nil
}
func (f *fakeSecrets) DeleteValue(_ context.Context, s, e string) error {
	delete(f.vals, f.k(s, e))
	return nil
}

type fakeDatabases struct{ engine string }

func (f fakeDatabases) GetDesiredDatabase(_ context.Context, n string) (*store.DesiredDatabase, error) {
	if n != dbName {
		return nil, store.ErrDatabaseNotFound
	}
	return &store.DesiredDatabase{Name: n, Engine: f.engine}, nil
}

type fakeTarget struct{ ex Exec }

func (f fakeTarget) Locate(context.Context, *store.DesiredDatabase) (Exec, string, error) {
	return f.ex, "c1", nil
}

type fakeApps struct{ restarts int }

func (f *fakeApps) Dependents(context.Context, string) ([]string, []string, error) {
	return []string{"web", "worker"}, []string{"cron"}, nil
}
func (f *fakeApps) Restart(context.Context, string) []string {
	f.restarts++
	return []string{"web", "worker"}
}

type fakeAudit struct{ actions []string }

func (f *fakeAudit) Record(_ context.Context, a, _, _ string) { f.actions = append(f.actions, a) }

type rig struct {
	svc     *Service
	engine  *fakeEngine
	secrets *fakeSecrets
	store   *fakeStore
	apps    *fakeApps
	audit   *fakeAudit
	now     time.Time
}

func newRig(t *testing.T, engine string) *rig {
	t.Helper()
	r := &rig{
		engine:  &fakeEngine{password: "old-pw"},
		secrets: &fakeSecrets{vals: map[string]string{dbName + "/mysql_password": "old-pw"}, failSet: map[string]int{}},
		store:   &fakeStore{recs: map[string]store.DatabasePasswordRotation{}},
		apps:    &fakeApps{},
		audit:   &fakeAudit{},
		now:     time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
	}
	r.svc = &Service{
		Store: r.store, Databases: fakeDatabases{engine: engine}, Secrets: r.secrets,
		Target: fakeTarget{ex: r.engine}, Apps: r.apps, Auditor: r.audit,
		Now: func() time.Time { return r.now },
	}
	return r
}

func (r *rig) assertNoPasswordLeak(t *testing.T, passwords ...string) {
	t.Helper()
	for _, a := range r.audit.actions {
		for _, p := range passwords {
			if strings.Contains(a, p) {
				t.Fatalf("audit action leaks a password: %q", a)
			}
		}
	}
	for _, line := range r.engine.seen {
		cmd, _, _ := strings.Cut(line, "\n")
		for _, p := range passwords {
			if strings.Contains(cmd, p) {
				t.Fatalf("command line carries a password: %q", cmd)
			}
		}
	}
}

func TestRotateHappyPath(t *testing.T) {
	r := newRig(t, store.EngineMySQL)
	res, err := r.svc.Rotate(context.Background(), dbName, "alice")
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	fresh := r.secrets.vals[dbName+"/mysql_password"]
	if fresh == "old-pw" || fresh == "" || r.engine.password != fresh {
		t.Fatalf("stored %q, engine %q: want both the new password", fresh, r.engine.password)
	}
	if _, ok := r.secrets.vals[dbName+"/mysql_password_pending"]; ok {
		t.Error("staged password was not cleared")
	}
	if len(res.RestartedApps) != 2 || r.apps.restarts != 1 {
		t.Errorf("restarted = %v (%d calls), want the 2 linked apps once", res.RestartedApps, r.apps.restarts)
	}
	rec, _ := r.store.GetDatabasePasswordRotation(context.Background(), dbName)
	if rec.State != store.DBPasswordRotationIdle || !rec.LastRotatedAt.Equal(r.now) {
		t.Errorf("record = %+v, want idle and stamped", rec)
	}
	if got := strings.Join(r.audit.actions, ","); got != ActionStart+","+ActionComplete {
		t.Errorf("audit = %s", got)
	}
	r.assertNoPasswordLeak(t, "old-pw", fresh)
}

func TestRotateVerifyFailureRestoresOldPassword(t *testing.T) {
	r := newRig(t, store.EngineMySQL)
	r.engine.refuse = "will-set-below"
	orig := r.engine
	r.svc.Target = fakeTarget{ex: refuseNewExec{orig}}
	_, err := r.svc.Rotate(context.Background(), dbName, "alice")
	if !errors.Is(err, ErrVerifyFailed) {
		t.Fatalf("Rotate() error = %v, want ErrVerifyFailed", err)
	}
	if r.engine.password != "old-pw" || r.secrets.vals[dbName+"/mysql_password"] != "old-pw" {
		t.Errorf("engine %q, stored %q: want the old password back in both", r.engine.password, r.secrets.vals[dbName+"/mysql_password"])
	}
	if _, ok := r.secrets.vals[dbName+"/mysql_password_pending"]; ok {
		t.Error("staged password was not cleared after rollback")
	}
	rec, _ := r.store.GetDatabasePasswordRotation(context.Background(), dbName)
	if rec.State != store.DBPasswordRotationFailed || rec.LastError != ReasonVerifyFailed {
		t.Errorf("record = %+v", rec)
	}
	if r.apps.restarts != 0 {
		t.Error("apps restarted although the rotation was rolled back")
	}
}

// refuseNewExec makes the engine apply ALTER USER but refuse any login with
// a password other than the one it started with, as a server that took the
// change yet rejects the new credential would.
type refuseNewExec struct{ *fakeEngine }

func (e refuseNewExec) ExecWithInput(ctx context.Context, cid string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	raw, _ := io.ReadAll(stdin)
	first, rest, _ := strings.Cut(string(raw), "\n")
	if !strings.Contains(rest, "ALTER USER") && first != "old-pw" {
		return nil, &docker.ExecExitError{ExitCode: 1}
	}
	return e.fakeEngine.ExecWithInput(ctx, cid, cmd, strings.NewReader(string(raw)))
}

func TestHalfSucceededReferenceUpdateSelfHeals(t *testing.T) {
	r := newRig(t, store.EngineMySQL)
	r.secrets.failSet["mysql_password"] = 1
	_, err := r.svc.Rotate(context.Background(), dbName, "alice")
	if err == nil {
		t.Fatal("Rotate() error = nil, want the reference update failure")
	}
	staged := r.secrets.vals[dbName+"/mysql_password_pending"]
	if staged == "" || r.engine.password != staged || r.secrets.vals[dbName+"/mysql_password"] != "old-pw" {
		t.Fatalf("not half-succeeded: staged %q engine %q stored %q", staged, r.engine.password, r.secrets.vals[dbName+"/mysql_password"])
	}
	rec, _ := r.store.GetDatabasePasswordRotation(context.Background(), dbName)
	if rec.State != store.DBPasswordRotationPending {
		t.Fatalf("state = %q, want pending", rec.State)
	}

	r.svc.Sweep(context.Background())

	if got := r.secrets.vals[dbName+"/mysql_password"]; got != staged {
		t.Errorf("stored = %q, want the staged password after the next pass", got)
	}
	if _, ok := r.secrets.vals[dbName+"/mysql_password_pending"]; ok {
		t.Error("staged password still present after healing")
	}
	rec, _ = r.store.GetDatabasePasswordRotation(context.Background(), dbName)
	if rec.State != store.DBPasswordRotationIdle {
		t.Errorf("state = %q, want idle", rec.State)
	}
	if r.apps.restarts != 1 {
		t.Errorf("apps restarted %d times, want once after healing", r.apps.restarts)
	}
}

func TestEngineErrorAfterApplyIsSettled(t *testing.T) {
	r := newRig(t, store.EngineMySQL)
	r.engine.failSetAt = 1
	if _, err := r.svc.Rotate(context.Background(), dbName, "alice"); err != nil {
		t.Fatalf("Rotate() error = %v, want the settled rotation to complete", err)
	}
	if r.secrets.vals[dbName+"/mysql_password"] != r.engine.password || r.engine.password == "old-pw" {
		t.Errorf("stored %q engine %q: want both the new password", r.secrets.vals[dbName+"/mysql_password"], r.engine.password)
	}
}

func TestInterruptedBeforeApplyClearsStage(t *testing.T) {
	r := newRig(t, store.EngineMySQL)
	ctx := context.Background()
	r.secrets.vals[dbName+"/mysql_password_pending"] = "never-applied"
	_ = r.store.SaveDatabasePasswordRotationState(ctx, dbName, store.DBPasswordRotationPending, "", time.Time{}, r.now, r.now)
	if err := r.svc.Heal(ctx, dbName); err != nil {
		t.Fatalf("Heal() error = %v", err)
	}
	if r.secrets.vals[dbName+"/mysql_password"] != "old-pw" || r.engine.password != "old-pw" {
		t.Error("healing changed a password that was never rotated")
	}
	if _, ok := r.secrets.vals[dbName+"/mysql_password_pending"]; ok {
		t.Error("stale stage not cleared")
	}
	rec, _ := r.store.GetDatabasePasswordRotation(ctx, dbName)
	if rec.State != store.DBPasswordRotationFailed || rec.LastError != ReasonInterrupted {
		t.Errorf("record = %+v", rec)
	}
}

func TestUnsupportedEngine(t *testing.T) {
	for _, engine := range []string{store.EngineRedis, store.EngineClickHouse, store.EngineMeilisearch, store.EngineMemcached} {
		r := newRig(t, engine)
		if _, err := r.svc.Rotate(context.Background(), dbName, "alice"); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: Rotate() error = %v, want ErrUnsupported", engine, err)
		}
		p, err := r.svc.Preview(context.Background(), dbName)
		if err != nil || p.Supported || p.Reason == "" {
			t.Errorf("%s: Preview() = %+v, %v, want unsupported with a reason", engine, p, err)
		}
	}
}

func TestPreviewListsRestartsWithoutChanging(t *testing.T) {
	r := newRig(t, store.EngineMySQL)
	p, err := r.svc.Preview(context.Background(), dbName)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Supported || !p.RestartRequired || len(p.Apps) != 2 || len(p.SuspendedApps) != 1 {
		t.Errorf("Preview() = %+v", p)
	}
	if r.engine.setCalls != 0 || r.apps.restarts != 0 {
		t.Error("preview changed state")
	}
}

func TestSweepRotatesDueDatabasesOnly(t *testing.T) {
	r := newRig(t, store.EngineMySQL)
	ctx := context.Background()
	created := r.now.AddDate(0, 0, -10)
	r.now = created
	if err := r.svc.SetInterval(ctx, dbName, 30, "alice"); err != nil {
		t.Fatal(err)
	}
	r.now = created.AddDate(0, 0, 20)
	if got := r.svc.Sweep(ctx); got != 0 {
		t.Fatalf("Sweep() rotated %d before the interval elapsed", got)
	}
	r.now = created.AddDate(0, 0, 31)
	if got := r.svc.Sweep(ctx); got != 1 {
		t.Fatalf("Sweep() rotated %d, want 1 once due", got)
	}
	if got := r.svc.Sweep(ctx); got != 0 {
		t.Errorf("Sweep() rotated %d right after a rotation, want 0", got)
	}
}

func TestSetIntervalValidation(t *testing.T) {
	r := newRig(t, store.EngineMySQL)
	if err := r.svc.SetInterval(context.Background(), dbName, -1, "a"); !errors.Is(err, ErrBadInterval) {
		t.Errorf("error = %v, want ErrBadInterval", err)
	}
	ru := newRig(t, store.EngineRedis)
	if err := ru.svc.SetInterval(context.Background(), dbName, 7, "a"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("error = %v, want ErrUnsupported for a schedule on redis", err)
	}
}

func TestDriverSecretsStayOnStdin(t *testing.T) {
	cases := []struct {
		engine string
		marker string
	}{
		{store.EngineMySQL, "ALTER USER"},
		{store.EngineMariaDB, "ALTER USER"},
		{store.EngineMongoDB, "changeUserPassword"},
	}
	for _, c := range cases {
		d, ok := DriverFor(c.engine)
		if !ok {
			t.Fatalf("no driver for %s", c.engine)
		}
		rec := &recordExec{}
		if err := d.Set(context.Background(), rec, "c1", dbName, "admin-pw", "new-pw"); err != nil && c.engine != store.EngineMongoDB {
			t.Fatalf("%s Set() error = %v", c.engine, err)
		}
		if !strings.Contains(rec.stdin, c.marker) || !strings.HasPrefix(rec.stdin, "admin-pw\n") {
			t.Errorf("%s stdin = %q, want the admin password line then the statement", c.engine, rec.stdin)
		}
		if strings.Contains(strings.Join(rec.cmd, " "), "new-pw") || strings.Contains(strings.Join(rec.cmd, " "), "admin-pw") {
			t.Errorf("%s command line carries a password: %v", c.engine, rec.cmd)
		}
	}
}

func TestEngineErrorsNeverEchoOutput(t *testing.T) {
	d, _ := DriverFor(store.EngineMySQL)
	err := d.Verify(context.Background(), failExec{}, "c1", dbName, "s3cret-value")
	if err == nil || strings.Contains(err.Error(), "s3cret-value") || !errors.Is(err, ErrEngine) {
		t.Errorf("error = %v, want ErrEngine without the password or stderr", err)
	}
}

type recordExec struct {
	cmd   []string
	stdin string
}

func (r *recordExec) ExecWithInput(_ context.Context, _ string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	raw, _ := io.ReadAll(stdin)
	r.cmd, r.stdin = cmd, string(raw)
	return io.NopCloser(strings.NewReader("LR_OK\n")), nil
}

type failExec struct{}

func (failExec) ExecWithInput(_ context.Context, _ string, _ []string, stdin io.Reader) (io.ReadCloser, error) {
	raw, _ := io.ReadAll(stdin)
	return nil, &docker.ExecExitError{ExitCode: 1, Stderr: "Access denied for password " + string(raw)}
}
