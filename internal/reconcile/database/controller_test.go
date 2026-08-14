package database

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeStore is a hand-written fake, not a mocking framework, the same
// pattern application.fakeStore and nginxdemo's tests already establish
// for internal/store.
type fakeStore struct {
	db  *store.DesiredDatabase
	err error
}

func (f *fakeStore) GetDesiredDatabase(_ context.Context, _ string) (*store.DesiredDatabase, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.db, nil
}

// fakeRuntime is a stateful fake, not just a call counter: it tracks an
// in-memory set of "containers" and "volumes" so multi-step scenarios
// (ensure volume, then create, then re-inspect) can be tested
// realistically, the same pattern application.fakeRuntime established.
type fakeRuntime struct {
	mu         sync.Mutex
	containers map[string]*docker.ContainerState
	volumes    map[string]bool
	nextID     int

	ensureVolumeErr error
	createErr       error
	startErr        error
	stopErr         error
	removeErr       error

	createCalls       int
	ensureVolumeCalls int
	// lastCreateEnv records the Env of the most recent Create call, so a
	// test can assert exactly what a controller sent (e.g. which
	// MYSQL_*/POSTGRES_* keys), not just that Create was called.
	lastCreateEnv map[string]string
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{
		containers: map[string]*docker.ContainerState{},
		volumes:    map[string]bool{},
	}
}

func (f *fakeRuntime) seed(name, image string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.containers[name] = &docker.ContainerState{ID: strconv.Itoa(f.nextID), Name: name, Image: image, Running: running}
}

func (f *fakeRuntime) EnsureVolume(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureVolumeCalls++
	if f.ensureVolumeErr != nil {
		return f.ensureVolumeErr
	}
	f.volumes[name] = true
	return nil
}

func (f *fakeRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cs, ok := f.containers[name]
	if !ok {
		return nil, nil
	}
	cp := *cs
	return &cp, nil
}

func (f *fakeRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	f.lastCreateEnv = spec.Env
	if f.createErr != nil {
		return "", f.createErr
	}
	f.nextID++
	id := strconv.Itoa(f.nextID)
	f.containers[spec.Name] = &docker.ContainerState{ID: id, Name: spec.Name, Image: spec.Image}
	return id, nil
}

func (f *fakeRuntime) Start(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	for _, cs := range f.containers {
		if cs.ID == id {
			cs.Running = true
		}
	}
	return nil
}

func (f *fakeRuntime) Stop(_ context.Context, id string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopErr != nil {
		return f.stopErr
	}
	for _, cs := range f.containers {
		if cs.ID == id {
			cs.Running = false
		}
	}
	return nil
}

func (f *fakeRuntime) Remove(_ context.Context, id string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.removeErr != nil {
		return f.removeErr
	}
	for name, cs := range f.containers {
		if cs.ID == id {
			delete(f.containers, name)
		}
	}
	return nil
}

func (f *fakeRuntime) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []docker.ContainerState
	for name, cs := range f.containers {
		if strings.HasPrefix(name, prefix) {
			out = append(out, *cs)
		}
	}
	return out, nil
}

func (f *fakeRuntime) ListImages(_ context.Context, _ string) ([]docker.ImageInfo, error) {
	return nil, nil
}

func (f *fakeRuntime) Events(_ context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil
}

// Exec is unused by this package's own tests: this controller reconciles
// container lifecycle, never runs a command inside one. internal/backup's
// Dumper is the real Exec caller, exercised by internal/backup's own
// tests instead. Stubbed here to satisfy docker.Runtime.
func (f *fakeRuntime) Exec(_ context.Context, _ string, _ []string) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: Exec not implemented")
}

func (f *fakeRuntime) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.containers)
}

func (f *fakeRuntime) hasVolume(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.volumes[name]
}

func conditionOf(t *testing.T, result reconcile.Result) reconcile.Condition {
	t.Helper()
	if len(result.Conditions) == 0 {
		t.Fatal("expected at least one condition, got none")
	}
	return result.Conditions[0]
}

func TestController_Reconcile_NoDesiredState(t *testing.T) {
	rt := newFakeRuntime()
	c := New("main", &fakeStore{err: store.ErrDatabaseNotFound}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil (missing desired state is not a failure)", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionUnknown || cond.Reason != "NoDesiredState" {
		t.Errorf("condition = %+v, want Status=Unknown Reason=NoDesiredState", cond)
	}
}

func TestController_Reconcile_StoreError(t *testing.T) {
	rt := newFakeRuntime()
	c := New("main", &fakeStore{err: errors.New("db unreachable")}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "StoreError" {
		t.Errorf("condition = %+v, want Status=False Reason=StoreError", cond)
	}
}

func TestController_Reconcile_Redis_FreshDeploy(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	c := New("main", &fakeStore{db: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", rt.createCalls)
	}

	target := containerName("main")
	state, err := rt.InspectByName(context.Background(), target)
	if err != nil {
		t.Fatalf("InspectByName() error = %v", err)
	}
	if state == nil || state.Image != "redis:7" {
		t.Fatalf("state = %+v, want a container with image redis:7", state)
	}

	volName := dataVolumeName("main")
	if !rt.hasVolume(volName) {
		t.Errorf("expected volume %q to have been ensured", volName)
	}
}

func TestController_Reconcile_Redis_FreshDeploy_VersionDefaultsToLatest(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: ""}
	c := New("main", &fakeStore{db: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	state, err := rt.InspectByName(context.Background(), containerName("main"))
	if err != nil {
		t.Fatalf("InspectByName() error = %v", err)
	}
	if state == nil || state.Image != "redis:latest" {
		t.Fatalf("state = %+v, want image redis:latest when Version is empty", state)
	}
}

func TestController_Reconcile_Redis_AlreadyRunning_NoOp(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	rt.seed(containerName("main"), "redis:7", true)

	c := New("main", &fakeStore{db: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "AlreadyRunning" {
		t.Errorf("condition = %+v, want Status=True Reason=AlreadyRunning", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (already converged, must be a no-op)", rt.createCalls)
	}
}

func TestController_Reconcile_Redis_RestartAfterCrash(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	rt.seed(containerName("main"), "redis:7", false) // exists, not running: crashed

	c := New("main", &fakeStore{db: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0 (a crashed container is restarted, not recreated)", rt.createCalls)
	}
}

func TestController_Reconcile_Redis_VersionBump_ReplacesInPlace(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(containerName("main"), "redis:6", true)

	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	c := New("main", &fakeStore{db: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}

	// Exactly one container for this database, never two: a version
	// bump must replace in place, not run old and new side by side.
	if got := rt.count(); got != 1 {
		t.Fatalf("container count after version bump = %d, want 1 (sequential replace, not blue-green)", got)
	}
	state, err := rt.InspectByName(context.Background(), containerName("main"))
	if err != nil {
		t.Fatalf("InspectByName() error = %v", err)
	}
	if state == nil || state.Image != "redis:7" {
		t.Fatalf("state = %+v, want the replaced container running redis:7", state)
	}
}

func TestController_Reconcile_Redis_VolumeFailure(t *testing.T) {
	rt := newFakeRuntime()
	rt.ensureVolumeErr = errors.New("disk full")
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	c := New("main", &fakeStore{db: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the volume failure to surface")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "VolumeFailed" {
		t.Errorf("condition = %+v, want Status=False Reason=VolumeFailed", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: must not attempt to create a container without its volume", rt.createCalls)
	}
}

func TestController_Reconcile_Postgres_AlwaysCredentialsBlocked(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}
	c := New("main", &fakeStore{db: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: credentials-blocked is a documented permanent state, not a transient failure", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CredentialsNotConfigured" {
		t.Errorf("condition = %+v, want Status=False Reason=CredentialsNotConfigured", cond)
	}
	if cond.Message == "" {
		t.Error("expected a non-empty Message explaining the block")
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: must never start an unauthenticated postgres container", rt.createCalls)
	}
	if rt.ensureVolumeCalls != 0 {
		t.Errorf("ensureVolumeCalls = %d, want 0: must not touch Docker at all while blocked", rt.ensureVolumeCalls)
	}
}

func TestController_Reconcile_Postgres_CredentialsBlocked_EvenIfAlreadyRunning(t *testing.T) {
	// Even if something external created a postgres container out of
	// band, this controller's own Reconcile must never claim credit for
	// it or touch it: the credentials-blocked condition is unconditional
	// while postgresCreds is nil.
	rt := newFakeRuntime()
	rt.seed(containerName("main"), "postgres:16", true)
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}
	c := New("main", &fakeStore{db: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CredentialsNotConfigured" {
		t.Errorf("condition = %+v, want Status=False Reason=CredentialsNotConfigured", cond)
	}
}

func TestController_Reconcile_Postgres_WithCredentials_Reconciles(t *testing.T) {
	// Proves the activation path: once credentials are supplied (as they
	// will be once TASKS.md 1.7 lands), Postgres reconciles for real
	// through the same shared logic Redis uses, no rewrite needed.
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}
	c := New("main", &fakeStore{db: desired}, rt, WithPostgresCredentials(&PostgresCredentials{
		Username: "levelrail",
		Password: "s3cret",
	}))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", rt.createCalls)
	}
}

func TestController_Reconcile_MySQL_AlwaysCredentialsBlocked(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8"}
	c := New("main", &fakeStore{db: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: credentials-blocked is a documented permanent state, not a transient failure", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CredentialsNotConfigured" {
		t.Errorf("condition = %+v, want Status=False Reason=CredentialsNotConfigured", cond)
	}
	if cond.Message == "" {
		t.Error("expected a non-empty Message explaining the block")
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: must never start an unauthenticated mysql container", rt.createCalls)
	}
	if rt.ensureVolumeCalls != 0 {
		t.Errorf("ensureVolumeCalls = %d, want 0: must not touch Docker at all while blocked", rt.ensureVolumeCalls)
	}
}

func TestController_Reconcile_MySQL_CredentialsBlocked_EvenIfAlreadyRunning(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(containerName("main"), "mysql:8", true)
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8"}
	c := New("main", &fakeStore{db: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "CredentialsNotConfigured" {
		t.Errorf("condition = %+v, want Status=False Reason=CredentialsNotConfigured", cond)
	}
}

func TestController_Reconcile_MySQL_WithCredentials_Reconciles(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8"}
	c := New("main", &fakeStore{db: desired}, rt, WithMySQLCredentials(&MySQLCredentials{
		Username: "levelrail",
		Password: "s3cret",
	}))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Deployed" {
		t.Errorf("condition = %+v, want Status=True Reason=Deployed", cond)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", rt.createCalls)
	}
}

func TestController_Reconcile_MySQL_WithCredentials_SetsRootAndUserEnv(t *testing.T) {
	// MySQL's own image requires MYSQL_ROOT_PASSWORD (or an explicit
	// opt-out) to start at all, unlike Postgres where POSTGRES_PASSWORD
	// alone is enough: this asserts the container spec actually carries
	// both the root password and the dbName-scoped user/database, not
	// just the user side Postgres' own shape would suggest by analogy.
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineMySQL, Version: "8"}
	c := New("main", &fakeStore{db: desired}, rt, WithMySQLCredentials(&MySQLCredentials{
		Username: "main",
		Password: "s3cret",
	}))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	env := rt.lastCreateEnv
	if env["MYSQL_ROOT_PASSWORD"] != "s3cret" {
		t.Errorf("MYSQL_ROOT_PASSWORD = %q, want %q", env["MYSQL_ROOT_PASSWORD"], "s3cret")
	}
	if env["MYSQL_DATABASE"] != "main" {
		t.Errorf("MYSQL_DATABASE = %q, want %q", env["MYSQL_DATABASE"], "main")
	}
	if env["MYSQL_USER"] != "main" {
		t.Errorf("MYSQL_USER = %q, want %q", env["MYSQL_USER"], "main")
	}
	if env["MYSQL_PASSWORD"] != "s3cret" {
		t.Errorf("MYSQL_PASSWORD = %q, want %q", env["MYSQL_PASSWORD"], "s3cret")
	}
}

func TestController_Reconcile_UnknownEngine(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: "mongodb", Version: "7"}
	c := New("main", &fakeStore{db: desired}, rt)

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error for an unrecognized engine")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "UnknownEngine" {
		t.Errorf("condition = %+v, want Status=False Reason=UnknownEngine", cond)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
}

func TestController_Name(t *testing.T) {
	c := New("main", &fakeStore{}, newFakeRuntime())
	if got, want := c.Name(), "database/main"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestContainerName_DeterministicPerDatabase_NotPerVersion(t *testing.T) {
	a := containerName("main")
	b := containerName("main")
	other := containerName("cache")

	if a != b {
		t.Errorf("containerName is not deterministic: %q != %q for the same input", a, b)
	}
	if a == other {
		t.Errorf("containerName collided for different databases: %q", a)
	}
	if !strings.HasPrefix(a, "db-") {
		t.Errorf("containerName(%q) = %q, want it prefixed with db-", "main", a)
	}
}

func TestDataVolumeName_StableAndDistinctFromContainerName(t *testing.T) {
	vol := dataVolumeName("main")
	cname := containerName("main")
	if vol == cname {
		t.Errorf("dataVolumeName and containerName collided: %q", vol)
	}
	if !strings.HasPrefix(vol, "db-main") {
		t.Errorf("dataVolumeName(%q) = %q, want it prefixed with db-main", "main", vol)
	}
}

func TestVersionOrDefault(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty defaults to latest", in: "", want: "latest"},
		{name: "explicit version passed through", in: "16", want: "16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionOrDefault(tt.in); got != tt.want {
				t.Errorf("versionOrDefault(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
